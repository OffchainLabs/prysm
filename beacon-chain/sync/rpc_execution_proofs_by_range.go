package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/sirupsen/logrus"
)

// executionProofsRpcThrottleInterval staggers large range responses.
const executionProofsRpcThrottleInterval = time.Second

// executionProofsByRangeRPCHandler handles incoming ExecutionProofsByRange RPC
// requests per EIP-8025. It iterates canonical blocks in the requested slot
// range and streams every stored SignedExecutionProof, up to the spec cap of
// MAX_REQUEST_BLOCKS_DENEB * MAX_EXECUTION_PROOFS_PER_PAYLOAD chunks.
// Per the spec, no <context-bytes> are written.
// spec: https://github.com/ethereum/consensus-specs/blob/master/specs/_features/eip8025/p2p-interface.md#executionproofsbyrange
func (s *Service) executionProofsByRangeRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.executionProofsByRangeRPCHandler")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.ExecutionProofsByRangeName[1:]) // drop leading slash

	req, ok := msg.(*p2ptypes.ExecutionProofsByRangeReq)
	if !ok {
		return errors.New("message is not type *ExecutionProofsByRangeReq")
	}

	// Charge one rate-limit token per request, matching ExecutionProofsByRoot.
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		return fmt.Errorf("validator request: %w", err)
	}
	s.rateLimiter.add(stream, 1)
	defer closeStream(stream, log)

	remotePeer := stream.Conn().RemotePeer()
	log = log.WithFields(logrus.Fields{
		"peer":      remotePeer.String(),
		"startSlot": req.StartSlot,
		"count":     req.Count,
	})
	log.Debug("Received execution proofs by range request")

	rp, err := validateExecutionProofsByRange(req, s.cfg.clock.CurrentSlot())
	if err != nil {
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		s.downscorePeer(remotePeer, "executionProofsByRangeValidationError")
		tracing.AnnotateError(span, err)
		return err
	}

	// Stream proofs up to the spec-defined per-request cap.
	cfg := params.BeaconConfig()
	wQuota := cfg.MaxRequestBlocksDeneb * cfg.MaxExecutionProofsPerPayload

	// Use the existing block-range batcher (canonical-only iteration) so the
	// response follows slot-ascending order.
	ticker := time.NewTicker(executionProofsRpcThrottleInterval)
	defer ticker.Stop()
	batcher, err := newBlockRangeBatcher(rp, s.cfg.beaconDB, s.rateLimiter, s.cfg.chain.IsCanonical, ticker)
	if err != nil {
		log.WithError(err).Error("Cannot create new block range batcher")
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		return err
	}

	sent := uint64(0)
	var batch blockBatch
	for batch, ok = batcher.next(ctx, stream); ok; batch, ok = batcher.next(ctx, stream) {
		var streamErr error
		wQuota, streamErr = s.streamExecutionProofsBatch(ctx, batch, wQuota, stream)
		if streamErr != nil {
			return streamErr
		}
		sent = (cfg.MaxRequestBlocksDeneb * cfg.MaxExecutionProofsPerPayload) - wQuota
		if wQuota == 0 {
			break
		}
	}
	if err := batch.error(); err != nil {
		log.WithError(err).Debug("Error in ExecutionProofsByRange batch")
		if !errors.Is(err, p2ptypes.ErrRateLimited) {
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		}
		tracing.AnnotateError(span, err)
		return err
	}

	log.WithField("proofCount", sent).Debug("Sent execution proofs by range response")
	return nil
}

// streamExecutionProofsBatch writes every stored SignedExecutionProof for the
// canonical blocks in the given batch, decrementing the per-request quota.
func (s *Service) streamExecutionProofsBatch(ctx context.Context, batch blockBatch, wQuota uint64, stream libp2pcore.Stream) (uint64, error) {
	if wQuota == 0 {
		return 0, nil
	}
	_, span := trace.StartSpan(ctx, "sync.streamExecutionProofsBatch")
	defer span.End()

	for _, b := range batch.canonical() {
		root := b.Root()

		summary := s.cfg.proofStorage.Summary(root)
		if summary.Count() == 0 {
			continue
		}

		proofs, err := s.cfg.proofStorage.Get(root, nil /* nil = all stored proof types */)
		if err != nil {
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
			return wQuota, fmt.Errorf("proof storage get for %#x: %w", root, err)
		}

		for _, proof := range proofs {
			SetStreamWriteDeadline(stream, defaultWriteDuration)
			if err := WriteExecutionProofChunk(stream, s.cfg.p2p.Encoding(), proof); err != nil {
				s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
				return wQuota, fmt.Errorf("write execution proof chunk: %w", err)
			}
			wQuota--
			if wQuota == 0 {
				return 0, nil
			}
		}
	}
	return wQuota, nil
}

// validateExecutionProofsByRange enforces the spec bounds and produces range
// params compatible with newBlockRangeBatcher.
func validateExecutionProofsByRange(req *p2ptypes.ExecutionProofsByRangeReq, current primitives.Slot) (rangeParams, error) {
	if req.Count == 0 {
		return rangeParams{}, fmt.Errorf("%w: invalid request Count parameter", p2ptypes.ErrInvalidRequest)
	}
	// `count * MAX_EXECUTION_PROOFS_PER_PAYLOAD <= compute_max_request_execution_proofs()`
	// simplifies to `count <= MAX_REQUEST_BLOCKS_DENEB`.
	if req.Count > params.BeaconConfig().MaxRequestBlocksDeneb {
		return rangeParams{}, fmt.Errorf("%w: count %d exceeds MAX_REQUEST_BLOCKS_DENEB=%d", p2ptypes.ErrInvalidRequest, req.Count, params.BeaconConfig().MaxRequestBlocksDeneb)
	}

	rp := rangeParams{
		start: req.StartSlot,
		size:  req.Count,
	}
	// A start slot past the current slot is treated as a no-op, matching the
	// blob-by-range behavior: peers in initial sync may overshoot and shouldn't
	// be penalized.
	if rp.start > current {
		return rangeParams{start: current, end: current, size: 0}, nil
	}

	end, err := rp.start.SafeAdd(rp.size - 1)
	if err != nil {
		return rangeParams{}, fmt.Errorf("%w: start + count - 1 overflow", p2ptypes.ErrInvalidRequest)
	}
	rp.end = end
	if rp.end > current {
		rp.end = current
	}
	if rp.end < rp.start {
		rp.end = rp.start
	}
	return rp, nil
}

