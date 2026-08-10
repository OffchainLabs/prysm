package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensusblocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// storeExecutionPayloadEnvelope creates and caches the execution payload envelope
// after the block is fully built (state root set), returning the envelope for the caller to bundle.
func (vs *Server) storeExecutionPayloadEnvelope(
	sBlk interfaces.SignedBeaconBlock,
	local *consensusblocks.GetPayloadResponse,
) (*ethpb.ExecutionPayloadEnvelope, error) {
	blockRoot, err := sBlk.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block hash tree root")
	}

	payload := extractExecutionPayloadGloas(local)

	parentRoot := sBlk.Block().ParentRoot()
	envelope := &ethpb.ExecutionPayloadEnvelope{
		Payload:               payload,
		ExecutionRequests:     local.ExecutionRequestsGloas,
		BuilderIndex:          params.BeaconConfig().BuilderIndexSelfBuild,
		BeaconBlockRoot:       blockRoot[:],
		ParentBeaconBlockRoot: parentRoot[:],
	}

	// Precompute sidecars here (during ProposeBeaconBlock slack) so publish stays fast.
	var roSidecars []consensusblocks.RODataColumn
	if bundle := local.BlobsBundler; bundle != nil && len(bundle.GetBlobs()) > 0 {
		cellsPerBlob, proofsPerBlob, err := peerdas.ComputeCellsAndProofsFromFlat(bundle.GetBlobs(), bundle.GetProofs())
		if err != nil {
			return nil, errors.Wrap(err, "compute cells and proofs from blobs bundle")
		}
		roSidecars, err = peerdas.DataColumnSidecarsGloas(cellsPerBlob, proofsPerBlob, sBlk.Block().Slot(), blockRoot)
		if err != nil {
			return nil, errors.Wrap(err, "build gloas data column sidecars")
		}
	}

	vs.CoreService.ExecutionPayloadEnvelopeCache.Set(&cache.ExecutionPayloadContents{
		Envelope:    envelope,
		DataColumns: roSidecars,
	})
	return envelope, nil
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

// GetExecutionPayloadEnvelope returns the cached execution payload envelope for the requested
// slot so the proposer can sign and publish it.
func (vs *Server) GetExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.ExecutionPayloadEnvelopeRequest,
) (*ethpb.ExecutionPayloadEnvelopeResponse, error) {
	envelope, rpcErr := vs.CoreService.GetExecutionPayloadEnvelope(ctx, req)
	if rpcErr != nil {
		return nil, status.Error(core.ErrorReasonToGRPC(rpcErr.Reason), rpcErr.Err.Error())
	}
	return &ethpb.ExecutionPayloadEnvelopeResponse{
		Envelope: envelope,
	}, nil
}

// PublishExecutionPayloadEnvelope validates and broadcasts a signed execution payload envelope,
// called by validators after signing the envelope from GetExecutionPayloadEnvelope.
func (vs *Server) PublishExecutionPayloadEnvelope(
	ctx context.Context,
	req *ethpb.GenericSignedExecutionPayloadEnvelope,
) (*emptypb.Empty, error) {
	if rpcErr := vs.CoreService.PublishExecutionPayloadEnvelope(ctx, req); rpcErr != nil {
		return nil, status.Error(core.ErrorReasonToGRPC(rpcErr.Reason), rpcErr.Err.Error())
	}
	return &emptypb.Empty{}, nil
}

// setParentExecutionRequests populates the parent_execution_requests field
// in the block body based on the parent's execution payload envelope.
func (vs *Server) setParentExecutionRequests(ctx context.Context, sBlk interfaces.SignedBeaconBlock, head state.BeaconState, parentFull bool) error {
	if head.Version() < version.Gloas {
		return sBlk.SetParentExecutionRequests(&enginev1.ExecutionRequestsGloas{})
	}

	parentRoot := sBlk.Block().ParentRoot()
	parentSlot, err := vs.ForkchoiceFetcher.RecentBlockSlot(parentRoot)
	if err != nil {
		return errors.Wrap(err, "could not get parent block slot")
	}
	if slots.ToEpoch(parentSlot) < params.BeaconConfig().GloasForkEpoch || !parentFull {
		return sBlk.SetParentExecutionRequests(&enginev1.ExecutionRequestsGloas{})
	}

	// TODO: replace DB lookup with a single-entry cache (blockroot → envelope).
	signedEnvelope, err := vs.BeaconDB.ExecutionPayloadEnvelope(ctx, parentRoot)
	if err != nil {
		return errors.Wrap(err, "could not get parent execution payload envelope")
	}
	return sBlk.SetParentExecutionRequests(signedEnvelope.Message.ExecutionRequests)
}
