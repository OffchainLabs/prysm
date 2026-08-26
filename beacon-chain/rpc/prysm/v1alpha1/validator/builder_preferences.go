package validator

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/io/logs"
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
	// Not gated on Configured(), gloas builders are dialed per URL from the request rather than the endpoint flag.
	if vs.BlockBuilder == nil {
		return nil, status.Error(codes.FailedPrecondition, "builder is not configured")
	}
	var wg sync.WaitGroup
	var failed atomic.Uint64
	for _, e := range req.Entries {
		if len(e.GetUrl()) == 0 {
			log.Warn("Skipping builder preferences entry with no builder url")
			failed.Add(1)
			continue
		}
		attempted++
		wg.Add(1)
		go func(e *ethpb.BuilderPreferencesEntry) {
			defer wg.Done()
			breq := &ethpb.BuilderPreferencesRequest{
				Preferences: &ethpb.BuilderPreferences{MaxExecutionPayment: e.MaxExecutionPayment},
				Auth:        e.Auth,
			}
			url := string(e.Url)
			if err := vs.BlockBuilder.SubmitBuilderPreferences(ctx, bytesutil.ToBytes48(e.ProposerPubkey), url, breq); err != nil {
				log.WithError(err).WithField("builder", logs.MaskCredentialsLogging(url)).Warn("Could not submit builder preferences")
				failed.Add(1)
			}
		}(e)
	}
	wg.Wait()
	if n := failed.Load(); n > 0 {
		return nil, status.Errorf(codes.InvalidArgument, "%d of %d builder preference submissions failed", n, len(req.Entries))
	}
	return &emptypb.Empty{}, nil
}
