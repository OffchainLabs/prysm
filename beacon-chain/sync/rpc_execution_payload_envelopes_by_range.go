package sync

import (
	"context"
	"math"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	p2ptypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var envelopeRpcThrottleInterval = time.Second

// executionPayloadEnvelopesByRangeRPCHandler looks up the request execution payload envelopes from
// the database for the given slot range, serving one envelope per canonical block slot.
func (s *Service) executionPayloadEnvelopesByRangeRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.ExecutionPayloadEnvelopesByRangeHandler")
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.ExecutionPayloadEnvelopesByRangeName[1:])

	r, ok := msg.(*pb.ExecutionPayloadEnvelopesByRangeRequest)
	if !ok {
		return errors.New("message is not type *pb.ExecutionPayloadEnvelopesByRangeRequest")
	}
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		return err
	}

	remotePeer := stream.Conn().RemotePeer()

	log.WithFields(logrus.Fields{
		"startSlot": r.StartSlot,
		"count":     r.Count,
		"peer":      remotePeer,
	}).Debug("Serving execution payload envelopes by range request")

	rp, err := validateEnvelopesByRange(r, s.cfg.clock.CurrentSlot())
	if err != nil {
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		s.downscorePeer(remotePeer, "executionPayloadEnvelopesByRangeRPCHandlerValidationError")
		tracing.AnnotateError(span, err)
		return err
	}
	available := s.validateRangeAvailability(rp)
	if !available {
		currentSlot := s.cfg.clock.CurrentSlot()
		unavailableErr := errors.Wrapf(
			p2ptypes.ErrResourceUnavailable,
			"execution payload envelope range unavailable start=%d end=%d current=%d",
			rp.start,
			rp.end,
			currentSlot,
		)
		log.WithFields(logrus.Fields{
			"startSlot": rp.start,
			"endSlot":   rp.end,
			"size":      rp.size,
			"current":   currentSlot,
		}).WithError(unavailableErr).Debug("Execution payload envelope range unavailable")
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, p2ptypes.ErrResourceUnavailable.Error(), stream)
		tracing.AnnotateError(span, unavailableErr)
		return nil
	}

	// Ticker to stagger out large requests.
	ticker := time.NewTicker(envelopeRpcThrottleInterval)
	defer ticker.Stop()
	batcher, err := newBlockRangeBatcher(rp, s.cfg.beaconDB, s.rateLimiter, s.cfg.chain.IsCanonical, ticker)
	if err != nil {
		log.WithError(err).Error("Cannot create new block range batcher for envelopes")
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		return err
	}

	var batch blockBatch
	var more bool
	// wQuota caps total envelopes sent per request, bounded by MAX_REQUEST_PAYLOADS.
	wQuota := params.BeaconConfig().MaxRequestPayloads
	for batch, more = batcher.next(ctx, stream); more; batch, more = batcher.next(ctx, stream) {
		wQuota, err = s.streamEnvelopeBatch(ctx, batch, wQuota, stream)
		if err != nil {
			return err
		}
		if wQuota == 0 {
			break
		}
	}

	if err := batch.error(); err != nil {
		log.WithError(err).Debug("Error in ExecutionPayloadEnvelopesByRange batch")
		if !errors.Is(err, p2ptypes.ErrRateLimited) {
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		}
		tracing.AnnotateError(span, err)
		return err
	}

	closeStream(stream, log)
	return nil
}

