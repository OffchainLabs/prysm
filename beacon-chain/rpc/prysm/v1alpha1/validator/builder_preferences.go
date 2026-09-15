package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SubmitBuilderPreferences forwards a batch of per-builder preferences, each routed
// to its own url. A failing entry drops only itself; errors only if all entries fail.
func (vs *Server) SubmitBuilderPreferences(ctx context.Context, req *ethpb.SubmitBuilderPreferencesRequest) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "ValidatorServer.SubmitBuilderPreferences")
	defer span.End()

	if req == nil || len(req.Entries) == 0 {
		return nil, status.Error(codes.InvalidArgument, "builder preferences request is empty")
	}
	if vs.SyncChecker.Syncing() {
		return nil, status.Error(codes.Unavailable, "Syncing to latest head, not ready to respond")
	}
	// Not gated on Configured(), gloas builders are dialed per URL from the request rather than the endpoint flag.
	if vs.BlockBuilder == nil {
		return nil, status.Error(codes.FailedPrecondition, "builder is not configured")
	}
	failures := vs.BlockBuilder.SubmitBuilderPreferences(ctx, req.Entries)
	if len(failures) == len(req.Entries) {
		return nil, status.Error(codes.Unavailable, "could not submit builder preferences to any builder")
	}
	return &emptypb.Empty{}, nil
}
