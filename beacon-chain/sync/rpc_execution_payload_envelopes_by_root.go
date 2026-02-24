package sync

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
)

// executionPayloadEnvelopesByRootRPCHandler handles the
// /eth2/beacon_chain/req/execution_payload_envelopes_by_root/1/ RPC request.
// spec: https://github.com/ethereum/consensus-specs/blob/master/specs/gloas/p2p-interface.md#executionpayloadenvelopesbyroot-v1
func (s *Service) executionPayloadEnvelopesByRootRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.executionPayloadEnvelopesByRootRPCHandler")
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.ExecutionPayloadEnvelopesByRootName[1:]) // slice the leading slash off the name var

	ref, ok := msg.(*types.ExecutionPayloadEnvelopesByRootReq)
	if !ok {
		return errors.New("message is not type ExecutionPayloadEnvelopesByRootReq")
	}

	requestedRoots := *ref

	if err := s.rateLimiter.validateRequest(stream, uint64(len(requestedRoots))); err != nil {
		return errors.Wrap(err, "rate limiter validate request")
	}

	remotePeer := stream.Conn().RemotePeer()
	if err := validateExecutionPayloadEnvelopeByRootRequest(len(requestedRoots)); err != nil {
		s.downscorePeer(remotePeer, "executionPayloadEnvelopesByRootRPCHandlerValidationError")
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		return err
	}

	// Compute the oldest slot we'll allow a peer to request, based on the finalized epoch.
	finalized := s.cfg.chain.FinalizedCheckpt()
	minReqSlot, err := slots.EpochStart(finalized.Epoch)
	if err != nil {
		return errors.Wrapf(err, "could not compute start slot for finalized epoch %d", finalized.Epoch)
	}

	batchSize := flags.Get().BlockBatchLimit
	var ticker *time.Ticker
	if len(requestedRoots) > batchSize {
		ticker = time.NewTicker(time.Second)
	}

	defer closeStream(stream, log)

	for i, root := range requestedRoots {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Throttle request processing.
		if i != 0 && i%batchSize == 0 && ticker != nil {
			<-ticker.C
		}
		s.rateLimiter.add(stream, 1)

		// Look up the full envelope from the in-memory cache.
		// TODO: Add a fallback DB lookup path once persistent full-envelope storage is implemented.
		// Currently only the gossip-populated cache is checked, which means envelopes may be
		// unavailable after restart, during initial sync, or after LRU eviction.
		val, ok := s.executionPayloadEnvelopeCache.Get(root)
		if !ok {
			log.WithField("root", root).Trace("Peer requested execution payload envelope by root not found in cache")
			continue
		}

		envelope, ok := val.(*ethpb.SignedExecutionPayloadEnvelope)
		if !ok || envelope == nil || envelope.Message == nil {
			continue
		}

		// Filter out envelopes that are too old.
		if envelope.Message.Slot < minReqSlot {
			continue
		}

		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if chunkErr := WriteExecutionPayloadEnvelopeChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), envelope); chunkErr != nil {
			log.WithError(chunkErr).Debug("Could not send a chunked response")
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			tracing.AnnotateError(span, chunkErr)
			return chunkErr
		}
	}

	return nil
}

// validateExecutionPayloadEnvelopeByRootRequest checks if the request for execution payload envelopes is valid.
func validateExecutionPayloadEnvelopeByRootRequest(count int) error {
	if uint64(count) > params.BeaconConfig().MaxRequestPayloads {
		return types.ErrMaxPayloadEnvelopeReqExceeded
	}
	return nil
}
