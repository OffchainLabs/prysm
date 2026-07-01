package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SubmitSignedExecutionPayloadBid broadcasts a signed execution payload bid
// to the P2P gossip network.
func (vs *Server) SubmitSignedExecutionPayloadBid(
	ctx context.Context,
	req *ethpb.SignedExecutionPayloadBid,
) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "ValidatorServer.SubmitSignedExecutionPayloadBid")
	defer span.End()

	if req == nil || req.Message == nil {
		return nil, status.Errorf(codes.InvalidArgument, "signed execution payload bid is nil")
	}

	if vs.SyncChecker.Syncing() {
		return nil, status.Errorf(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}

	if slots.ToEpoch(req.Message.Slot) < params.BeaconConfig().GloasForkEpoch {
		return nil, status.Errorf(codes.InvalidArgument,
			"execution payload bids are not supported before Gloas fork (slot %d)", req.Message.Slot)
	}

	if err := vs.P2P.Broadcast(ctx, req); err != nil {
		return nil, status.Errorf(codes.Internal, "could not broadcast signed execution payload bid: %v", err)
	}

	return &emptypb.Empty{}, nil
}
