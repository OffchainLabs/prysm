package sync

import (
	"context"
	"fmt"
	"sort"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	coreTime "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/sirupsen/logrus"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	statefeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

type roundSummary struct {
	RequestedColumns []uint64
	MissingColumns   map[uint64]bool
}

// randomizeColumns returns a slice containing all columns in a random order.
func randomizeColumns(columns map[uint64]bool) []uint64 {
	// Create a slice from columns.
	randomized := make([]uint64, 0, len(columns))
	for column := range columns {
		randomized = append(randomized, column)
	}

	// Shuffle the slice.
	rand.NewGenerator().Shuffle(len(randomized), func(i, j int) {
		randomized[i], randomized[j] = randomized[j], randomized[i]
	})

	return randomized
}

// sortedSliceFromMap returns a sorted slices of keys from a map.
func sortedSliceFromMap(m map[uint64]bool) []uint64 {
	result := make([]uint64, 0, len(m))
	for k := range m {
		result = append(result, k)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})

	return result
}

// custodyColumnsFromPeer returns the columns the peer should custody.
func (s *Service) custodyColumnsFromPeer(pid peer.ID) (map[uint64]bool, error) {
	// Retrieve the custody count of the peer.
	custodySubnetCount := s.cfg.p2p.CustodyCountFromRemotePeer(pid)

	// Extract the node ID from the peer ID.
	nodeID, err := p2p.ConvertPeerIDToNodeID(pid)
	if err != nil {
		return nil, errors.Wrap(err, "extract node ID")
	}

	// Determine which columns the peer should custody.
	custodyColumns, err := peerdas.CustodyColumns(nodeID, custodySubnetCount)
	if err != nil {
		return nil, errors.Wrap(err, "custody columns")
	}

	return custodyColumns, nil
}

// verifyColumn verifies the retrieved column against the root, the index,
// the KZG inclusion and the KZG proof.
func verifyColumn(
	roDataColumn blocks.RODataColumn,
	root [32]byte,
	pid peer.ID,
	requestedColumns map[uint64]bool,
) bool {
	retrievedColumn := roDataColumn.ColumnIndex

	// Filter out columns with incorrect root.
	actualRoot := roDataColumn.BlockRoot()
	if actualRoot != root {
		log.WithFields(logrus.Fields{
			"peerID":        pid,
			"requestedRoot": fmt.Sprintf("%#x", root),
			"actualRoot":    fmt.Sprintf("%#x", actualRoot),
		}).Debug("Retrieved root does not match requested root")

		return false
	}

	// Filter out columns that were not requested.
	if !requestedColumns[retrievedColumn] {
		columnsToSampleList := sortedSliceFromMap(requestedColumns)

		log.WithFields(logrus.Fields{
			"peerID":           pid,
			"requestedColumns": columnsToSampleList,
			"retrievedColumn":  retrievedColumn,
		}).Debug("Retrieved column was not requested")

		return false
	}

	// Filter out columns which did not pass the KZG inclusion proof verification.
	if err := blocks.VerifyKZGInclusionProofColumn(roDataColumn.DataColumnSidecar); err != nil {
		log.WithFields(logrus.Fields{
			"peerID": pid,
			"root":   fmt.Sprintf("%#x", root),
			"index":  retrievedColumn,
		}).Debug("Failed to verify KZG inclusion proof for retrieved column")

		return false
	}

	// Filter out columns which did not pass the KZG proof verification.
	verified, err := peerdas.VerifyDataColumnSidecarKZGProofs(roDataColumn.DataColumnSidecar)
	if err != nil {
		log.WithFields(logrus.Fields{
			"peerID": pid,
			"root":   fmt.Sprintf("%#x", root),
			"index":  retrievedColumn,
		}).Debug("Error when verifying KZG proof for retrieved column")

		return false
	}

	if !verified {
		log.WithFields(logrus.Fields{
			"peerID": pid,
			"root":   fmt.Sprintf("%#x", root),
			"index":  retrievedColumn,
		}).Debug("Failed to verify KZG proof for retrieved column")

		return false
	}

	return true
}

