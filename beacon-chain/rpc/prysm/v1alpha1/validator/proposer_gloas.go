package validator

import (
	"bytes"
	"context"
	"fmt"

	coregloas "github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls/common"
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

// setGloasExecutionData creates an execution payload bid from the local payload
// and sets it on the block body. The envelope is created and cached later by
// buildExecutionPayloadEnvelope once the block is fully built.
func (vs *Server) setGloasExecutionData(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
) error {
	_, span := trace.StartSpan(ctx, "ProposerServer.setGloasExecutionData")
	defer span.End()

	if local == nil || local.ExecutionData == nil {
		return errors.New("local execution payload is nil")
	}

	// Create execution payload bid from the local payload.
	parentRoot := sBlk.Block().ParentRoot()
	bid, err := vs.createSelfBuildExecutionPayloadBid(
		local,
		primitives.BuilderIndex(sBlk.Block().ProposerIndex()),
		parentRoot[:],
		sBlk.Block().Slot(),
	)
	if err != nil {
		return errors.Wrap(err, "could not create execution payload bid")
	}

	// Per spec, self-build bids must use G2 point-at-infinity as the signature.
	// Only the execution payload envelope requires a real signature from the proposer.
	signedBid := &ethpb.SignedExecutionPayloadBid{
		Message:   bid,
		Signature: common.InfiniteSignature[:],
	}
	if err := sBlk.SetSignedExecutionPayloadBid(signedBid); err != nil {
		return errors.Wrap(err, "could not set signed execution payload bid")
	}

	return nil
}

// buildExecutionPayloadEnvelope creates and caches the execution payload envelope
// after the block is fully built (state root set). The envelope is cached with a
// zeroed state root; the actual post-payload state root is computed lazily in
// ExecutionPayloadEnvelope once the block has been submitted and the post-block
// state is available via StateGen.
func (vs *Server) buildExecutionPayloadEnvelope(
	ctx context.Context,
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
) error {
	blockRoot, err := sBlk.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not compute block hash tree root")
	}

	// Extract the underlying ExecutionPayloadDeneb proto.
	var payload *enginev1.ExecutionPayloadDeneb
	if local.ExecutionData != nil && !local.ExecutionData.IsNil() {
		if p, ok := local.ExecutionData.Proto().(*enginev1.ExecutionPayloadDeneb); ok {
			payload = p
		}
	}

	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload:           payload,
		ExecutionRequests: local.ExecutionRequests,
		BuilderIndex:      primitives.BuilderIndex(sBlk.Block().ProposerIndex()),
		BeaconBlockRoot:   blockRoot[:],
		Slot:              sBlk.Block().Slot(),
		StateRoot:         make([]byte, 32), // zeroed; computed lazily in ExecutionPayloadEnvelope
	}

	vs.ExecutionPayloadEnvelopeCache.Set(envelope, local.BlobsBundler)
	return nil
}

// computePostPayloadStateRoot retrieves the post-block state (after the block has
// been submitted and processed) and applies the execution payload state mutations
// to compute the post-payload state root for the envelope.
func (vs *Server) computePostPayloadStateRoot(ctx context.Context, envelope interfaces.ROExecutionPayloadEnvelope) ([]byte, error) {
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

// getPayloadAttestations returns payload attestations for inclusion in a GLOAS block.
// PTC members broadcast PayloadAttestationMessages via P2P gossip during slot N.
// All nodes collect these in a pool. The slot N+1 proposer retrieves and aggregates
// them into PayloadAttestations for block inclusion.
func (vs *Server) getPayloadAttestations(ctx context.Context, head state.BeaconState, slot primitives.Slot) []*ethpb.PayloadAttestation {
	// TODO: Retrieve and aggregate PayloadAttestationMessages from the pool
	// for the previous slot. Blocks are valid without payload attestations.
	return []*ethpb.PayloadAttestation{}
}

// createSelfBuildExecutionPayloadBid creates an ExecutionPayloadBid for self-building,
// where the proposer acts as its own builder. The value is the block value from the
// execution layer, and execution payment is zero since the proposer doesn't pay themselves.
func (vs *Server) createSelfBuildExecutionPayloadBid(
	local *consensusblocks.GetPayloadResponse,
	builderIndex primitives.BuilderIndex,
	parentBlockRoot []byte,
	slot primitives.Slot,
) (*ethpb.ExecutionPayloadBid, error) {
	ed := local.ExecutionData
	if ed == nil || ed.IsNil() {
		return nil, errors.New("execution data is nil")
	}

	return &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    ed.ParentHash(),
		ParentBlockRoot:    bytesutil.SafeCopyBytes(parentBlockRoot),
		BlockHash:          ed.BlockHash(),
		PrevRandao:         ed.PrevRandao(),
		FeeRecipient:       ed.FeeRecipient(),
		GasLimit:           ed.GasLimit(),
		BuilderIndex:       builderIndex,
		Slot:               slot,
		Value:              primitives.WeiToGwei(local.Bid),
		ExecutionPayment:   0, // Self-build: proposer doesn't pay themselves.
		BlobKzgCommitments: extractKzgCommitments(local.BlobsBundler),
	}, nil
}

