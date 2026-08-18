package validator

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/logs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SubmitBuilderPreferences forwards a batch of per-builder preferences, each entry
// routed to its own url. A failing entry drops only its own submission.
func (vs *Server) SubmitBuilderPreferences(ctx context.Context, req *ethpb.SubmitBuilderPreferencesRequest) (*emptypb.Empty, error) {
	ctx, span := trace.StartSpan(ctx, "ValidatorServer.SubmitBuilderPreferences")
	defer span.End()

	if req == nil || len(req.Entries) == 0 {
		return nil, status.Error(codes.InvalidArgument, "builder preferences request is empty")
	}
	// Not gated on Configured(), gloas builders are dialed per URL from the request rather than the endpoint flag.
	if vs.BlockBuilder == nil {
		return nil, status.Error(codes.FailedPrecondition, "builder is not configured")
	}
	for _, e := range req.Entries {
		if e.GetUrl() == "" {
			log.Warn("Skipping builder preferences entry with no builder url")
			continue
		}
		pubkey := bytesutil.ToBytes48(e.ProposerPubkey)
		breq := &ethpb.BuilderPreferencesRequest{
			Preferences: &ethpb.BuilderPreferences{MaxExecutionPayment: e.MaxExecutionPayment},
			Auth:        e.Auth,
		}
		if err := vs.BlockBuilder.SubmitBuilderPreferences(ctx, pubkey, e.Url, breq); err != nil {
			log.WithError(err).WithField("builder", logs.MaskCredentialsLogging(e.Url)).Warn("Could not submit builder preferences")
			continue
		}
		vs.maxExecutionPayments.Store(pubkey, uint64(e.MaxExecutionPayment))
	}
	return &emptypb.Empty{}, nil
}
