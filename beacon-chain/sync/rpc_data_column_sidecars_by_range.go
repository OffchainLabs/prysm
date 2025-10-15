package sync

import (
	"context"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	pb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"

	"github.com/sirupsen/logrus"
)

// We count a single request as a single rate limiting amount, regardless of the number of columns requested.
const rateLimitingAmount = 1

var notDataColumnsByRangeIdentifiersError = errors.New("not data columns by range identifiers")

// dataColumnSidecarsByRangeRPCHandler looks up the request data columns from the database from a given start slot index
func (s *Service) dataColumnSidecarsByRangeRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.DataColumnSidecarsByRangeHandler")
	defer span.End()

	// Check if the message type is the one expected.
	request, ok := msg.(*pb.DataColumnSidecarsByRangeRequest)
	if !ok {
		return notDataColumnsByRangeIdentifiersError
	}

	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()

	SetRPCStreamDeadlines(stream)
	beaconConfig := params.BeaconConfig()
	maxRequestDataColumnSidecars := beaconConfig.MaxRequestDataColumnSidecars
	remotePeer := stream.Conn().RemotePeer()

	log := log.WithFields(logrus.Fields{
		"remotePeer": remotePeer,
		"startSlot":  request.StartSlot,
		"count":      request.Count,
	})

	if log.Logger.Level >= logrus.DebugLevel {
		slices.Sort(request.Columns)
		log = log.WithField("requestedColumns", helpers.PrettySlice(request.Columns))
	}

	// Validate the request regarding rate limiting.
	if err := s.rateLimiter.validateRequest(stream, rateLimitingAmount); err != nil {
		return errors.Wrap(err, "rate limiter validate request")
	}

	// Validate the request regarding its parameters.
	rangeParameters, err := validateDataColumnsByRange(request, s.cfg.clock.CurrentSlot())
	if err != nil {
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		s.downscorePeer(remotePeer, "dataColumnSidecarsByRangeRpcHandlerValidationError")
		tracing.AnnotateError(span, err)
		return errors.Wrap(err, "validate data columns by range")
	}

	log.Trace("Serving data column sidecars by range")

	if rangeParameters == nil {
		closeStream(stream, log)
		return nil
	}

	// Ticker to stagger out large requests.
	ticker := time.NewTicker(time.Second)

	batcher, err := newBlockRangeBatcher(*rangeParameters, s.cfg.beaconDB, s.rateLimiter, s.cfg.chain.IsCanonical, ticker)
	if err != nil {
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		return errors.Wrap(err, "new block range batcher")
	}

	// Derive the wanted columns for the request.
	wantedColumns := make([]uint64, len(request.Columns))
	copy(wantedColumns, request.Columns)

	// Sort the wanted columns.
	slices.Sort(wantedColumns)

	var batch blockBatch
	for batch, ok = batcher.next(ctx, stream); ok; batch, ok = batcher.next(ctx, stream) {
		batchStart := time.Now()
		maxRequestDataColumnSidecars, err = s.streamDataColumnBatch(ctx, batch, maxRequestDataColumnSidecars, wantedColumns, stream)
		rpcDataColumnsByRangeResponseLatency.Observe(float64(time.Since(batchStart).Milliseconds()))
		if err != nil {
			return err
		}

		// Once the quota is reached, we're done serving the request.
		if maxRequestDataColumnSidecars == 0 {
			log.WithField("initialQuota", beaconConfig.MaxRequestDataColumnSidecars).Trace("Reached quota for data column sidecars by range request")
			break
		}
	}

	if err := batch.error(); err != nil {
		log.WithError(err).Error("Cannot get next batch of blocks")

		// If we hit a rate limit, the error response has already been written, and the stream is already closed.
		if !errors.Is(err, p2ptypes.ErrRateLimited) {
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		}

		tracing.AnnotateError(span, err)
		return err
	}

	closeStream(stream, log)
	return nil
}

func (s *Service) streamDataColumnBatch(ctx context.Context, batch blockBatch, quota uint64, wantedDataColumnIndices []uint64, stream libp2pcore.Stream) (uint64, error) {
	_, span := trace.StartSpan(ctx, "sync.streamDataColumnBatch")
	defer span.End()

	// Defensive check to guard against underflow.
	if quota == 0 {
		return 0, nil
	}

	// Loop over the blocks in the batch.
	for _, block := range batch.canonical() {
		// Get the block blockRoot.
		blockRoot := block.Root()

		// Retrieve the data column sidecars from the store.
		verifiedRODataColumns, err := s.cfg.dataColumnStorage.Get(blockRoot, wantedDataColumnIndices)
		if err != nil {
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
			return quota, errors.Wrapf(err, "get data column sidecars: block root %#x", blockRoot)
		}

		// Write the retrieved sidecars to the stream.
		for _, verifiedRODataColumn := range verifiedRODataColumns {
			sidecar := verifiedRODataColumn.DataColumnSidecar
			SetStreamWriteDeadline(stream, defaultWriteDuration)

			if err := WriteDataColumnSidecarChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), sidecar); err != nil {
				s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
				tracing.AnnotateError(span, err)
				return quota, errors.Wrap(err, "write data column sidecar chunk")
			}

			s.rateLimiter.add(stream, rateLimitingAmount)
			quota -= 1

			// Stop streaming results once the quota of writes for the request is consumed.
			if quota == 0 {
				return 0, nil
			}
		}
	}

	return quota, nil
}

func validateDataColumnsByRange(request *pb.DataColumnSidecarsByRangeRequest, currentSlot primitives.Slot) (*rangeParams, error) {
	startSlot, count := request.StartSlot, request.Count

	if count == 0 {
		return nil, errors.Wrap(p2ptypes.ErrInvalidRequest, "invalid request count parameter")
	}

	endSlot, err := request.StartSlot.SafeAdd(count - 1)
	if err != nil {
		return nil, errors.Wrap(p2ptypes.ErrInvalidRequest, "overflow start + count -1")
	}

	// Peers may overshoot the current slot when in initial sync,
	// so we don't want to penalize them by treating the request as an error.
	if startSlot > currentSlot {
		return nil, nil
	}

	// Compute the oldest slot we'll allow a peer to request, based on the current slot.
	minStartSlot, err := dataColumnsRPCMinValidSlot(currentSlot)
	if err != nil {
		return nil, errors.Wrap(p2ptypes.ErrInvalidRequest, "data columns RPC min valid slot")
	}

	// Return early if there is nothing to serve.
	if endSlot < minStartSlot {
		return nil, nil
	}

	// Do not serve sidecars for slots before the minimum valid slot or after the current slot.
	startSlot = max(startSlot, minStartSlot)
	endSlot = min(endSlot, currentSlot)

	sizeMinusOne, err := endSlot.SafeSub(uint64(startSlot))
	if err != nil {
		return nil, errors.Errorf("overflow end - start: %d - %d - should never happen", endSlot, startSlot)
	}

	size, err := sizeMinusOne.SafeAdd(1)
	if err != nil {
		return nil, errors.Wrap(p2ptypes.ErrInvalidRequest, "overflow end - start + 1")
	}

	rangeParameters := &rangeParams{start: startSlot, end: endSlot, size: uint64(size)}
	return rangeParameters, nil
}
