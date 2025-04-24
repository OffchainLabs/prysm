package sync

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"

	coreTime "github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// uint64MapToSortedSlice produces a sorted uint64 slice from a map.
func uint64MapToSortedSlice(input map[uint64]bool) []uint64 {
	output := make([]uint64, 0, len(input))
	for idx := range input {
		output = append(output, idx)
	}

	slices.Sort[[]uint64](output)
	return output
}

func (s *Service) dataColumnSidecarByRootRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.dataColumnSidecarByRootRPCHandler")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	SetRPCStreamDeadlines(stream)

	// We use the same type as for blobs as they are the same data structure.
	// TODO: Make the type naming more generic to be extensible to data columns
	ref, ok := msg.(*types.DataColumnsByRootIdentifiers)
	if !ok {
		return errors.New("message is not type DataColumnsByRootIdentifiers")
	}

	requestedColumnIdents := *ref

	if err := validateDataColumnsByRootRequest(requestedColumnIdents); err != nil {
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		return errors.Wrap(err, "validate data columns by root request")
	}

	// Sort the identifiers so that requests for the same data columns root will be adjacent, minimizing db lookups.
	sort.Sort(&requestedColumnIdents)

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	requestedColumnsByRoot := make(map[[fieldparams.RootLength]byte][]uint64)
	for _, columnIdent := range requestedColumnIdents {
		var root [fieldparams.RootLength]byte
		copy(root[:], columnIdent.BlockRoot)
		requestedColumnsByRoot[root] = append(requestedColumnsByRoot[root], columnIdent.Columns...)
	}

	// Sort by column index for each root.
	for _, columns := range requestedColumnsByRoot {
		slices.Sort[[]uint64](columns)
	}

	requestedColumnsByRootLog := make(map[string]interface{})
	for root, columns := range requestedColumnsByRoot {
		rootStr := fmt.Sprintf("%#x", root)
		requestedColumnsByRootLog[rootStr] = "all"
		if uint64(len(columns)) != numberOfColumns {
			requestedColumnsByRootLog[rootStr] = columns
		}
	}

	batchSize := flags.Get().DataColumnBatchLimit
	var ticker *time.Ticker
	if len(requestedColumnIdents) > batchSize {
		ticker = time.NewTicker(time.Second)
	}

	// Compute the oldest slot we'll allow a peer to request, based on the current slot.
	cs := s.cfg.clock.CurrentSlot()
	minReqSlot, err := DataColumnsRPCMinValidSlot(cs)
	if err != nil {
		return errors.Wrapf(err, "unexpected error computing min valid data columns request slot, currentSlot=%d", cs)
	}

	log := log.WithFields(logrus.Fields{
		"peer":    stream.Conn().RemotePeer(),
		"columns": requestedColumnsByRootLog,
	})

	log.Debug("Serving data column sidecar by root request")

	count := 0
	for root, columns := range requestedColumnsByRoot {
		if err := ctx.Err(); err != nil {
			closeStream(stream, log)
			return errors.Wrap(err, "context error")
		}

		// Throttle request processing to no more than batchSize/sec.
		// TODO: Find a more efficient way to throttle requests...
		for range columns {
			if ticker != nil && count != 0 && count%batchSize == 0 {
				for {
					select {
					case <-ticker.C:
						log.Debug("Throttling data column sidecar request")
					case <-ctx.Done():
						log.Debug("Context closed, exiting routine")
						return nil
					}
				}
			}

			count++
		}

		s.rateLimiter.add(stream, int64(len(columns)))

		verifiedRODataColumns, err := s.cfg.dataColumnStorage.Get(root, columns)
		if err != nil {
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			return errors.Wrap(err, "get data column sidecars")
		}

		for _, verifiedRODataColumn := range verifiedRODataColumns {
			if verifiedRODataColumn.SignedBlockHeader.Header.Slot < minReqSlot {
				continue
			}

			SetStreamWriteDeadline(stream, defaultWriteDuration)
			if chunkErr := WriteDataColumnSidecarChunk(stream, s.cfg.chain, s.cfg.p2p.Encoding(), verifiedRODataColumn.DataColumnSidecar); chunkErr != nil {
				log.WithError(chunkErr).Debug("Could not send a chunked response")
				s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
				tracing.AnnotateError(span, chunkErr)
				return chunkErr
			}
		}
	}

	closeStream(stream, log)
	return nil
}

func validateDataColumnsByRootRequest(colIdents types.DataColumnsByRootIdentifiers) error {
	total := 0
	for _, id := range colIdents {
		total += len(id.Columns)
	}
	if uint64(total) > params.BeaconConfig().MaxRequestDataColumnSidecars {
		return types.ErrMaxDataColumnReqExceeded
	}
	return nil
}

func DataColumnsRPCMinValidSlot(current primitives.Slot) (primitives.Slot, error) {
	// Avoid overflow if we're running on a config where deneb is set to far future epoch.
	if !coreTime.PeerDASIsActive(current) {
		return primitives.Slot(math.MaxUint64), nil
	}

	minReqEpochs := params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest
	currEpoch := slots.ToEpoch(current)
	minStart := params.BeaconConfig().FuluForkEpoch
	if currEpoch > minReqEpochs && currEpoch-minReqEpochs > minStart {
		minStart = currEpoch - minReqEpochs
	}
	return slots.EpochStart(minStart)
}
