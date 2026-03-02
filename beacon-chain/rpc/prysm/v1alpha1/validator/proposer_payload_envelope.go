package validator

import (
	"bytes"
	"context"
	"fmt"

	coregloas "github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// storeExecutionPayloadEnvelope creates and caches the execution payload envelope
// after the block is fully built (state root set). The envelope is cached with a
// zeroed state root; the actual post-payload state root is computed lazily in
// GetExecutionPayloadEnvelope once the block has been submitted and the post-block
// state is available via StateGen.
func (vs *Server) storeExecutionPayloadEnvelope(
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
) error {
	blockRoot, err := sBlk.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute block hash tree root")
	}

	payload := extractExecutionPayloadDeneb(local)

	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload:           payload,
		ExecutionRequests: local.ExecutionRequests,
		BuilderIndex:      params.BeaconConfig().BuilderIndexSelfBuild,
		BeaconBlockRoot:   blockRoot[:],
		Slot:              sBlk.Block().Slot(),
		StateRoot:         make([]byte, 32), // zeroed; computed lazily in GetExecutionPayloadEnvelope
	}

	vs.setExecutionPayloadEnvelope(envelope)
	return nil
}

func extractExecutionPayloadDeneb(local *consensusblocks.GetPayloadResponse) *enginev1.ExecutionPayloadDeneb {
	if local == nil || local.ExecutionData == nil || local.ExecutionData.IsNil() {
		return nil
	}
	if p, ok := local.ExecutionData.Proto().(*enginev1.ExecutionPayloadDeneb); ok {
		return p
	}
	return nil
}

func (vs *Server) setExecutionPayloadEnvelope(envelope *ethpb.ExecutionPayloadEnvelope) {
	if envelope == nil {
		return
	}
	vs.executionPayloadEnvelopeMu.Lock()
	defer vs.executionPayloadEnvelopeMu.Unlock()
	vs.executionPayloadEnvelope = envelope
}

func (vs *Server) getExecutionPayloadEnvelope(slot primitives.Slot) (*ethpb.ExecutionPayloadEnvelope, bool) {
	vs.executionPayloadEnvelopeMu.RLock()
	envelope := vs.executionPayloadEnvelope
	vs.executionPayloadEnvelopeMu.RUnlock()
	if envelope == nil {
		return nil, false
	}
	if envelope.Slot != slot {
		return nil, false
	}
	return envelope, true
}

// GetExecutionPayloadEnvelope implements the gRPC endpoint:
// /eth/v1alpha1/validator/execution_payload_envelope/{slot}/{builder_index}
// It returns the stored execution payload envelope for a slot/builder and, for
// self-build envelopes, computes the post-payload state root on demand.
func (vs *Server) GetExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.ExecutionPayloadEnvelopeRequest,
) (*ethpb.ExecutionPayloadEnvelopeResponse, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.GetExecutionPayloadEnvelope")
	defer span.End()

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	span.SetAttributes(trace.Int64Attribute("slot", int64(req.Slot)))

	if slots.ToEpoch(req.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload envelopes are not supported before Gloas fork (slot %d)", req.Slot)
	}

	envelope, found := vs.getExecutionPayloadEnvelope(req.Slot)
	if !found {
		return nil, status.Errorf(codes.NotFound,
			"execution payload envelope not found for slot %d", req.Slot)
	}

	if bytes.Equal(envelope.StateRoot, make([]byte, 32)) {
		// Lazily set the state root in the envelope by applying the payload evelope on the post block state
		roEnvelope, err := consensusblocks.WrappedROExecutionPayloadEnvelope(envelope)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "could not wrap envelope: %v", err)
		}
		stateRoot, err := vs.computePostPayloadStateRoot(ctx, roEnvelope)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "could not compute post-payload state root: %v", err)
		}
		vs.executionPayloadEnvelopeMu.Lock()
		envelope.StateRoot = stateRoot
		vs.executionPayloadEnvelopeMu.Unlock()
	}

	return &ethpb.ExecutionPayloadEnvelopeResponse{
		Envelope: envelope,
	}, nil
}

// computePostPayloadStateRoot retrieves the post-block state (after the block has
// been submitted and processed) and applies the execution payload state mutations
// to compute the post-payload state root for the envelope.
func (vs *Server) computePostPayloadStateRoot(ctx context.Context, envelope interfaces.ROExecutionPayloadEnvelope) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.computePostPayloadStateRoot")
	defer span.End()

	beaconState, err := vs.StateGen.StateByRoot(ctx, envelope.BeaconBlockRoot())
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve post-block state")
	}
	beaconState = beaconState.Copy()
	if err := coregloas.ApplyExecutionPayload(ctx, beaconState, envelope); err != nil {
		return nil, errors.Wrapf(err, "could not apply execution payload at slot %d", beaconState.Slot())
	}
	root, err := beaconState.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "could not compute post-payload state root at slot %d", beaconState.Slot())
	}
	return root[:], nil
}

// PublishExecutionPayloadEnvelope validates and broadcasts a signed execution payload envelope.
// This is called by validators after signing the envelope retrieved from GetExecutionPayloadEnvelope.
//
// gRPC endpoint: POST /eth/v1alpha1/validator/execution_payload_envelope
func (vs *Server) PublishExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.SignedExecutionPayloadEnvelope,
) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.PublishExecutionPayloadEnvelope")
	defer span.End()

	if req == nil || req.Message == nil {
		return nil, status.Error(codes.InvalidArgument, "signed envelope cannot be nil")
	}

	if slots.ToEpoch(req.Message.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload envelopes are not supported before Gloas fork (slot %d)", req.Message.Slot)
	}

	beaconBlockRoot := bytesutil.ToBytes32(req.Message.BeaconBlockRoot)
	span.SetAttributes(
		trace.Int64Attribute("slot", int64(req.Message.Slot)),
		trace.Int64Attribute("builderIndex", int64(req.Message.BuilderIndex)),
		trace.StringAttribute("beaconBlockRoot", fmt.Sprintf("%#x", beaconBlockRoot[:8])),
	)

	log := log.WithFields(logrus.Fields{
		"slot":            req.Message.Slot,
		"builderIndex":    req.Message.BuilderIndex,
		"beaconBlockRoot": fmt.Sprintf("%#x", beaconBlockRoot[:8]),
	})
	log.Info("Publishing signed execution payload envelope")

	if err := vs.P2P.Broadcast(ctx, req); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to broadcast execution payload envelope: %v", err)
	}

	roSigned, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not wrap signed envelope: %v", err)
	}
	if err := vs.ExecutionPayloadEnvelopeReceiver.ReceiveExecutionPayloadEnvelope(ctx, roSigned); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to receive execution payload envelope: %v", err)
	}

	// TODO: Build and broadcast data column sidecars from the cached blobs bundle.
	// In Gloas, blob data is delivered alongside the execution payload envelope
	// rather than with the beacon block (which only carries the bid). Not needed
	// for devnet-0.

	log.Info("Successfully published execution payload envelope")

	return &emptypb.Empty{}, nil
}
