package sync

import (
	"context"
	"time"

	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// beaconBlocksByRangeRPCHandler looks up the request blocks from the database from a given start block.
func (s *Service) beaconBlocksByRangeRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.BeaconBlocksByRangeHandler")
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)

	remotePeer := stream.Conn().RemotePeer()

	m, ok := msg.(*pb.BeaconBlocksByRangeRequest)
	if !ok {
		return errors.New("message is not type *pb.BeaconBlockByRangeRequest")
	}

	log.WithFields(logrus.Fields{
		"startSlot": m.StartSlot,
		"count":     m.Count,
		"peer":      remotePeer,
	}).Debug("Serving block by range request")

	rp, err := validateRangeRequest(m, s.cfg.clock.CurrentSlot())
	if err != nil {
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		s.downscorePeer(remotePeer, "beaconBlocksByRangeRPCHandlerValidationError")
		tracing.AnnotateError(span, err)
		return err
	}
	available := s.validateRangeAvailability(rp)
	if !available {
		log.WithFields(logrus.Fields{
			"startSlot": rp.start,
			"endSlot":   rp.end,
			"size":      rp.size,
			"current":   s.cfg.clock.CurrentSlot(),
		}).Debug("Error in validating range availability")
		s.writeErrorResponseToStream(responseCodeResourceUnavailable, p2ptypes.ErrResourceUnavailable.Error(), stream)
		tracing.AnnotateError(span, err)
		return nil
	}

	blockLimiter, err := s.rateLimiter.topicCollector(string(stream.Protocol()))
	if err != nil {
		return err
	}
	remainingBucketCapacity := blockLimiter.Remaining(remotePeer.String())
	span.SetAttributes(
		trace.Int64Attribute("start", int64(rp.start)), // lint:ignore uintcast -- This conversion is OK for tracing.
		trace.Int64Attribute("end", int64(rp.end)),     // lint:ignore uintcast -- This conversion is OK for tracing.
		trace.Int64Attribute("count", int64(m.Count)),
		trace.StringAttribute("peer", remotePeer.String()),
		trace.Int64Attribute("remaining_capacity", remainingBucketCapacity),
	)

	// Ticker to stagger out large requests.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	batcher, err := newBlockRangeBatcher(rp, s.cfg.beaconDB, s.rateLimiter, s.cfg.chain.IsCanonical, ticker)
	if err != nil {
		log.WithError(err).Info("Error in BlocksByRange batch")
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		return err
	}

	// prevRoot is used to ensure that returned chains are strictly linear for singular steps
	// by comparing the previous root of the block in the list with the current block's parent.
	var batch blockBatch
	var more bool
	for batch, more = batcher.next(ctx, stream); more; batch, more = batcher.next(ctx, stream) {
		batchStart := time.Now()
		if err := s.writeBlockBatchToStream(ctx, batch, stream); err != nil {
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
			return err
		}
		rpcBlocksByRangeResponseLatency.Observe(float64(time.Since(batchStart).Milliseconds()))
	}

	if err := batch.error(); err != nil {
		log.WithError(err).Debug("Serving block by range request - BlocksByRange batch")

		// If a rate limit is hit, it means an error response has already been sent and the stream has been closed.
		if !errors.Is(err, p2ptypes.ErrRateLimited) {
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		}

		tracing.AnnotateError(span, err)
		return err
	}

	closeStream(stream, log)
	return nil
}

type rangeParams struct {
	start primitives.Slot
	end   primitives.Slot
	size  uint64
}

func validateRangeRequest(r *pb.BeaconBlocksByRangeRequest, current primitives.Slot) (rangeParams, error) {
	rp := rangeParams{
		start: r.StartSlot,
		size:  r.Count,
	}
	maxRequest := params.MaxRequestBlock(slots.ToEpoch(current))
	// Ensure all request params are within appropriate bounds
	if rp.size == 0 || rp.size > maxRequest {
		return rangeParams{}, p2ptypes.ErrInvalidRequest
	}
	// Allow some wiggle room, up to double the MaxRequestBlocks past the current slot,
	// to give nodes syncing close to the head of the chain some margin for error.
	maxStart, err := current.SafeAdd(maxRequest * 2)
	if err != nil {
		return rangeParams{}, p2ptypes.ErrInvalidRequest
	}
	if rp.start > maxStart {
		return rangeParams{}, p2ptypes.ErrInvalidRequest
	}
	rp.end, err = rp.start.SafeAdd(rp.size - 1)
	if err != nil {
		return rangeParams{}, p2ptypes.ErrInvalidRequest
	}

	limit := uint64(flags.Get().BlockBatchLimit)
	if limit > maxRequest {
		limit = maxRequest
	}
	if rp.size > limit {
		rp.size = limit
	}

	return rp, nil
}

func (s *Service) validateRangeAvailability(rp rangeParams) bool {
	startBlock := rp.start
	return s.availableBlocker.AvailableBlock(startBlock)
}

func (s *Service) writeBlockBatchToStream(ctx context.Context, batch blockBatch, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.WriteBlockRangeToStream")
	defer span.End()

	blinded := make([]interfaces.ReadOnlySignedBeaconBlock, 0)
	for _, b := range batch.canonical() {
		if err := blocks.BeaconBlockIsNil(b); err != nil {
			continue
		}
		if b.IsBlinded() {
			blinded = append(blinded, b.ReadOnlySignedBeaconBlock)
			continue
		}
		if chunkErr := s.chunkBlockWriter(stream, b); chunkErr != nil {
			log.WithError(chunkErr).Debug("Could not send a chunked response")
			return chunkErr
		}
	}
	if len(blinded) == 0 {
		return nil
	}

	reconstructed, err := s.cfg.executionReconstructor.ReconstructFullBellatrixBlockBatch(ctx, blinded)
	if err != nil {
		log.WithError(err).Error("Could not reconstruct full bellatrix block batch from blinded bodies")
		return err
	}
	for _, b := range reconstructed {
		if err := blocks.BeaconBlockIsNil(b); err != nil {
			continue
		}
		if b.IsBlinded() {
			continue
		}
		if chunkErr := s.chunkBlockWriter(stream, b); chunkErr != nil {
			log.WithError(chunkErr).Debug("Could not send a chunked response")
			return chunkErr
		}
	}

	return nil
}

func (s *Service) downscorePeer(peerID peer.ID, reason string, fields ...logrus.Fields) {
	log := log
	for _, field := range fields {
		log = log.WithFields(field)
	}

	newScore := s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(peerID)
	log.WithFields(logrus.Fields{"peerID": peerID, "reason": reason, "newScore": newScore}).Debug("Downscore peer")
}
