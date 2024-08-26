package sync

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/sirupsen/logrus"

	"github.com/prysmaticlabs/prysm/v5/async"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	statefeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	coreTime "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/crypto/rand"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

const PeerRefreshInterval = 1 * time.Minute

type roundSummary struct {
	RequestedColumns []uint64
	MissingColumns   map[uint64]bool
}

// DataColumnSampler defines the interface for sampling data columns from peers for requested block root and samples count.
type DataColumnSampler interface {
	// Run starts the data column sampling service.
	Run(ctx context.Context)
}

var _ DataColumnSampler = (*dataColumnSampler1D)(nil)

// dataColumnSampler1D implements the DataColumnSampler interface for PeerDAS 1D.
type dataColumnSampler1D struct {
	sync.RWMutex

	p2p           p2p.P2P
	clock         *startup.Clock
	ctxMap        ContextByteVersions
	stateNotifier statefeed.Notifier

	// nonCustodyColumns is a set of columns that are not custodied by the node.
	nonCustodyColumns map[uint64]bool
	// columnFromPeer maps a peer to the columns it is responsible for custody.
	columnFromPeer map[peer.ID]map[uint64]bool
	// peerFromColumn maps a column to the peer responsible for custody.
	peerFromColumn map[uint64]map[peer.ID]bool
	// columnVerifier verifies a column according to the specified requirements.
	columnVerifier verification.NewColumnVerifier
}

// newDataColumnSampler1D creates a new 1D data column sampler.
func newDataColumnSampler1D(
	p2p p2p.P2P,
	clock *startup.Clock,
	ctxMap ContextByteVersions,
	stateNotifier statefeed.Notifier,
	colVerifier verification.NewColumnVerifier,
) *dataColumnSampler1D {
	numColumns := params.BeaconConfig().NumberOfColumns
	peerFromColumn := make(map[uint64]map[peer.ID]bool, numColumns)
	for i := uint64(0); i < numColumns; i++ {
		peerFromColumn[i] = make(map[peer.ID]bool)
	}

	return &dataColumnSampler1D{
		p2p:            p2p,
		clock:          clock,
		ctxMap:         ctxMap,
		stateNotifier:  stateNotifier,
		columnFromPeer: make(map[peer.ID]map[uint64]bool),
		peerFromColumn: peerFromColumn,
		columnVerifier: colVerifier,
	}
}

// Run implements DataColumnSampler.
func (d *dataColumnSampler1D) Run(ctx context.Context) {
	// verify if we need to run sampling or not, if not, return directly
	csc := peerdas.CustodySubnetCount()
	columns, err := peerdas.CustodyColumns(d.p2p.NodeID(), csc)
	if err != nil {
		log.WithError(err).Error("Failed to determine local custody columns")
		return
	}

	custodyColumnsCount := uint64(len(columns))
	if peerdas.CanSelfReconstruct(custodyColumnsCount) {
		log.WithFields(logrus.Fields{
			"custodyColumnsCount": custodyColumnsCount,
			"totalColumns":        params.BeaconConfig().NumberOfColumns,
		}).Debug("The node custodies at least the half the data columns, no need to sample")
		return
	}

	// initialize non custody columns.
	d.nonCustodyColumns = make(map[uint64]bool)
	for i := uint64(0); i < params.BeaconConfig().NumberOfColumns; i++ {
		if exists := columns[i]; !exists {
			d.nonCustodyColumns[i] = true
		}
	}

	// initialize peer info first.
	d.refreshPeerInfo()

	// periodically refresh peer info to keep peer <-> column mapping up to date.
	async.RunEvery(ctx, PeerRefreshInterval, d.refreshPeerInfo)

	// start the sampling loop.
	d.samplingRoutine(ctx)
}