// streamEnvelopeBatch sends all available envelopes for the canonical blocks in the batch.
// It returns the remaining write quota and any error encountered.
func (s *Service) streamEnvelopeBatch(ctx context.Context, batch blockBatch, wQuota uint64, stream libp2pcore.Stream) (uint64, error) {
	if wQuota == 0 {
		return 0, nil
	}
	_, span := trace.StartSpan(ctx, "sync.streamEnvelopeBatch")
	defer span.End()
	if s.cfg.executionReconstructor == nil {
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		return wQuota, errors.New("execution reconstructor is nil")
	}

	type requestedEnvelope struct {
		root      [32]byte
		env       *pb.SignedBlindedExecutionPayloadEnvelope
		blockHash [32]byte
	}

	limit := min(uint64(len(batch.canonical())), wQuota)
	requestedEnvs := make([]requestedEnvelope, 0, limit)
	batchHashes := make([][32]byte, 0, limit)
	hashSeen := make(map[[32]byte]struct{}, limit)

	for _, b := range batch.canonical() {
		if uint64(len(requestedEnvs)) >= wQuota {
			break
		}
		root := b.Root()
		if !s.cfg.beaconDB.HasExecutionPayloadEnvelope(ctx, root) {
			// No envelope for this slot — spec allows gaps, just skip.
			continue
		}
		blindedEnv, err := s.cfg.beaconDB.ExecutionPayloadEnvelope(ctx, root)
		if err != nil {
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
			return wQuota, errors.Wrapf(err, "could not retrieve execution payload envelope for root %#x", root)
		}
		if blindedEnv == nil || blindedEnv.Message == nil {
			continue
		}

		blockHash := bytesutil.ToBytes32(blindedEnv.Message.BlockHash)
		requestedEnvs = append(requestedEnvs, requestedEnvelope{
			root:      root,
			env:       blindedEnv,
			blockHash: blockHash,
		})
		if _, ok := hashSeen[blockHash]; !ok {
			hashSeen[blockHash] = struct{}{}
			batchHashes = append(batchHashes, blockHash)
		}
	}

	if len(requestedEnvs) == 0 {
		return wQuota, nil
	}

	payloadByHash, err := s.cfg.executionReconstructor.ReconstructFullExecutionPayloadsByHash(ctx, batchHashes)
	if err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		return wQuota, errors.Wrap(err, "could not batch reconstruct full execution payload envelopes")
	}

	for _, req := range requestedEnvs {
		payload := payloadByHash[req.blockHash]
		if payload == nil {
			log.WithField("root", bytesutil.Trunc(req.root[:])).Debug("Missing reconstructed payload after successful batch call")
			continue
		}
		fullEnv := &pb.SignedExecutionPayloadEnvelope{
			Message: &pb.ExecutionPayloadEnvelope{
				Payload:           payload,
				ExecutionRequests: req.env.Message.ExecutionRequests,
				BuilderIndex:      req.env.Message.BuilderIndex,
				BeaconBlockRoot:   req.env.Message.BeaconBlockRoot,
				Slot:              req.env.Message.Slot,
				StateRoot:         req.env.Message.StateRoot,
			},
			Signature: req.env.Signature,
		}

		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if chunkErr := WriteExecutionPayloadEnvelopeChunk(stream, s.cfg.p2p.Encoding(), fullEnv); chunkErr != nil {
			log.WithError(chunkErr).Debug("Could not send execution payload envelope chunk")
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
			tracing.AnnotateError(span, chunkErr)
			return wQuota, chunkErr
		}
		s.rateLimiter.add(stream, 1)
		wQuota -= 1
		if wQuota == 0 {
			return 0, nil
		}
	}
	return wQuota, nil
}

// validateEnvelopesByRange validates the ExecutionPayloadEnvelopesByRange request and returns
// normalized rangeParams. Mirrors validateBlobsByRange in structure.
func validateEnvelopesByRange(r *pb.ExecutionPayloadEnvelopesByRangeRequest, current primitives.Slot) (rangeParams, error) {
	if r.Count == 0 {
		return rangeParams{}, errors.Wrap(p2ptypes.ErrInvalidRequest, "invalid request Count parameter")
	}
	rp := rangeParams{
		start: r.StartSlot,
		size:  r.Count,
	}
	// Peers may overshoot the current slot when in initial sync — treat as noop rather than error.
	if rp.start > current {
		return rangeParams{start: current, end: current, size: 0}, nil
	}

	var err error
	rp.end, err = rp.start.SafeAdd(rp.size - 1)
	if err != nil {
		return rangeParams{}, errors.Wrap(p2ptypes.ErrInvalidRequest, "overflow start + count - 1")
	}

	// Envelopes only exist from the Gloas fork onward — clamp start if needed.
	if params.BeaconConfig().GloasForkEpoch != math.MaxUint64 {
		gloasStart, err := slots.EpochStart(params.BeaconConfig().GloasForkEpoch)
		if err != nil {
			return rangeParams{}, errors.Wrap(p2ptypes.ErrInvalidRequest, "could not compute Gloas fork start slot")
		}
		if rp.start < gloasStart {
			rp.start = gloasStart
		}
	}

	if rp.end > current {
		rp.end = current
	}
	if rp.end < rp.start {
		rp.end = rp.start
	}
	maxRequest := params.BeaconConfig().MaxRequestPayloads
	if rp.size > maxRequest {
		rp.size = maxRequest
	}

	return rp, nil
}
