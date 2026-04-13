package sync

import (
	"context"
	"math"
	"slices"

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

// executionPayloadEnvelopesByRangeRPCHandler looks up the request execution payload envelopes from
// the database for the given slot range, serving one envelope per canonical block slot.
func (s *Service) executionPayloadEnvelopesByRangeRPCHandler(ctx context.Context, msg any, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.ExecutionPayloadEnvelopesByRangeHandler")
	defer span.End()
	recordResult := func(result executionPayloadEnvelopeRPCResult) {
		gloasExecutionPayloadEnvelopesRPCRequestsTotal.WithLabelValues("by_range", string(result)).Inc()
		if result == executionPayloadEnvelopeRPCResultServed {
			syncPayloadEnvelopeByRangeServedTotal.Inc()
		}
	}
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.ExecutionPayloadEnvelopesByRangeName[1:])

	r, ok := msg.(*pb.ExecutionPayloadEnvelopesByRangeRequest)
	if !ok {
		recordResult(executionPayloadEnvelopeRPCResultInvalid)
		return errors.New("message is not type *pb.ExecutionPayloadEnvelopesByRangeRequest")
	}
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		recordResult(executionPayloadEnvelopeRPCResultRateLimited)
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
		recordResult(executionPayloadEnvelopeRPCResultInvalid)
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		s.downscorePeer(remotePeer, "executionPayloadEnvelopesByRangeRPCHandlerValidationError")
		tracing.AnnotateError(span, err)
		return err
	}
	available := s.validateRangeAvailability(rp)
	if !available {
		recordResult(executionPayloadEnvelopeRPCResultResourceUnavailable)
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

	if err := s.streamCanonicalEnvelopes(ctx, rp, stream); err != nil {
		recordResult(executionPayloadEnvelopeRPCResultError)
		tracing.AnnotateError(span, err)
		return err
	}

	recordResult(executionPayloadEnvelopeRPCResultServed)
	closeStream(stream, log)
	return nil
}

// streamCanonicalEnvelopes walks the canonical payload chain backwards from the successor of rp.end
// to rp.start, collecting only envelopes whose payloads were actually included in the canonical chain.
func (s *Service) streamCanonicalEnvelopes(ctx context.Context, rp rangeParams, stream libp2pcore.Stream) error {
	_, span := trace.StartSpan(ctx, "sync.streamCanonicalEnvelopes")
	defer span.End()
	if s.cfg.executionReconstructor == nil {
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		return errors.New("execution reconstructor is nil")
	}

	type collectedEnvelope struct {
		env       *pb.SignedBlindedExecutionPayloadEnvelope
		blockHash [32]byte
	}

	_, roots, err := s.cfg.beaconDB.LowestRootsAtOrAboveSlot(ctx, rp.end+1)
	if err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		return errors.Wrap(err, "could not find successor block")
	}
	if len(roots) == 0 {
		return nil
	}

	var successorRoot [32]byte
	var found bool
	for _, r := range roots {
		canonical, err := s.cfg.chain.IsCanonical(ctx, r)
		if err != nil {
			log.WithError(err).WithField("blockRoot", bytesutil.Trunc(r[:])).Debug("Could not check if block is canonical")
			continue
		}
		if canonical {
			successorRoot = r
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	successorBlock, err := s.cfg.beaconDB.Block(ctx, successorRoot)
	if err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		return errors.Wrap(err, "could not load successor block")
	}
	bid, err := successorBlock.Block().Body().SignedExecutionPayloadBid()
	if err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		return errors.Wrap(err, "could not get bid from successor block")
	}
	parentBlockHash := bytesutil.ToBytes32(bid.Message.ParentBlockHash)

	wQuota := params.BeaconConfig().MaxRequestPayloads
	var collected []collectedEnvelope

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		blindedEnv, err := s.cfg.beaconDB.ExecutionPayloadEnvelopeByBlockHash(ctx, parentBlockHash)
		if err != nil {
			log.WithError(err).WithField("blockHash", bytesutil.Trunc(parentBlockHash[:])).Debug("Could not load execution payload envelope")
			break
		}

		if blindedEnv.Message.Slot < rp.start {
			break
		}

		collected = append(collected, collectedEnvelope{
			env:       blindedEnv,
			blockHash: parentBlockHash,
		})
		if uint64(len(collected)) >= wQuota {
			break
		}

		parentBlockHash = bytesutil.ToBytes32(blindedEnv.Message.ParentBlockHash)
	}

	if len(collected) == 0 {
		return nil
	}

	slices.Reverse(collected)

	batchHashes := make([][32]byte, 0, len(collected))
	for _, c := range collected {
		batchHashes = append(batchHashes, c.blockHash)
	}

	payloadByHash, err := s.cfg.executionReconstructor.ReconstructFullExecutionPayloadsByHash(ctx, batchHashes)
	if err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		return errors.Wrap(err, "could not batch reconstruct full execution payload envelopes")
	}

	for _, c := range collected {
		payload := payloadByHash[c.blockHash]
		if payload == nil {
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
			return errors.Errorf("missing reconstructed payload for block hash %#x", c.blockHash)
		}
		fullEnv := &pb.SignedExecutionPayloadEnvelope{
			Message: &pb.ExecutionPayloadEnvelope{
				Payload:           payload,
				ExecutionRequests: c.env.Message.ExecutionRequests,
				BuilderIndex:      c.env.Message.BuilderIndex,
				BeaconBlockRoot:   c.env.Message.BeaconBlockRoot,
				Slot:              c.env.Message.Slot,
			},
			Signature: c.env.Signature,
		}

		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if chunkErr := WriteExecutionPayloadEnvelopeChunk(stream, s.cfg.p2p.Encoding(), fullEnv); chunkErr != nil {
			log.WithError(chunkErr).Debug("Could not send execution payload envelope chunk")
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
			tracing.AnnotateError(span, chunkErr)
			return chunkErr
		}
		s.rateLimiter.add(stream, 1)
		wQuota -= 1
		if wQuota == 0 {
			break
		}
	}
	return nil
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