func (d *dataColumnSampler1D) samplingRoutine(ctx context.Context) {
	stateCh := make(chan *feed.Event, 1)
	stateSub := d.stateNotifier.StateFeed().Subscribe(stateCh)
	defer stateSub.Unsubscribe()

	for {
		select {
		case evt := <-stateCh:
			d.handleStateNotification(ctx, evt)
		case err := <-stateSub.Err():
			log.WithError(err).Error("DataColumnSampler1D subscription to state feed failed")
		case <-ctx.Done():
			log.Debug("Context canceled, exiting data column sampling loop.")
			return
		}
	}
}

// Refresh peer information.
func (d *dataColumnSampler1D) refreshPeerInfo() {
	dataColumnSidecarSubnetCount := params.BeaconConfig().DataColumnSidecarSubnetCount
	columnsPerSubnet := fieldparams.NumberOfColumns / dataColumnSidecarSubnetCount

	d.Lock()
	defer d.Unlock()

	activePeers := d.p2p.Peers().Active()
	d.prunePeerInfo(activePeers)

	for _, pid := range activePeers {
		csc := d.p2p.CustodyCountFromRemotePeer(pid)

		columns, ok := d.columnFromPeer[pid]
		columnsCount := uint64(len(columns))

		if ok && columnsCount == csc*columnsPerSubnet {
			// No change for this peer.
			continue
		}

		nid, err := p2p.ConvertPeerIDToNodeID(pid)
		if err != nil {
			log.WithError(err).WithField("peerID", pid).Error("Failed to convert peer ID to node ID")
			continue
		}

		columns, err = peerdas.CustodyColumns(nid, csc)
		if err != nil {
			log.WithError(err).WithField("peerID", pid).Error("Failed to determine peer custody columns")
			continue
		}

		d.columnFromPeer[pid] = columns
		for column := range columns {
			d.peerFromColumn[column][pid] = true
		}
	}

	columnWithNoPeers := make([]uint64, 0)
	for column, peers := range d.peerFromColumn {
		if len(peers) == 0 {
			columnWithNoPeers = append(columnWithNoPeers, column)
		}
	}
	if len(columnWithNoPeers) > 0 {
		log.WithField("columnWithNoPeers", columnWithNoPeers).Warn("Some columns have no peers responsible for custody")
	}
}

// prunePeerInfo prunes inactive peers from peerFromColumn and columnFromPeer.
// This should not be called outside of refreshPeerInfo without being locked.
func (d *dataColumnSampler1D) prunePeerInfo(activePeers []peer.ID) {
	active := make(map[peer.ID]bool)
	for _, pid := range activePeers {
		active[pid] = true
	}

	for pid := range d.columnFromPeer {
		if !active[pid] {
			d.prunePeer(pid)
		}
	}
}

// prunePeer removes a peer from stored peer info map, it should be called with lock held.
func (d *dataColumnSampler1D) prunePeer(pid peer.ID) {
	delete(d.columnFromPeer, pid)
	for _, peers := range d.peerFromColumn {
		delete(peers, pid)
	}
}

