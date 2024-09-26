package sync

import (
	"context"
	"time"

	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2ptypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func (s *Service) streamDataColumnBatch(ctx context.Context, batch blockBatch, wQuota uint64, wantedIndexes map[uint64]bool, stream libp2pcore.Stream) (uint64, error) {
	// Defensive check to guard against underflow.
	if wQuota == 0 {
		return 0, nil
	}
	_, span := trace.StartSpan(ctx, "sync.streamDataColumnBatch")
	defer span.End()
	for _, b := range batch.canonical() {
		root := b.Root()
		idxs, err := s.cfg.blobStorage.ColumnIndices(b.Root())
		if err != nil {
			s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
			return wQuota, errors.Wrapf(err, "could not retrieve sidecars for block root %#x", root)
		}
		for i, l := uint64(0), uint64(len(idxs)); i < l; i++ {
			// index not available or unwanted, skip
			if !idxs[i] || !wantedIndexes[i] {
				continue
			}
			// We won't check for file not found since the .Indices method should normally prevent that from happening.
			sc, err := s.cfg.blobStorage.GetColumn(b.Root(), i)
			if err != nil {
				s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
				return wQuota, errors.Wrapf(err, "could not retrieve data column sidecar: index %d, block root %#x", i, root)
			}
			SetStreamWriteDeadline(stream, defaultWriteDuration)
			if chunkErr := WriteDataColumnSidecarChunk(stream, s.cfg.chain, s.cfg.p2p.Encoding(), sc); chunkErr != nil {
				log.WithError(chunkErr).Debug("Could not send a chunked response")
				s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
				tracing.AnnotateError(span, chunkErr)
				return wQuota, chunkErr
			}
			s.rateLimiter.add(stream, 1)
			wQuota -= 1
			// Stop streaming results once the quota of writes for the request is consumed.
			if wQuota == 0 {
				return 0, nil
			}
		}
	}
	return wQuota, nil
}

// dataColumnSidecarsByRangeRPCHandler looks up the request data columns from the database from a given start slot index
func (s *Service) dataColumnSidecarsByRangeRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	var err error
	ctx, span := trace.StartSpan(ctx, "sync.DataColumnSidecarsByRangeHandler")
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, respTimeout)
	defer cancel()
	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.DataColumnSidecarsByRangeName[1:]) // slice the leading slash off the name var

	r, ok := msg.(*pb.DataColumnSidecarsByRangeRequest)
	if !ok {
		return errors.New("message is not type *pb.DataColumnSidecarsByRangeRequest")
	}
	if err := s.rateLimiter.validateRequest(stream, 1); err != nil {
		return err
	}
	rp, err := validateDataColumnsByRange(r, s.cfg.chain.CurrentSlot())
	if err != nil {
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
		tracing.AnnotateError(span, err)
		return err
	}

	// Ticker to stagger out large requests.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	batcher, err := newBlockRangeBatcher(rp, s.cfg.beaconDB, s.rateLimiter, s.cfg.chain.IsCanonical, ticker)
	if err != nil {
		log.WithError(err).Info("Error in DataColumnSidecarsByRange batch")
		s.writeErrorResponseToStream(responseCodeServerError, p2ptypes.ErrGeneric.Error(), stream)
		tracing.AnnotateError(span, err)
		return err
	}
	// Derive the wanted columns for the request.
	wantedColumns := map[uint64]bool{}
	for _, c := range r.Columns {
		wantedColumns[c] = true
	}

	var batch blockBatch
	wQuota := params.BeaconConfig().MaxRequestDataColumnSidecars
	for batch, ok = batcher.next(ctx, stream); ok; batch, ok = batcher.next(ctx, stream) {
		batchStart := time.Now()
		wQuota, err = s.streamDataColumnBatch(ctx, batch, wQuota, wantedColumns, stream)
		rpcBlobsByRangeResponseLatency.Observe(float64(time.Since(batchStart).Milliseconds()))
		if err != nil {
			return err
		}
		// once we have written MAX_REQUEST_BLOB_SIDECARS, we're done serving the request
		if wQuota == 0 {
			break
		}
	}
	if err := batch.error(); err != nil {
		log.WithError(err).Debug("error in DataColumnSidecarsByRange batch")

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

// Set the count limit to the number of blobs in a batch.
func columnBatchLimit() uint64 {
	return uint64(flags.Get().BlockBatchLimit) / fieldparams.MaxBlobsPerBlock
}

// TODO: Generalize between data columns and blobs, while the validation parameters used are different they
// are the same value in the config. Can this be safely abstracted ?
func validateDataColumnsByRange(r *pb.DataColumnSidecarsByRangeRequest, current primitives.Slot) (rangeParams, error) {
	if r.Count == 0 {
		return rangeParams{}, errors.Wrap(p2ptypes.ErrInvalidRequest, "invalid request Count parameter")
	}
	rp := rangeParams{
		start: r.StartSlot,
		size:  r.Count,
	}
	// Peers may overshoot the current slot when in initial sync, so we don't want to penalize them by treating the
	// request as an error. So instead we return a set of params that acts as a noop.
	if rp.start > current {
		return rangeParams{start: current, end: current, size: 0}, nil
	}

	var err error
	rp.end, err = rp.start.SafeAdd(rp.size - 1)
	if err != nil {
		return rangeParams{}, errors.Wrap(p2ptypes.ErrInvalidRequest, "overflow start + count -1")
	}

	maxRequest := params.MaxRequestBlock(slots.ToEpoch(current))
	// Allow some wiggle room, up to double the MaxRequestBlocks past the current slot,
	// to give nodes syncing close to the head of the chain some margin for error.
	maxStart, err := current.SafeAdd(maxRequest * 2)
	if err != nil {
		return rangeParams{}, errors.Wrap(p2ptypes.ErrInvalidRequest, "current + maxRequest * 2 > max uint")
	}

	// Clients MUST keep a record of signed data column sidecars seen on the epoch range
	// [max(current_epoch - MIN_EPOCHS_FOR_DATA_COLUMN_SIDECARS_REQUESTS, DENEB_FORK_EPOCH), current_epoch]
	// where current_epoch is defined by the current wall-clock time,
	// and clients MUST support serving requests of data columns on this range.
	minStartSlot, err := DataColumnsRPCMinValidSlot(current)
	if err != nil {
		return rangeParams{}, errors.Wrap(p2ptypes.ErrInvalidRequest, "DataColumnsRPCMinValidSlot error")
	}
	if rp.start > maxStart {
		return rangeParams{}, errors.Wrap(p2ptypes.ErrInvalidRequest, "start > maxStart")
	}
	if rp.start < minStartSlot {
		rp.start = minStartSlot
	}

	if rp.end > current {
		rp.end = current
	}
	if rp.end < rp.start {
		rp.end = rp.start
	}

	limit := columnBatchLimit()
	if limit > maxRequest {
		limit = maxRequest
	}
	if rp.size > limit {
		rp.size = limit
	}

	return rp, nil
}