// sampleDataColumnsFromPeer samples data columns from a peer.
// It returns the retrieved columns.
func (s *Service) sampleDataColumnsFromPeer(
	pid peer.ID,
	requestedColumns map[uint64]bool,
	root [fieldparams.RootLength]byte,
) (map[uint64]bool, error) {
	// Build the data column identifiers.
	dataColumnIdentifiers := make(types.DataColumnSidecarsByRootReq, 0, len(requestedColumns))
	for index := range requestedColumns {
		dataColumnIdentifiers = append(dataColumnIdentifiers, &eth.DataColumnIdentifier{
			BlockRoot:   root[:],
			ColumnIndex: index,
		})
	}

	// Send the request.
	roDataColumns, err := SendDataColumnSidecarByRoot(s.ctx, s.cfg.clock, s.cfg.p2p, pid, s.ctxMap, &dataColumnIdentifiers)
	if err != nil {
		return nil, errors.Wrap(err, "send data column sidecar by root")
	}

	retrievedColumns := make(map[uint64]bool, len(roDataColumns))

	for _, roDataColumn := range roDataColumns {
		if verifyColumn(roDataColumn, root, pid, requestedColumns) {
			retrievedColumns[roDataColumn.ColumnIndex] = true
		}
	}

	if len(retrievedColumns) == len(requestedColumns) {
		// This is the happy path.
		log.WithFields(logrus.Fields{
			"peerID":           pid,
			"root":             fmt.Sprintf("%#x", root),
			"requestedColumns": sortedSliceFromMap(requestedColumns),
		}).Debug("All requested columns were successfully sampled from peer")

		return retrievedColumns, nil
	}

	// Some columns are missing.
	log.WithFields(logrus.Fields{
		"peerID":           pid,
		"root":             fmt.Sprintf("%#x", root),
		"requestedColumns": sortedSliceFromMap(requestedColumns),
		"retrievedColumns": sortedSliceFromMap(retrievedColumns),
	}).Warning("Some requested columns were not sampled from peer")

	return retrievedColumns, nil
}

// sampleDataColumnsFromPeers samples data columns from active peers.
// It returns the retrieved columns count.
// If one peer fails to return a column it should custody, the column is considered as missing.
func (s *Service) sampleDataColumnsFromPeers(
	columnsToSample []uint64,
	root [fieldparams.RootLength]byte,
) (map[uint64]bool, error) {
	// Build all remaining columns to sample.
	remainingColumnsToSample := make(map[uint64]bool, len(columnsToSample))
	for _, column := range columnsToSample {
		remainingColumnsToSample[column] = true
	}

	// Get the active peers from the p2p service.
	activePids := s.cfg.p2p.Peers().Active()

	retrievedColumns := make(map[uint64]bool, len(columnsToSample))

	// Query all peers until either all columns to request are retrieved or all active peers are queried (whichever comes first).
	for i := 0; len(remainingColumnsToSample) > 0 && i < len(activePids); i++ {
		// Get the peer ID.
		pid := activePids[i]

		// Get the custody columns of the peer.
		peerCustodyColumns, err := s.custodyColumnsFromPeer(pid)
		if err != nil {
			return nil, errors.Wrap(err, "custody columns from peer")
		}

		// Compute the intersection of the peer custody columns and the remaining columns to request.
		peerRequestedColumns := make(map[uint64]bool, len(peerCustodyColumns))
		for column := range remainingColumnsToSample {
			if peerCustodyColumns[column] {
				peerRequestedColumns[column] = true
			}
		}

		// Remove the newsly requested columns from the remaining columns to request.
		for column := range peerRequestedColumns {
			delete(remainingColumnsToSample, column)
		}

		// Sample data columns from the peer.
		peerRetrievedColumns, err := s.sampleDataColumnsFromPeer(pid, peerRequestedColumns, root)
		if err != nil {
			return nil, errors.Wrap(err, "sample data columns from peer")
		}

		// Update the retrieved columns.
		for column := range peerRetrievedColumns {
			retrievedColumns[column] = true
		}
	}

	return retrievedColumns, nil
}

// incrementalDAS samples data columns from active peers using incremental DAS.
// https://ethresear.ch/t/lossydas-lossy-incremental-and-diagonal-sampling-for-data-availability/18963#incrementaldas-dynamically-increase-the-sample-size-10
func (s *Service) incrementalDAS(
	root [fieldparams.RootLength]byte,
	columns []uint64,
	sampleCount uint64,
) (bool, []roundSummary, error) {
	columnsCount, missingColumnsCount := uint64(len(columns)), uint64(0)
	firstColumnToSample, extendedSampleCount := uint64(0), peerdas.ExtendedSampleCount(sampleCount, 0)

	roundSummaries := make([]roundSummary, 0, 1) // We optimistically allocate only one round summary.

	for round := 1; ; /*No exit condition */ round++ {
		if extendedSampleCount > columnsCount {
			// We already tried to sample all possible columns, this is the unhappy path.
			log.WithField("root", fmt.Sprintf("%#x", root)).Warning("Some columns are still missing after sampling all possible columns")
			return false, roundSummaries, nil
		}

		// Get the columns to sample for this round.
		columnsToSample := columns[firstColumnToSample:extendedSampleCount]
		columnsToSampleCount := extendedSampleCount - firstColumnToSample

		// Sample the data columns from the peers.
		retrievedSamples, err := s.sampleDataColumnsFromPeers(columnsToSample, root)
		if err != nil {
			return false, nil, errors.Wrap(err, "sample data columns from peers")
		}

		// Compute the missing samples.
		missingSamples := make(map[uint64]bool, max(0, len(columnsToSample)-len(retrievedSamples)))
		for _, column := range columnsToSample {
			if !retrievedSamples[column] {
				missingSamples[column] = true
			}
		}

		roundSummaries = append(roundSummaries, roundSummary{
			RequestedColumns: columnsToSample,
			MissingColumns:   missingSamples,
		})

		retrievedSampleCount := uint64(len(retrievedSamples))

		if retrievedSampleCount == columnsToSampleCount {
			// All columns were correctly sampled, this is the happy path.
			log.WithFields(logrus.Fields{
				"root":         fmt.Sprintf("%#x", root),
				"roundsNeeded": round,
			}).Debug("All columns were successfully sampled")
			return true, roundSummaries, nil
		}

		if retrievedSampleCount > columnsToSampleCount {
			// This should never happen.
			return false, nil, errors.New("retrieved more columns than requested")
		}

		// Some columns are missing, we need to extend the sample size.
		missingColumnsCount += columnsToSampleCount - retrievedSampleCount

		firstColumnToSample = extendedSampleCount
		oldExtendedSampleCount := extendedSampleCount
		extendedSampleCount = peerdas.ExtendedSampleCount(sampleCount, missingColumnsCount)

		log.WithFields(logrus.Fields{
			"root":                fmt.Sprintf("%#x", root),
			"round":               round,
			"missingColumnsCount": missingColumnsCount,
			"currentSampleCount":  oldExtendedSampleCount,
			"nextSampleCount":     extendedSampleCount,
		}).Debug("Some columns are still missing after sampling this round.")
	}
}