func (d *dataColumnSampler1D) handleStateNotification(ctx context.Context, event *feed.Event) {
	if event.Type != statefeed.BlockProcessed {
		return
	}

	data, ok := event.Data.(*statefeed.BlockProcessedData)
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

	if !coreTime.PeerDASIsActive(data.Slot) {
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

	// Randomize columns for sample selection.
	randomizedColumns := randomizeColumns(d.nonCustodyColumns)
	samplesCount := min(params.BeaconConfig().SamplesPerSlot, uint64(len(d.nonCustodyColumns))-params.BeaconConfig().NumberOfColumns/2)

	// TODO: Use the first output of `incrementalDAS` as input of the fork choice rule.
	_, _, err = d.incrementalDAS(ctx, data.BlockRoot, randomizedColumns, samplesCount)
	if err != nil {
		log.WithError(err).Error("Failed to run incremental DAS")
	}
}

// incrementalDAS samples data columns from active peers using incremental DAS.
// https://ethresear.ch/t/lossydas-lossy-incremental-and-diagonal-sampling-for-data-availability/18963#incrementaldas-dynamically-increase-the-sample-size-10
// According to https://github.com/ethereum/consensus-specs/issues/3825, we're going to select query samples exclusively from the non custody columns.
func (d *dataColumnSampler1D) incrementalDAS(
	ctx context.Context,
	root [fieldparams.RootLength]byte,
	columns []uint64,
	sampleCount uint64,
) (bool, []roundSummary, error) {
	allowedFailures := uint64(0)
	firstColumnToSample, extendedSampleCount := uint64(0), peerdas.ExtendedSampleCount(sampleCount, allowedFailures)
	roundSummaries := make([]roundSummary, 0, 1) // We optimistically allocate only one round summary.

	start := time.Now()

	for round := 1; ; /*No exit condition */ round++ {
		if extendedSampleCount > uint64(len(columns)) {
			// We already tried to sample all possible columns, this is the unhappy path.
			log.WithFields(logrus.Fields{
				"root":  fmt.Sprintf("%#x", root),
				"round": round - 1,
			}).Warning("Some columns are still missing after trying to sample all possible columns")
			return false, roundSummaries, nil
		}

		// Get the columns to sample for this round.
		columnsToSample := columns[firstColumnToSample:extendedSampleCount]
		columnsToSampleCount := extendedSampleCount - firstColumnToSample

		log.WithFields(logrus.Fields{
			"root":    fmt.Sprintf("%#x", root),
			"columns": columnsToSample,
			"round":   round,
		}).Debug("Start data columns sampling")

		// Sample data columns from peers in parallel.
		retrievedSamples := d.sampleDataColumns(ctx, root, columnsToSample)

		missingSamples := make(map[uint64]bool)
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
				"neededRounds": round,
				"duration":     time.Since(start),
			}).Debug("All columns were successfully sampled")
			return true, roundSummaries, nil
		}

		if retrievedSampleCount > columnsToSampleCount {
			// This should never happen.
			return false, nil, errors.New("retrieved more columns than requested")
		}

		// missing columns, extend the samples.
		allowedFailures += columnsToSampleCount - retrievedSampleCount
		oldExtendedSampleCount := extendedSampleCount
		firstColumnToSample = extendedSampleCount
		extendedSampleCount = peerdas.ExtendedSampleCount(sampleCount, allowedFailures)

		log.WithFields(logrus.Fields{
			"root":                fmt.Sprintf("%#x", root),
			"round":               round,
			"missingColumnsCount": allowedFailures,
			"currentSampleIndex":  oldExtendedSampleCount,
			"nextSampleIndex":     extendedSampleCount,
		}).Debug("Some columns are still missing after sampling this round.")
	}
}

func (d *dataColumnSampler1D) sampleDataColumns(
	ctx context.Context,
	root [fieldparams.RootLength]byte,
	columns []uint64,
) map[uint64]bool {
	// distribute samples to peer
	peerToColumns := d.distributeSamplesToPeer(columns)

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	res := make(map[uint64]bool)
	sampleFromPeer := func(pid peer.ID, cols map[uint64]bool) {
		defer wg.Done()
		retrieved := d.sampleDataColumnsFromPeer(ctx, pid, root, cols)

		mu.Lock()
		for col := range retrieved {
			res[col] = true
		}
		mu.Unlock()
	}

	// sample from peers in parallel
	for pid, cols := range peerToColumns {
		wg.Add(1)
		go sampleFromPeer(pid, cols)
	}

	wg.Wait()
	return res
}

