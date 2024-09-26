package sync

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"

	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	coreTime "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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
	ref, ok := msg.(*types.DataColumnSidecarsByRootReq)
	if !ok {
		return errors.New("message is not type DataColumnSidecarsByRootReq")
	}

	requestedColumnIdents := *ref

	if err := validateDataColumnsByRootRequest(requestedColumnIdents); err != nil {
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		return errors.Wrap(err, "validate data columns by root request")
	}

	// Sort the identifiers so that requests for the same blob root will be adjacent, minimizing db lookups.
	sort.Sort(&requestedColumnIdents)

	numberOfColumns := params.BeaconConfig().NumberOfColumns

	requestedColumnsByRoot := make(map[[fieldparams.RootLength]byte]map[uint64]bool)
	for _, columnIdent := range requestedColumnIdents {
		var root [fieldparams.RootLength]byte
		copy(root[:], columnIdent.BlockRoot)

		columnIndex := columnIdent.ColumnIndex

		if _, ok := requestedColumnsByRoot[root]; !ok {
			requestedColumnsByRoot[root] = map[uint64]bool{columnIndex: true}
			continue
		}

		requestedColumnsByRoot[root][columnIndex] = true
	}

	requestedColumnsByRootLog := make(map[[fieldparams.RootLength]byte]interface{})
	for root, columns := range requestedColumnsByRoot {
		requestedColumnsByRootLog[root] = "all"
		if uint64(len(columns)) != numberOfColumns {
			requestedColumnsByRootLog[root] = uint64MapToSortedSlice(columns)
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
		return errors.Wrapf(err, "unexpected error computing min valid blob request slot, current_slot=%d", cs)
	}

	// Compute all custody columns.
	nodeID := s.cfg.p2p.NodeID()
	custodySubnetCount := peerdas.CustodySubnetCount()
	custodyColumns, err := peerdas.CustodyColumns(nodeID, custodySubnetCount)
	custodyColumnsCount := uint64(len(custodyColumns))

	if err != nil {
		log.WithError(err).Errorf("unexpected error retrieving the node id")
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		return errors.Wrap(err, "custody columns")
	}

	var custody interface{} = "all"

	if custodyColumnsCount != numberOfColumns {
		custody = uint64MapToSortedSlice(custodyColumns)
	}

	remotePeer := stream.Conn().RemotePeer()
	log := log.WithFields(logrus.Fields{
		"peer":    remotePeer,
		"custody": custody,
	})

	i := 0
	for root, columns := range requestedColumnsByRootLog {
		log = log.WithFields(logrus.Fields{
			fmt.Sprintf("root%d", i):    fmt.Sprintf("%#x", root),
			fmt.Sprintf("columns%d", i): columns,
		})

		i++
	}

	log.Debug("Serving data column sidecar by root request")

	// Subscribe to the data column feed.
	rootIndexChan := make(chan filesystem.RootIndexPair)
	subscription := s.cfg.blobStorage.DataColumnFeed.Subscribe(rootIndexChan)
	defer subscription.Unsubscribe()

	for i := range requestedColumnIdents {
		if err := ctx.Err(); err != nil {
			closeStream(stream, log)
			return errors.Wrap(err, "context error")
		}

		// Throttle request processing to no more than batchSize/sec.
		if ticker != nil && i != 0 && i%batchSize == 0 {
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

		s.rateLimiter.add(stream, 1)
		requestedRoot, requestedIndex := bytesutil.ToBytes32(requestedColumnIdents[i].BlockRoot), requestedColumnIdents[i].ColumnIndex

		// TODO: Differentiate between blobs and columns for our storage engine
		// Retrieve the data column from the database.
		dataColumnSidecar, err := s.cfg.blobStorage.GetColumn(requestedRoot, requestedIndex)

		if err != nil && !db.IsNotFound(err) {
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			return errors.Wrap(err, "get column")
		}

		// If the data column is not found in the db, just skip it.
		if err != nil && db.IsNotFound(err) {
			continue
		}

		// If any root in the request content references a block earlier than minimum_request_epoch,
		// peers MAY respond with error code 3: ResourceUnavailable or not include the data column in the response.
		// note: we are deviating from the spec to allow requests for data column that are before minimum_request_epoch,
		// up to the beginning of the retention period.
		if dataColumnSidecar.SignedBlockHeader.Header.Slot < minReqSlot {
			s.writeErrorResponseToStream(responseCodeResourceUnavailable, types.ErrDataColumnLTMinRequest.Error(), stream)
			log.WithError(types.ErrDataColumnLTMinRequest).
				Debugf("requested data column for block %#x before minimum_request_epoch", requestedColumnIdents[i].BlockRoot)
			return types.ErrDataColumnLTMinRequest
		}

		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if chunkErr := WriteDataColumnSidecarChunk(stream, s.cfg.chain, s.cfg.p2p.Encoding(), dataColumnSidecar); chunkErr != nil {
			log.WithError(chunkErr).Debug("Could not send a chunked response")
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			tracing.AnnotateError(span, chunkErr)
			return chunkErr
		}
	}

	closeStream(stream, log)
	return nil
}

func validateDataColumnsByRootRequest(colIdents types.DataColumnSidecarsByRootReq) error {
	if uint64(len(colIdents)) > params.BeaconConfig().MaxRequestDataColumnSidecars {
		return types.ErrMaxDataColumnReqExceeded
	}
	return nil
}

func DataColumnsRPCMinValidSlot(current primitives.Slot) (primitives.Slot, error) {
	// Avoid overflow if we're running on a config where deneb is set to far future epoch.
	if params.BeaconConfig().DenebForkEpoch == math.MaxUint64 || !coreTime.PeerDASIsActive(current) {
		return primitives.Slot(math.MaxUint64), nil
	}
	minReqEpochs := params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest
	currEpoch := slots.ToEpoch(current)
	minStart := params.BeaconConfig().Eip7594ForkEpoch
	if currEpoch > minReqEpochs && currEpoch-minReqEpochs > minStart {
		minStart = currEpoch - minReqEpochs
	}
	return slots.EpochStart(minStart)
}