// extractKzgCommitments pulls KZG commitments from a blobs bundler.
func extractKzgCommitments(blobsBundler enginev1.BlobsBundler) [][]byte {
	if blobsBundler == nil {
		return nil
	}
	switch b := blobsBundler.(type) {
	case *enginev1.BlobsBundle:
		if b != nil {
			return b.KzgCommitments
		}
	case *enginev1.BlobsBundleV2:
		if b != nil {
			return b.KzgCommitments
		}
	}
	return nil
}

// ExecutionPayloadEnvelope retrieves a cached execution payload envelope.
// If the envelope has a zeroed state root (self-build), the post-payload state
// root is lazily computed using the post-block state from StateGen.
//
// gRPC endpoint: /eth/v1alpha1/validator/execution_payload_envelope/{slot}/{builder_index}
func (vs *Server) ExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.ExecutionPayloadEnvelopeRequest,
) (*ethpb.ExecutionPayloadEnvelopeResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}

	if slots.ToEpoch(req.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload envelopes are not supported before GLOAS fork (slot %d)", req.Slot)
	}

	envelope, found := vs.ExecutionPayloadEnvelopeCache.Get(req.Slot, req.BuilderIndex)
	if !found {
		return nil, status.Errorf(
			codes.NotFound,
			"execution payload envelope not found for slot %d builder %d",
			req.Slot,
			req.BuilderIndex,
		)
	}

	// Lazily compute the post-payload state root if the envelope was cached
	// with a zeroed state root (self-build). The block must have been submitted
	// by this point so the post-block state is available via StateGen.
	if bytes.Equal(envelope.StateRoot, make([]byte, 32)) {
		roEnvelope, err := consensusblocks.WrappedROExecutionPayloadEnvelope(envelope)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "could not wrap envelope: %v", err)
		}
		stateRoot, err := vs.computePostPayloadStateRoot(ctx, roEnvelope)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "could not compute post-payload state root: %v", err)
		}
		envelope.StateRoot = stateRoot
	}

	return &ethpb.ExecutionPayloadEnvelopeResponse{
		Envelope: envelope,
	}, nil
}

// PublishExecutionPayloadEnvelope validates and broadcasts a signed execution payload envelope.
// This is called by validators after signing the envelope retrieved from ExecutionPayloadEnvelope.
//
// gRPC endpoint: POST /eth/v1alpha1/validator/execution_payload_envelope
func (vs *Server) PublishExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.SignedExecutionPayloadEnvelope,
) (*emptypb.Empty, error) {
	if req == nil || req.Message == nil {
		return nil, status.Error(codes.InvalidArgument, "signed envelope cannot be nil")
	}

	if slots.ToEpoch(req.Message.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload envelopes are not supported before GLOAS fork (slot %d)", req.Message.Slot)
	}

	beaconBlockRoot := bytesutil.ToBytes32(req.Message.BeaconBlockRoot)

	log := log.WithFields(logrus.Fields{
		"slot":            req.Message.Slot,
		"builderIndex":    req.Message.BuilderIndex,
		"beaconBlockRoot": fmt.Sprintf("%#x", beaconBlockRoot[:8]),
	})
	log.Info("Publishing signed execution payload envelope")

	// Validate the envelope signature before broadcasting.
	roSigned, err := consensusblocks.WrappedROSignedExecutionPayloadEnvelope(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid envelope: %v", err)
	}
	headState, err := vs.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get head state: %v", err)
	}
	if err := coregloas.VerifyExecutionPayloadEnvelopeSignature(headState, roSigned); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "envelope signature verification failed: %v", err)
	}

	if err := vs.P2P.Broadcast(ctx, req); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to broadcast execution payload envelope: %v", err)
	}

	// TODO: Receive the envelope locally following the broadcastReceiveBlock pattern.

	// TODO: Build and broadcast data column sidecars from the cached blobs bundle.
	// In GLOAS, blob data is delivered alongside the execution payload envelope
	// rather than with the beacon block (which only carries the bid). Not needed
	// for devnet-0.

	log.Info("Successfully published execution payload envelope")

	return &emptypb.Empty{}, nil
}
