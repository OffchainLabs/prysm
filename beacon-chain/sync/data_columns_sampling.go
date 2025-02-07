package sync

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/sync/verify"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/sirupsen/logrus"

	"github.com/prysmaticlabs/prysm/v5/async"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	statefeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
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

	// nonCustodyGroups is a set of groups that are not custodied by the node.
	nonCustodyGroups map[uint64]bool

	// groupsByPeer maps a peer to the groups it is responsible for custody.
	groupsByPeer map[peer.ID]map[uint64]bool

	// peersByCustodyGroup maps a group to the peer responsible for custody.
	peersByCustodyGroup map[uint64]map[peer.ID]bool

	// columnVerifier verifies a column according to the specified requirements.
	columnVerifier verification.NewDataColumnsVerifier
}

// newDataColumnSampler1D creates a new 1D data column sampler.
func newDataColumnSampler1D(
	p2p p2p.P2P,
	clock *startup.Clock,
	ctxMap ContextByteVersions,
	stateNotifier statefeed.Notifier,
	colVerifier verification.NewDataColumnsVerifier,
) *dataColumnSampler1D {
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups
	peersByCustodyGroup := make(map[uint64]map[peer.ID]bool, numberOfCustodyGroups)

	for i := range numberOfCustodyGroups {
		peersByCustodyGroup[i] = make(map[peer.ID]bool)
	}

	return &dataColumnSampler1D{
		p2p:                 p2p,
		clock:               clock,
		ctxMap:              ctxMap,
		stateNotifier:       stateNotifier,
		groupsByPeer:        make(map[peer.ID]map[uint64]bool),
		peersByCustodyGroup: peersByCustodyGroup,
		columnVerifier:      colVerifier,
	}
}

// Run implements DataColumnSampler.
func (d *dataColumnSampler1D) Run(ctx context.Context) {
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups

	// Get the node ID.
	nodeID := d.p2p.NodeID()

	// Verify if we need to run sampling or not, if not, return directly.
	custodyGroupCount := peerdas.CustodyGroupCount()

	// Retrieve our local node info.
	localNodeInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
	if err != nil {
		log.WithError(err).Error("peer info")
		return
	}

	if peerdas.CanSelfReconstruct(custodyGroupCount) {
		log.WithFields(logrus.Fields{
			"custodyGroupCount": custodyGroupCount,
			"totalGroups":       numberOfCustodyGroups,
		}).Debug("The node custodies at least the half of the groups, no need to sample")
		return
	}

	// Initialize non custody groups.
	d.nonCustodyGroups = make(map[uint64]bool)
	for i := range numberOfCustodyGroups {
		if !localNodeInfo.CustodyGroups[i] {
			d.nonCustodyGroups[i] = true
		}
	}

	// Initialize peer info first.
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
	d.Lock()
	defer d.Unlock()

	activePeers := d.p2p.Peers().Active()
	d.prunePeerInfo(activePeers)

	for _, pid := range activePeers {
		// Retrieve the custody group count of the peer.
		retrievedCustodyGroupCount := d.p2p.CustodyGroupCountFromPeer(pid)

		// Look into our store the custody storedGroups for this peer.
		storedGroups, ok := d.groupsByPeer[pid]
		storedGroupsCount := uint64(len(storedGroups))

		if ok && storedGroupsCount == retrievedCustodyGroupCount {
			// No change for this peer.
			continue
		}

		nodeID, err := p2p.ConvertPeerIDToNodeID(pid)
		if err != nil {
			log.WithError(err).WithField("peerID", pid).Error("Failed to convert peer ID to node ID")
			continue
		}

		// Retrieve the peer info.
		peerInfo, _, err := peerdas.Info(nodeID, retrievedCustodyGroupCount)
		if err != nil {
			log.WithError(err).WithField("peerID", pid.String()).Error("Failed to determine peer info")
		}

		d.groupsByPeer[pid] = peerInfo.CustodyGroups
		for group := range peerInfo.CustodyGroups {
			d.peersByCustodyGroup[group][pid] = true
		}
	}

	groupsWithoutPeers := make([]uint64, 0)
	for group, peers := range d.peersByCustodyGroup {
		if len(peers) == 0 {
			groupsWithoutPeers = append(groupsWithoutPeers, group)
		}
	}

	if len(groupsWithoutPeers) > 0 {
		slices.Sort[[]uint64](groupsWithoutPeers)
		log.WithField("groups", groupsWithoutPeers).Warn("Some groups have no peers responsible for custody")
	}
}

