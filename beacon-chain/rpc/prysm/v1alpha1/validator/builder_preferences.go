package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SubmitBuilderPreferences forwards a proposer's signed preferences to the named
// builder ahead of a proposal slot; bid-selection params travel with the block request.
func (vs *Server) SubmitBuilderPreferences(ctx context.Context, req *ethpb.SubmitBuilderPreferencesRequest) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "ValidatorServer.SubmitBuilderPreferences")
	defer span.End()

	if req == nil || req.Request == nil {
		return nil, status.Error(codes.InvalidArgument, "builder preferences request is empty")
	}
	if !validBuilderURL(req.Url) {
		return nil, status.Error(codes.InvalidArgument, "builder preferences request has a malformed builder url")
	}
	// Not gated on Configured(): gloas builders are dialed per URL rather than the endpoint flag.
	if vs.BlockBuilder == nil {
		return nil, status.Error(codes.FailedPrecondition, "builder is not configured")
	}
	pubkey := bytesutil.ToBytes48(req.ValidatorPubkey)
	if err := vs.BlockBuilder.SubmitBuilderPreferences(ctx, req.Url, pubkey, req.Request); err != nil {
		return nil, status.Errorf(codes.Internal, "could not submit builder preferences: %v", err)
	}
	return &emptypb.Empty{}, nil
}