// distributeSamplesToPeer distributes samples to peers based on the columns they are responsible for.
// Currently it randomizes peer selection for a column and did not take into account whole peer distribution balance. It could be improved if needed.
func (d *dataColumnSampler1D) distributeSamplesToPeer(
	columns []uint64,
) map[peer.ID]map[uint64]bool {
	dist := make(map[peer.ID]map[uint64]bool)

	for _, col := range columns {
		peers := d.peerFromColumn[col]
		if len(peers) == 0 {
			log.WithField("column", col).Warn("No peers responsible for custody of column")
			continue
		}

		pid := selectRandomPeer(peers)
		if _, ok := dist[pid]; !ok {
			dist[pid] = make(map[uint64]bool)
		}
		dist[pid][col] = true
	}

	return dist
}

func (d *dataColumnSampler1D) sampleDataColumnsFromPeer(
	ctx context.Context,
	pid peer.ID,
	root [fieldparams.RootLength]byte,
	requestedColumns map[uint64]bool,
) map[uint64]bool {
	retrievedColumns := make(map[uint64]bool)

	req := make(types.DataColumnSidecarsByRootReq, 0)
	for col := range requestedColumns {
		req = append(req, &eth.DataColumnIdentifier{
			BlockRoot:   root[:],
			ColumnIndex: col,
		})
	}

	// Send the request to the peer.
	roDataColumns, err := SendDataColumnSidecarByRoot(ctx, d.clock, d.p2p, pid, d.ctxMap, &req)
	if err != nil {
		log.WithError(err).Error("Failed to send data column sidecar by root")
		return nil
	}

	for _, roDataColumn := range roDataColumns {
		if verifyColumn(roDataColumn, root, pid, requestedColumns, d.columnVerifier) {
			retrievedColumns[roDataColumn.ColumnIndex] = true
		}
	}

	if len(retrievedColumns) == len(requestedColumns) {
		log.WithFields(logrus.Fields{
			"peerID":           pid,
			"root":             fmt.Sprintf("%#x", root),
			"requestedColumns": sortedSliceFromMap(requestedColumns),
		}).Debug("Sampled columns from peer successfully")
	} else {
		log.WithFields(logrus.Fields{
			"peerID":           pid,
			"root":             fmt.Sprintf("%#x", root),
			"requestedColumns": sortedSliceFromMap(requestedColumns),
			"retrievedColumns": sortedSliceFromMap(retrievedColumns),
		}).Debug("Sampled columns from peer with some errors")
	}

	return retrievedColumns
}

// randomizeColumns returns a slice containing all the numbers between 0 and colNum in a random order.
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

// sortedSliceFromMap returns a sorted list of keys from a map.
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

// selectRandomPeer returns a random peer from the given list of peers.
func selectRandomPeer(peers map[peer.ID]bool) peer.ID {
	pick := rand.NewGenerator().Uint64() % uint64(len(peers))
	for k := range peers {
		if pick == 0 {
			return k
		}
		pick--
	}

	// This should never be reached.
	return peer.ID("")
}

// verifyColumn verifies the retrieved column against the root, the index,
// the KZG inclusion and the KZG proof.
func verifyColumn(
	roDataColumn blocks.RODataColumn,
	root [32]byte,
	pid peer.ID,
	requestedColumns map[uint64]bool,
	columnVerifier verification.NewColumnVerifier,
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

	vf := columnVerifier(roDataColumn, verification.SamplingColumnSidecarRequirements)
	// Filter out columns which did not pass the KZG inclusion proof verification.
	if err := vf.SidecarInclusionProven(); err != nil {
		log.WithFields(logrus.Fields{
			"peerID": pid,
			"root":   fmt.Sprintf("%#x", root),
			"index":  retrievedColumn,
		}).WithError(err).Debug("Failed to verify KZG inclusion proof for retrieved column")
		return false
	}

	// Filter out columns which did not pass the KZG proof verification.
	if err := vf.SidecarKzgProofVerified(); err != nil {
		log.WithFields(logrus.Fields{
			"peerID": pid,
			"root":   fmt.Sprintf("%#x", root),
			"index":  retrievedColumn,
		}).WithError(err).Debug("Failed to verify KZG proof for retrieved column")
		return false
	}
	return true
}