// prunePeerInfo prunes inactive peers from peerByGroup and groupByPeer.
// This should not be called outside of refreshPeerInfo without being locked.
func (d *dataColumnSampler1D) prunePeerInfo(activePeers []peer.ID) {
	active := make(map[peer.ID]bool)
	for _, pid := range activePeers {
		active[pid] = true
	}

	for pid := range d.groupsByPeer {
		if !active[pid] {
			d.prunePeer(pid)
		}
	}
}

// prunePeer removes a peer from stored peer info map, it should be called with lock held.
func (d *dataColumnSampler1D) prunePeer(pid peer.ID) {
	delete(d.groupsByPeer, pid)
	for _, peers := range d.peersByCustodyGroup {
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

	if data.SignedBlock.Version() < version.Fulu {
		log.Debug("Pre Fulu block, skipping data column sampling")
		return
	}

	// Determine if we need to sample data columns for this block.
	beaconConfig := params.BeaconConfig()
	samplesPerSlots := beaconConfig.SamplesPerSlot
	halfOfCustodyGroups := beaconConfig.NumberOfCustodyGroups / 2
	nonCustodyGroupsCount := uint64(len(d.nonCustodyGroups))

	if nonCustodyGroupsCount <= halfOfCustodyGroups {
		// Nothing to sample.
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
	randomizedColumns, err := randomizeColumns(d.nonCustodyGroups)
	if err != nil {
		log.WithError(err).Error("Failed to randomize columns")
		return
	}

	samplesCount := min(samplesPerSlots, nonCustodyGroupsCount-halfOfCustodyGroups)

	// TODO: Use the first output of `incrementalDAS` as input of the fork choice rule.
	_, _, err = d.incrementalDAS(ctx, data, randomizedColumns, samplesCount)
	if err != nil {
		log.WithError(err).Error("Failed to run incremental DAS")
	}
}

// incrementalDAS samples data columns from active peers using incremental DAS.
// https://ethresear.ch/t/lossydas-lossy-incremental-and-diagonal-sampling-for-data-availability/18963#incrementaldas-dynamically-increase-the-sample-size-10
// According to https://github.com/ethereum/consensus-specs/issues/3825, we're going to select query samples exclusively from the non custody columns.
func (d *dataColumnSampler1D) incrementalDAS(
	ctx context.Context,
	blockProcessedData *statefeed.BlockProcessedData,
	columns []uint64,
	sampleCount uint64,
) (bool, []roundSummary, error) {
	allowedFailures := uint64(0)
	firstColumnToSample, extendedSampleCount := uint64(0), peerdas.ExtendedSampleCount(sampleCount, allowedFailures)
	roundSummaries := make([]roundSummary, 0, 1) // We optimistically allocate only one round summary.
	blockRoot := blockProcessedData.BlockRoot
	columnsCount := uint64(len(columns))

	start := time.Now()

	for round := 1; ; /*No exit condition */ round++ {
		if extendedSampleCount > columnsCount {
			// We already tried to sample all possible columns, this is the unhappy path.
			log.WithFields(logrus.Fields{
				"root":  fmt.Sprintf("%#x", blockRoot),
				"round": round - 1,
			}).Warning("Some columns are still missing after trying to sample all possible columns")
			return false, roundSummaries, nil
		}

		// Get the columns to sample for this round.
		columnsToSample := columns[firstColumnToSample:extendedSampleCount]
		columnsToSampleCount := extendedSampleCount - firstColumnToSample

		log.WithFields(logrus.Fields{
			"root":    fmt.Sprintf("%#x", blockRoot),
			"columns": columnsToSample,
			"round":   round,
		}).Debug("Start data columns sampling")

		// Sample data columns from peers in parallel.
		retrievedSamples, err := d.sampleDataColumns(ctx, blockProcessedData, columnsToSample)
		if err != nil {
			return false, nil, errors.Wrap(err, "sample data columns")
		}

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
				"root":         fmt.Sprintf("%#x", blockRoot),
				"neededRounds": round,
				"duration":     time.Since(start),
			}).Debug("All columns were successfully sampled")
			return true, roundSummaries, nil
		}

		if retrievedSampleCount > columnsToSampleCount {
			// This should never happen.
			return false, nil, errors.New("retrieved more columns than requested")
		}

		// There is still some missing columns, extend the samples.
		allowedFailures += columnsToSampleCount - retrievedSampleCount
		oldExtendedSampleCount := extendedSampleCount
		firstColumnToSample = extendedSampleCount
		extendedSampleCount = peerdas.ExtendedSampleCount(sampleCount, allowedFailures)

		log.WithFields(logrus.Fields{
			"root":                fmt.Sprintf("%#x", blockRoot),
			"round":               round,
			"missingColumnsCount": allowedFailures,
			"currentSampleIndex":  oldExtendedSampleCount,
			"nextSampleIndex":     extendedSampleCount,
		}).Debug("Some columns are still missing after sampling this round.")
	}
}

