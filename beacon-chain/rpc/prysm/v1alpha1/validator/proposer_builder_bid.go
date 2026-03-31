package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetExecutionPayloadBid produces an unsigned execution payload bid from the
// local execution engine for a validator acting as a GLOAS builder.
func (vs *Server) GetExecutionPayloadBid(
	ctx context.Context,
	req *ethpb.ExecutionPayloadBidRequest,
) (*ethpb.ExecutionPayloadBid, error) {
	ctx, span := trace.StartSpan(ctx, "ProposerServer.GetExecutionPayloadBid")
	defer span.End()

	if req == nil {
		return nil, status.Errorf(codes.InvalidArgument, "request is nil")
	}

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	if slots.ToEpoch(req.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload bids are not supported before Gloas fork (slot %d)", req.Slot)
	}

	headState, err := vs.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get head state: %v", err)
	}

	headRootBytes, err := vs.HeadFetcher.HeadRoot(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get head root: %v", err)
	}
	headRoot := bytesutil.ToBytes32(headRootBytes)

	proposerIdx, err := helpers.BeaconProposerIndex(ctx, headState)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get proposer index: %v", err)
	}

	local, err := vs.getLocalPayloadFromEngine(ctx, headState, headRoot, req.Slot, proposerIdx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get local payload: %v", err)
	}
	if local == nil || local.ExecutionData == nil || local.ExecutionData.IsNil() {
		return nil, status.Errorf(codes.NotFound, "no local execution payload available for slot %d", req.Slot)
	}

	ed := local.ExecutionData
	return &ethpb.ExecutionPayloadBid{
		ParentBlockHash:    ed.ParentHash(),
		ParentBlockRoot:    bytesutil.SafeCopyBytes(headRoot[:]),
		BlockHash:          ed.BlockHash(),
		PrevRandao:         ed.PrevRandao(),
		FeeRecipient:       ed.FeeRecipient(),
		GasLimit:           ed.GasLimit(),
		BuilderIndex:       req.BuilderIndex,
		Slot:               req.Slot,
		Value:              primitives.WeiToGwei(local.Bid),
		ExecutionPayment:   0,
		BlobKzgCommitments: local.BlobsBundler.GetKzgCommitments(),
	}, nil
}
