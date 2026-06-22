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

// SubmitBuilderPreferences forwards a proposer's builder preferences to the configured builder.
func (vs *Server) SubmitBuilderPreferences(ctx context.Context, req *ethpb.SubmitBuilderPreferencesRequest) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "ValidatorServer.SubmitBuilderPreferences")
	defer span.End()

	if req == nil || req.Request == nil {
		return nil, status.Error(codes.InvalidArgument, "builder preferences request is empty")
	}
	if vs.BlockBuilder == nil || !vs.BlockBuilder.Configured() {
		return nil, status.Error(codes.FailedPrecondition, "builder is not configured")
	}
	pubkey := bytesutil.ToBytes48(req.ValidatorPubkey)
	if err := vs.BlockBuilder.SubmitBuilderPreferences(ctx, pubkey, req.Request); err != nil {
		return nil, status.Errorf(codes.Internal, "could not submit builder preferences: %v", err)
	}
	vs.maxExecutionPayments.Store(pubkey, uint64(req.Request.Preferences.GetMaxExecutionPayment()))
	return &emptypb.Empty{}, nil
}