func (d *dataColumnSampler1D) sampleDataColumns(
	ctx context.Context,
	blockProcessedData *statefeed.BlockProcessedData,
	columns []uint64,
) (map[uint64]bool, error) {
	// distribute samples to peer
	peerToColumns, err := d.distributeSamplesToPeer(columns)
	if err != nil {
		return nil, errors.Wrap(err, "distribute samples to peer")
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	res := make(map[uint64]bool)

	sampleFromPeer := func(pid peer.ID, cols map[uint64]bool) {
		defer wg.Done()
		retrieved := d.sampleDataColumnsFromPeer(ctx, pid, blockProcessedData, cols)

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
	return res, nil
}

// distributeSamplesToPeer distributes samples to peers based on the columns they are responsible for.
// Currently it randomizes peer selection for a column and did not take into account whole peer distribution balance. It could be improved if needed.
func (d *dataColumnSampler1D) distributeSamplesToPeer(columns []uint64) (map[peer.ID]map[uint64]bool, error) {
	dist := make(map[peer.ID]map[uint64]bool)

	for _, column := range columns {
		custodyGroup, err := peerdas.ComputeCustodyGroupForColumn(column)
		if err != nil {
			return nil, errors.Wrap(err, "compute custody group for column")
		}

		peers := d.peersByCustodyGroup[custodyGroup]
		if len(peers) == 0 {
			log.WithField("column", column).Warning("No peers responsible for custody of column")
			continue
		}

		pid, err := selectRandomPeer(peers)
		if err != nil {
			return nil, errors.Wrap(err, "select random peer")
		}

		if _, ok := dist[pid]; !ok {
			dist[pid] = make(map[uint64]bool)
		}

		dist[pid][column] = true
	}

	return dist, nil
}

func (d *dataColumnSampler1D) sampleDataColumnsFromPeer(
	ctx context.Context,
	pid peer.ID,
	blockProcessedData *statefeed.BlockProcessedData,
	requestedColumns map[uint64]bool,
) map[uint64]bool {
	retrievedColumns := make(map[uint64]bool)

	req := make(types.DataColumnSidecarsByRootReq, 0)
	for col := range requestedColumns {
		req = append(req, &eth.DataColumnIdentifier{
			BlockRoot:   blockProcessedData.BlockRoot[:],
			ColumnIndex: col,
		})
	}

	// Send the request to the peer.
	roDataColumns, err := SendDataColumnSidecarsByRootRequest(ctx, d.clock, d.p2p, pid, d.ctxMap, &req)
	if err != nil {
		log.WithError(err).Error("Failed to send data column sidecar by root")
		return nil
	}

	// TODO: Once peer sampling is used, we should verify all sampled data columns in a single batch instead of looping over columns.
	for _, roDataColumn := range roDataColumns {
		if verifyColumn(roDataColumn, blockProcessedData, pid, requestedColumns, d.columnVerifier) {
			retrievedColumns[roDataColumn.ColumnIndex] = true
		}
	}

	if len(retrievedColumns) == len(requestedColumns) {
		log.WithFields(logrus.Fields{
			"peerID":           pid,
			"root":             fmt.Sprintf("%#x", blockProcessedData.BlockRoot),
			"requestedColumns": sortedSliceFromMap(requestedColumns),
		}).Debug("Sampled columns from peer successfully")
	} else {
		log.WithFields(logrus.Fields{
			"peerID":           pid,
			"root":             fmt.Sprintf("%#x", blockProcessedData.BlockRoot),
			"requestedColumns": sortedSliceFromMap(requestedColumns),
			"retrievedColumns": sortedSliceFromMap(retrievedColumns),
		}).Debug("Sampled columns from peer with some errors")
	}

	return retrievedColumns
}

// randomizeColumns returns a slice containing randomly ordered columns belonging to the input `groups`.
func randomizeColumns(custodyGroups map[uint64]bool) ([]uint64, error) {
	// Compute the number of columns per group.
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups
	columnsPerGroup := numberOfColumns / numberOfCustodyGroups

	// Compute the number of columns.
	groupCount := uint64(len(custodyGroups))
	expectedColumnCount := groupCount * columnsPerGroup

	// Compute the columns.
	columns := make([]uint64, 0, expectedColumnCount)
	for group := range custodyGroups {
		columnsGroup, err := peerdas.ComputeColumnsForCustodyGroup(group)
		if err != nil {
			return nil, errors.Wrap(err, "compute columns for custody group")
		}

		columns = append(columns, columnsGroup...)
	}

	actualColumnCount := len(columns)

	// Safety check.
	if uint64(actualColumnCount) != expectedColumnCount {
		return nil, errors.New("invalid number of columns, should never happen")
	}

	// Shuffle the columns.
	rand.NewGenerator().Shuffle(actualColumnCount, func(i, j int) {
		columns[i], columns[j] = columns[j], columns[i]
	})

	return columns, nil
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
func selectRandomPeer(peers map[peer.ID]bool) (peer.ID, error) {
	peersCount := uint64(len(peers))
	pick := rand.NewGenerator().Uint64() % peersCount

	for peer := range peers {
		if pick == 0 {
			return peer, nil
		}

		pick--
	}

	// This should never be reached.
	return peer.ID(""), errors.New("failed to select random peer")
}

// verifyColumn verifies the retrieved column against the root, the index,
// the KZG inclusion and the KZG proof.
func verifyColumn(
	roDataColumn blocks.RODataColumn,
	blockProcessedData *statefeed.BlockProcessedData,
	pid peer.ID,
	requestedColumns map[uint64]bool,
	dataColumnsVerifier verification.NewDataColumnsVerifier,
) bool {
	retrievedColumn := roDataColumn.ColumnIndex

	// Filter out columns with incorrect root.
	columnRoot := roDataColumn.BlockRoot()
	blockRoot := blockProcessedData.BlockRoot

	if columnRoot != blockRoot {
		log.WithFields(logrus.Fields{
			"peerID":        pid,
			"requestedRoot": fmt.Sprintf("%#x", blockRoot),
			"columnRoot":    fmt.Sprintf("%#x", columnRoot),
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

	roBlock := blockProcessedData.SignedBlock.Block()

	wrappedBlockDataColumns := []verify.WrappedBlockDataColumn{
		{
			ROBlock:      roBlock,
			RODataColumn: roDataColumn,
		},
	}

	if err := verify.DataColumnsAlignWithBlock(wrappedBlockDataColumns, dataColumnsVerifier); err != nil {
		return false
	}

	return true
}