// DataColumnSamplingRoutine runs incremental DAS on block when received.
func (s *Service) DataColumnSamplingRoutine(ctx context.Context) {
	// Get the custody subnets count.
	custodySubnetsCount := peerdas.CustodySubnetCount()

	// Create a subscription to the state feed.
	stateChannel := make(chan *feed.Event, 1)
	stateSub := s.cfg.stateNotifier.StateFeed().Subscribe(stateChannel)

	// Unsubscribe from the state feed when the function returns.
	defer stateSub.Unsubscribe()

	// Retrieve the number of columns.
	columnsCount := params.BeaconConfig().NumberOfColumns

	// Retrieve all columns we custody.
	custodyColumns, err := peerdas.CustodyColumns(s.cfg.p2p.NodeID(), custodySubnetsCount)
	if err != nil {
		log.WithError(err).Error("Failed to get custody columns")
		return
	}

	custodyColumnsCount := uint64(len(custodyColumns))

	// Compute the number of columns to sample.
	if custodyColumnsCount >= columnsCount/2 {
		log.WithFields(logrus.Fields{
			"custodyColumnsCount": custodyColumnsCount,
			"columnsCount":        columnsCount,
		}).Debug("The node custodies at least the half the data columns, no need to sample")
		return
	}

	samplesCount := min(params.BeaconConfig().SamplesPerSlot, columnsCount/2-custodyColumnsCount)

	// Compute all the columns we do NOT custody.
	nonCustodyColums := make(map[uint64]bool, columnsCount-custodyColumnsCount)
	for i := uint64(0); i < columnsCount; i++ {
		if !custodyColumns[i] {
			nonCustodyColums[i] = true
		}
	}

	for {
		select {
		case e := <-stateChannel:
			s.processEvent(e, nonCustodyColums, samplesCount)

		case <-s.ctx.Done():
			log.Debug("Context closed, exiting goroutine")
			return

		case err := <-stateSub.Err():
			log.WithError(err).Error("Subscription to state feed failed")
		}
	}
}

func (s *Service) processEvent(e *feed.Event, nonCustodyColums map[uint64]bool, samplesCount uint64) {
	if e.Type != statefeed.BlockProcessed {
		return
	}

	data, ok := e.Data.(*statefeed.BlockProcessedData)
	if !ok {
		log.Error("Event feed data is not of type *statefeed.BlockProcessedData")
		return
	}

	if !data.Verified {
		// We only process blocks that have been verified
		log.Error("Data is not verified")
		return
	}

	if data.SignedBlock.Version() < version.Deneb {
		log.Debug("Pre Deneb block, skipping data column sampling")
		return
	}

	if coreTime.PeerDASIsActive(data.Slot) {
		// We do not trigger sampling if peerDAS is not active yet.
		return
	}

	// Get the commitments for this block.
	commitments, err := data.SignedBlock.Block().Body().BlobKzgCommitments()
	if err != nil {
		log.WithError(err).Error("Failed to get blob KZG commitments")
		return
	}

	// Skip if there are no commitments.
	if len(commitments) == 0 {
		log.Debug("No commitments in block, skipping data column sampling")
		return
	}

	// Ramdomize all columns.
	randomizedColumns := randomizeColumns(nonCustodyColums)

	// Sample data columns with incremental DAS.
	ok, _, err = s.incrementalDAS(data.BlockRoot, randomizedColumns, samplesCount)
	if err != nil {
		log.WithError(err).Error("Error during incremental DAS")
	}

	if ok {
		log.WithFields(logrus.Fields{
			"root":        fmt.Sprintf("%#x", data.BlockRoot),
			"columns":     randomizedColumns,
			"sampleCount": samplesCount,
		}).Debug("Data column sampling successful")
	} else {
		log.WithFields(logrus.Fields{
			"root":        fmt.Sprintf("%#x", data.BlockRoot),
			"columns":     randomizedColumns,
			"sampleCount": samplesCount,
		}).Warning("Data column sampling failed")
	}
}
