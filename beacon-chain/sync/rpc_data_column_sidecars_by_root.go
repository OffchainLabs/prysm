package sync

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	libp2pcore "github.com/libp2p/go-libp2p/core"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

func (s *Service) dataColumnSidecarByRootRPCHandler(ctx context.Context, msg interface{}, stream libp2pcore.Stream) error {
	ctx, span := trace.StartSpan(ctx, "sync.dataColumnSidecarByRootRPCHandler")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, ttfbTimeout)
	defer cancel()

	SetRPCStreamDeadlines(stream)
	log := log.WithField("handler", p2p.DataColumnSidecarsByRootName[1:]) // slice the leading slash off the name var

	// We use the same type as for blobs as they are the same data structure.
	// TODO: Make the type naming more generic to be extensible to data columns
	ref, ok := msg.(*types.BlobSidecarsByRootReq)
	if !ok {
		return errors.New("message is not type BlobSidecarsByRootReq")
	}

	requestedColumnIdents := *ref
	if err := validateDataColummnsByRootRequest(requestedColumnIdents); err != nil {
		s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
		s.writeErrorResponseToStream(responseCodeInvalidRequest, err.Error(), stream)
		return errors.Wrap(err, "validate data columns by root request")
	}

	// Sort the identifiers so that requests for the same blob root will be adjacent, minimizing db lookups.
	sort.Sort(requestedColumnIdents)

	requestedColumnsList := make([]uint64, 0, len(requestedColumnIdents))
	for _, ident := range requestedColumnIdents {
		requestedColumnsList = append(requestedColumnsList, ident.Index)
	}

	// TODO: Customize data column batches too
	batchSize := flags.Get().BlobBatchLimit
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

	// Compute all custodied columns.
	custodiedSubnets := params.BeaconConfig().CustodyRequirement
	if flags.Get().SubscribeToAllSubnets {
		custodiedSubnets = params.BeaconConfig().DataColumnSidecarSubnetCount
	}

	custodiedColumns, err := peerdas.CustodyColumns(s.cfg.p2p.NodeID(), custodiedSubnets)
	if err != nil {
		log.WithError(err).Errorf("unexpected error retrieving the node id")
		s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
		return errors.Wrap(err, "custody columns")
	}

	custodiedColumnsList := make([]uint64, 0, len(custodiedColumns))
	for column := range custodiedColumns {
		custodiedColumnsList = append(custodiedColumnsList, column)
	}

	// Sort the custodied columns by index.
	sort.Slice(custodiedColumnsList, func(i, j int) bool {
		return custodiedColumnsList[i] < custodiedColumnsList[j]
	})

	log.WithFields(logrus.Fields{
		"custodied":      custodiedColumnsList,
		"requested":      requestedColumnsList,
		"custodiedCount": len(custodiedColumnsList),
		"requestedCount": len(requestedColumnsList),
	}).Debug("Received data column sidecar by root request")

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
		root, idx := bytesutil.ToBytes32(requestedColumnIdents[i].BlockRoot), requestedColumnIdents[i].Index

		isCustodied := custodiedColumns[idx]
		if !isCustodied {
			s.cfg.p2p.Peers().Scorers().BadResponsesScorer().Increment(stream.Conn().RemotePeer())
			s.writeErrorResponseToStream(responseCodeInvalidRequest, types.ErrInvalidColumnIndex.Error(), stream)
			return types.ErrInvalidColumnIndex
		}

		// TODO: Differentiate between blobs and columns for our storage engine
		// If the data column is nil, it means it is not yet available in the db.
		// We wait for it to be available.
		// TODO: Use a real feed like `nc := s.blobNotifiers.forRoot(root)` instead of this for/sleep loop looking in the DB.
		var sc *eth.DataColumnSidecar

		for {
			sc, err = s.cfg.blobStorage.GetColumn(root, idx)
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					closeStream(stream, log)
					return ctxErr
				}

				if db.IsNotFound(err) {
					fields := logrus.Fields{
						"root":  fmt.Sprintf("%#x", root),
						"index": idx,
					}

					log.WithFields(fields).Debugf("Peer requested data column sidecar by root not found in db, waiting for it to be available")
					time.Sleep(100 * time.Millisecond) // My heart is crying
					continue
				}

				log.WithError(err).Errorf("unexpected db error retrieving data column, root=%x, index=%d", root, idx)
				s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)

				return errors.Wrap(err, "get column")
			}

			break
		}

		// If any root in the request content references a block earlier than minimum_request_epoch,
		// peers MAY respond with error code 3: ResourceUnavailable or not include the data column in the response.
		// note: we are deviating from the spec to allow requests for data column that are before minimum_request_epoch,
		// up to the beginning of the retention period.
		if sc.SignedBlockHeader.Header.Slot < minReqSlot {
			s.writeErrorResponseToStream(responseCodeResourceUnavailable, types.ErrDataColumnLTMinRequest.Error(), stream)
			log.WithError(types.ErrDataColumnLTMinRequest).
				Debugf("requested data column for block %#x before minimum_request_epoch", requestedColumnIdents[i].BlockRoot)
			return types.ErrDataColumnLTMinRequest
		}

		SetStreamWriteDeadline(stream, defaultWriteDuration)
		if chunkErr := WriteDataColumnSidecarChunk(stream, s.cfg.chain, s.cfg.p2p.Encoding(), sc); chunkErr != nil {
			log.WithError(chunkErr).Debug("Could not send a chunked response")
			s.writeErrorResponseToStream(responseCodeServerError, types.ErrGeneric.Error(), stream)
			tracing.AnnotateError(span, chunkErr)
			return chunkErr
		}
	}

	closeStream(stream, log)
	return nil
}

func validateDataColummnsByRootRequest(colIdents types.BlobSidecarsByRootReq) error {
	if uint64(len(colIdents)) > params.BeaconConfig().MaxRequestDataColumnSidecars {
		return types.ErrMaxDataColumnReqExceeded
	}
	return nil
}

func DataColumnsRPCMinValidSlot(current primitives.Slot) (primitives.Slot, error) {
	// Avoid overflow if we're running on a config where deneb is set to far future epoch.
	if params.BeaconConfig().DenebForkEpoch == math.MaxUint64 || !features.Get().EnablePeerDAS {
		return primitives.Slot(math.MaxUint64), nil
	}
	minReqEpochs := params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest
	currEpoch := slots.ToEpoch(current)
	minStart := params.BeaconConfig().DenebForkEpoch
	if currEpoch > minReqEpochs && currEpoch-minReqEpochs > minStart {
		minStart = currEpoch - minReqEpochs
	}
	return slots.EpochStart(minStart)
}
