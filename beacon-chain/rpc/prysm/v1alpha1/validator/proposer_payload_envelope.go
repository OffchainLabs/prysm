package validator

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// storeExecutionPayloadEnvelope creates and caches the execution payload envelope
// after the block is fully built (state root set). If postBlockState is non-nil,
func (vs *Server) storeExecutionPayloadEnvelope(
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
) error {
	blockRoot, err := sBlk.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute block hash tree root")
	}

	payload := extractExecutionPayloadGloas(local)

	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload:           payload,
		ExecutionRequests: local.ExecutionRequests,
		BuilderIndex:      params.BeaconConfig().BuilderIndexSelfBuild,
		BeaconBlockRoot:   blockRoot[:],
	}

	// Precompute data column sidecars now (inside ProposeBeaconBlock) so the
	// expensive KZG cell computation doesn't run during
	// PublishExecutionPayloadEnvelope. Raw blobs and KZG proofs can be
	// reconstructed from the cached sidecars when the v4 producer response
	// needs them.
	var roSidecars []consensusblocks.RODataColumn
	if bundle := local.BlobsBundler; bundle != nil && len(bundle.GetBlobs()) > 0 {
		cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(bundle.GetBlobs(), bundle.GetProofs())
		if err != nil {
			return errors.Wrap(err, "compute cells and proofs from blobs bundle")
		}
		roSidecars, err = peerdas.DataColumnSidecarsGloas(cellsPerBlob, proofsPerBlob, sBlk.Block().Slot(), blockRoot)
		if err != nil {
			return errors.Wrap(err, "build gloas data column sidecars")
		}
	}

	vs.ExecutionPayloadEnvelopeCache.Set(&cache.ExecutionPayloadContents{
		Envelope:    envelope,
		DataColumns: roSidecars,
	})
	return nil
}

func extractExecutionPayloadGloas(local *consensusblocks.GetPayloadResponse) *enginev1.ExecutionPayloadGloas {
	if local == nil || local.ExecutionData == nil || local.ExecutionData.IsNil() {
		return nil
	}
	if p, ok := local.ExecutionData.Proto().(*enginev1.ExecutionPayloadGloas); ok {
		return p
	}
	return nil
}

// GetExecutionPayloadEnvelope implements the gRPC endpoint:
// /eth/v1alpha1/validator/execution_payload_envelope/{slot}/{builder_index}
// It returns the stored execution payload envelope for a slot/builder and, for
// self-build envelopes, computes the post-payload state root on demand.
func (vs *Server) GetExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.ExecutionPayloadEnvelopeRequest,
) (*ethpb.ExecutionPayloadEnvelopeResponse, error) {
	_, span := trace.StartSpan(ctx, "ProposerServer.GetExecutionPayloadEnvelope")
	defer span.End()

	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	span.SetAttributes(trace.Int64Attribute("slot", int64(req.Slot)))

	if slots.ToEpoch(req.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload envelopes are not supported before Gloas fork (slot %d)", req.Slot)
	}

	contents, ok := vs.ExecutionPayloadEnvelopeCache.Contents()
	if !ok || contents.Envelope.Payload.SlotNumber != req.Slot {
		return nil, status.Errorf(codes.NotFound,
			"execution payload envelope not found for slot %d", req.Slot)
	}

	return &ethpb.ExecutionPayloadEnvelopeResponse{
		Envelope: contents.Envelope,
	}, nil
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

	if req == nil || req.Message == nil || req.Message.Payload == nil {
		return nil, status.Error(codes.InvalidArgument, "signed envelope or payload cannot be nil")
	}

	envSlot := primitives.Slot(req.Message.Payload.SlotNumber)
	if slots.ToEpoch(envSlot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload envelopes are not supported before Gloas fork (slot %d)", envSlot)
	}

	beaconBlockRoot := bytesutil.ToBytes32(req.Message.BeaconBlockRoot)
	span.SetAttributes(
		trace.Int64Attribute("slot", int64(envSlot)), // lint:ignore uintcast -- safe for tracing.
		trace.Int64Attribute("builderIndex", int64(req.Message.BuilderIndex)),
		trace.StringAttribute("beaconBlockRoot", fmt.Sprintf("%#x", beaconBlockRoot[:8])),
	)

	log := log.WithFields(logrus.Fields{
		"slot":            envSlot,
		"builderIndex":    req.Message.BuilderIndex,
		"beaconBlockRoot": fmt.Sprintf("%#x", beaconBlockRoot[:8]),
	})
	log.Info("Publishing signed execution payload envelope")

	// Broadcast pre-computed data column sidecars BEFORE receiving the envelope,
	// because ReceiveExecutionPayloadEnvelope checks data availability. Sidecars
	// were computed during ProposeBeaconBlock (storeExecutionPayloadEnvelope).
	// The slot guard prevents broadcasting sidecars whose block root commits to
	// a different slot than the envelope being published.
	if contents, ok := vs.ExecutionPayloadEnvelopeCache.Contents(); ok &&
		contents.Envelope.Payload.SlotNumber == envSlot && len(contents.DataColumns) > 0 {
		if err := vs.broadcastAndReceiveDataColumns(ctx, contents.DataColumns); err != nil {
			log.WithError(err).Error("Failed to broadcast Gloas data column sidecars")
		}
	}

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

	log.Info("Successfully published execution payload envelope")

	return &emptypb.Empty{}, nil
}

// setParentExecutionRequests populates the parent_execution_requests field
// in the block body based on the parent's execution payload envelope.
func (vs *Server) setParentExecutionRequests(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, parentFull bool) error {
	if head.Version() < version.Gloas {
		return sBlk.SetParentExecutionRequests(&enginev1.ExecutionRequests{})
	}

	parentRoot := sBlk.Block().ParentRoot()
	parentSlot, err := vs.ForkchoiceFetcher.RecentBlockSlot(parentRoot)
	if err != nil {
		return errors.Wrap(err, "could not get parent block slot")
	}
	if slots.ToEpoch(parentSlot) < params.BeaconConfig().GloasForkEpoch || !parentFull {
		return sBlk.SetParentExecutionRequests(&enginev1.ExecutionRequests{})
	}

	// TODO: replace DB lookup with a single-entry cache (blockroot → envelope).
	signedEnvelope, err := vs.BeaconDB.ExecutionPayloadEnvelope(ctx, parentRoot)
	if err != nil {
		return errors.Wrap(err, "could not get parent execution payload envelope")
	}
	return sBlk.SetParentExecutionRequests(signedEnvelope.Message.ExecutionRequests)
}
