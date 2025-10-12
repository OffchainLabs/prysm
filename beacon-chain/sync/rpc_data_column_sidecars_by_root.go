package sync

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	notDataColumnsByRootIdentifiersError = errors.New("not data columns by root identifiers")
	tickerDelay                          = time.Second
)

// dataColumnSidecarByRootRPCHandler handles the data column sidecars by root RPC request.
// https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/p2p-interface.md#datacolumnsidecarsbyroot-v1
func (s *Service) dataColumnSidecarByRootRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.dataColumnSidecarByRootRPCHandler")
	defer span.End()

	batchSize := flags.Get().DataColumnBatchLimit

	// Check if the message type is the one expected.
	ref, ok := msg.(types.DataColumnsByRootIdentifiers)
	if !ok {
		return notDataColumnsByRootIdentifiersError
	}

	requestedColumnIdents := ref
	remotePeer := stream.Conn().RemotePeer()

	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	SetRPCStreamDeadlines(stream)

	// Penalize peers that send invalid requests.
	if err := validateDataColumnsByRootRequest(requestedColumnIdents); err != nil {
		s.downscorePeer(remotePeer, "dataColumnSidecarByRootRPCHandlerValidationError")
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		return errors.Wrap(err, "validate data columns by root request")
	}

	requestedColumnsByRoot := make(map[[fieldparams.RootLength]byte][]uint64)
	for _, columnIdent := range requestedColumnIdents {
		var root [fieldparams.RootLength]byte
		copy(root[:], columnIdent.BlockRoot)
		requestedColumnsByRoot[root] = append(requestedColumnsByRoot[root], columnIdent.Columns...)
	}

	// Sort by column index for each root.
	for _, columns := range requestedColumnsByRoot {
		slices.Sort(columns)
	}

	// Compute the oldest slot we'll allow a peer to request, based on the current slot.
	minReqSlot, err := dataColumnsRPCMinValidSlot(s.cfg.clock.CurrentSlot())
	if err != nil {
		return errors.Wrapf(err, "data columns RPC min valid slot")
	}

	log := log.WithField("peer", remotePeer)

	defer closeStream(stream, log)

	var ticker *time.Ticker
	if len(requestedColumnIdents) > batchSize {
		ticker = time.NewTicker(tickerDelay)
	}

	if log.Logger.Level >= logrus.TraceLevel {
		// We optimistially assume the peer requests the same set of columns for all roots,
		// pre-sizing the map accordingly.
		requestedRootsByColumnSet := make(map[string][]string, 1)
		for root, columns := range requestedColumnsByRoot {
			slices.Sort(columns)
			prettyColumns := helpers.PrettySlice(columns)
			requestedRootsByColumnSet[prettyColumns] = append(requestedRootsByColumnSet[prettyColumns], fmt.Sprintf("%#x", root))
		}

		log.WithField("requested", requestedRootsByColumnSet).Trace("Serving data column sidecars by root")
	}

	count := 0
	for _, ident := range requestedColumnIdents {
		if err := ctx.Err(); err != nil {
			closeStream(stream, log)
			return errors.Wrap(err, "context error")
		}

		root := bytesutil.ToBytes32(ident.BlockRoot)
		columns := ident.Columns

		// Throttle request processing to no more than batchSize/sec.
		for range columns {
			if ticker != nil && count != 0 && count%batchSize == 0 {
				<-ticker.C
			}

			count++
		}

		s.rateLimiter.add(stream, int64(len(columns)))

		// Retrieve the requested sidecars from the store.
		verifiedRODataColumns, err := s.cfg.dataColumnStorage.Get(root, columns)
		if err != nil {
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			return errors.Wrap(err, "get data column sidecars")
		}

		for _, verifiedRODataColumn := range verifiedRODataColumns {
			// Filter out data column sidecars that are too old.
			if verifiedRODataColumn.SignedBlockHeader.Header.Slot < minReqSlot {
				continue
			}

			SetStreamWriteDeadline(stream, defaultWriteDuration)
			if chunkErr := WriteDataColumnSidecarChunk(stream, s.cfg.clock, s.cfg.p2p.Encoding(), verifiedRODataColumn.DataColumnSidecar); chunkErr != nil {
				s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
				tracing.AnnotateError(span, chunkErr)
				return chunkErr
			}
		}
	}

	return nil
}

// validateDataColumnsByRootRequest checks if the request for data column sidecars is valid.
func validateDataColumnsByRootRequest(colIdents types.DataColumnsByRootIdentifiers) error {
	total := uint64(0)
	for _, id := range colIdents {
		total += uint64(len(id.Columns))
	}

	if total > params.BeaconConfig().MaxRequestDataColumnSidecars {
		return types.ErrMaxDataColumnReqExceeded
	}

	return nil
}

// dataColumnsRPCMinValidSlot returns the minimum slot that a peer can request data column sidecars for.
func dataColumnsRPCMinValidSlot(currentSlot primitives.Slot) (primitives.Slot, error) {
	// Avoid overflow if we're running on a config where fulu is set to far future epoch.
	if !params.FuluEnabled() {
		return primitives.Slot(math.MaxUint64), nil
	}

	beaconConfig := params.BeaconConfig()
	minReqEpochs := beaconConfig.MinEpochsForDataColumnSidecarsRequest
	minStartEpoch := beaconConfig.FuluForkEpoch

	currEpoch := slots.ToEpoch(currentSlot)
	if currEpoch > minReqEpochs && currEpoch-minReqEpochs > minStartEpoch {
		minStartEpoch = currEpoch - minReqEpochs
	}

	epochStart, err := slots.EpochStart(minStartEpoch)
	if err != nil {
		return 0, errors.Wrapf(err, "epoch start for epoch %d", minStartEpoch)
	}

	return epochStart, nil
}
