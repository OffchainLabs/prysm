package initialsync

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	coreTime "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	p2pTypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	p2ppb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

// forkData represents alternative chain path supported by a given peer.
// Blocks are stored in an ascending slot order. The first block is guaranteed to have parent
// either in DB or initial sync cache.
type forkData struct {
	peer peer.ID
	bwb  []blocks.BlockWithROBlobs
}

// nonSkippedSlotAfter checks slots after the given one in an attempt to find a non-empty future slot.
// For efficiency only one random slot is checked per epoch, so returned slot might not be the first
// non-skipped slot. This shouldn't be a problem, as in case of adversary peer, we might get incorrect
// data anyway, so code that relies on this function must be robust enough to re-request, if no progress
// is possible with a returned value.
func (f *blocksFetcher) nonSkippedSlotAfter(ctx context.Context, slot primitives.Slot) (primitives.Slot, error) {
	ctx, span := trace.StartSpan(ctx, "initialsync.nonSkippedSlotAfter")
	defer span.End()

	headEpoch, targetEpoch, peers := f.calculateHeadAndTargetEpochs()
	log.WithFields(logrus.Fields{
		"start":       slot,
		"headEpoch":   headEpoch,
		"targetEpoch": targetEpoch,
	}).Debug("Searching for non-skipped slot")

	// Exit early if no peers with epoch higher than our known head are found.
	if targetEpoch <= headEpoch {
		return 0, errSlotIsTooHigh
	}

	// Transform peer list to avoid eclipsing (filter, shuffle, trim).
	peers = f.filterPeers(ctx, peers, peersPercentagePerRequest)
	return f.nonSkippedSlotAfterWithPeersTarget(ctx, slot, peers, targetEpoch)
}

// nonSkippedSlotWithPeersTarget traverse peers (supporting a given target epoch), in an attempt
// to find non-skipped slot among returned blocks.
func (f *blocksFetcher) nonSkippedSlotAfterWithPeersTarget(
	ctx context.Context, slot primitives.Slot, peers []peer.ID, targetEpoch primitives.Epoch,
) (primitives.Slot, error) {
	// Exit early if no peers are ready.
	if len(peers) == 0 {
		return 0, errNoPeersAvailable
	}

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	pidInd := 0

	fetch := func(pid peer.ID, start primitives.Slot, count, step uint64) (primitives.Slot, error) {
		req := &p2ppb.BeaconBlocksByRangeRequest{
			StartSlot: start,
			Count:     count,
			Step:      step,
		}
		blocks, err := f.requestBlocks(ctx, req, pid)
		if err != nil {
			return 0, err
		}
		if len(blocks) > 0 {
			for _, block := range blocks {
				if block.Block().Slot() > slot {
					return block.Block().Slot(), nil
				}
			}
		}
		return 0, nil
	}

	// Start by checking several epochs fully, w/o resorting to random sampling.
	start := slot + 1
	end := start + nonSkippedSlotsFullSearchEpochs*slotsPerEpoch
	for ind := start; ind < end; ind += slotsPerEpoch {
		nextSlot, err := fetch(peers[pidInd%len(peers)], ind, uint64(slotsPerEpoch), 1)
		if err != nil {
			return 0, err
		}
		if nextSlot > slot {
			return nextSlot, nil
		}
		pidInd++
	}

	// Quickly find the close enough epoch where a non-empty slot definitely exists.
	// Only single random slot per epoch is checked - allowing to move forward relatively quickly.
	// This method has been changed to account for our spec change where step can only be 1 in a
	// block by range request. https://github.com/ethereum/consensus-specs/pull/2856
	// The downside is that this method will be less effective during periods without
	// finality.
	slot += nonSkippedSlotsFullSearchEpochs * slotsPerEpoch
	upperBoundSlot, err := slots.EpochStart(targetEpoch + 1)
	if err != nil {
		return 0, err
	}
	for ind := slot + 1; ind < upperBoundSlot; ind += slotsPerEpoch {
		nextSlot, err := fetch(peers[pidInd%len(peers)], ind, uint64(slotsPerEpoch), 1)
		if err != nil {
			return 0, err
		}
		pidInd++
		if nextSlot > slot && upperBoundSlot >= nextSlot {
			upperBoundSlot = nextSlot
			break
		}
	}

	// Epoch with non-empty slot is located. Check all slots within two nearby epochs.
	if upperBoundSlot > slotsPerEpoch {
		upperBoundSlot -= slotsPerEpoch
	}
	upperBoundSlot, err = slots.EpochStart(slots.ToEpoch(upperBoundSlot))
	if err != nil {
		return 0, err
	}
	nextSlot, err := fetch(peers[pidInd%len(peers)], upperBoundSlot, uint64(slotsPerEpoch*2), 1)
	if err != nil {
		return 0, err
	}
	s, err := slots.EpochStart(targetEpoch + 1)
	if err != nil {
		return 0, err
	}
	if nextSlot < slot || s < nextSlot {
		return 0, errors.New("invalid range for non-skipped slot")
	}
	return nextSlot, nil
}

// findFork queries all peers that have higher head slot, in an attempt to find
// ones that feature blocks from alternative branches. Once found, peer is further queried
// to find common ancestor slot. On success, all obtained blocks and peer is returned.
func (f *blocksFetcher) findFork(ctx context.Context, slot primitives.Slot) (*forkData, error) {
	ctx, span := trace.StartSpan(ctx, "initialsync.findFork")
	defer span.End()

	// Safe-guard, since previous epoch is used when calculating.
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	if slot < slotsPerEpoch*2 {
		return nil, fmt.Errorf("slot is too low to backtrack, min. expected %d", slotsPerEpoch*2)
	}

	// The current slot's epoch must be after the finalization epoch,
	// triggering backtracking on earlier epochs is unnecessary.
	cp := f.chain.FinalizedCheckpt()
	finalizedEpoch := cp.Epoch
	epoch := slots.ToEpoch(slot)
	if epoch <= finalizedEpoch {
		return nil, errors.New("slot is not after the finalized epoch, no backtracking is necessary")
	}
	// Update slot to the beginning of the current epoch (preserve original slot for comparison).
	slot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}

	// Select peers that have higher head slot, and potentially blocks from more favourable fork.
	// Exit early if no peers are ready.
	_, peers := f.p2p.Peers().BestNonFinalized(1, epoch+1)
	if len(peers) == 0 {
		return nil, errNoPeersAvailable
	}
	f.rand.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})

	// Query all found peers, stop on peer with alternative blocks, and try backtracking.
	for i, pid := range peers {
		log.WithFields(logrus.Fields{
			"peer": pid,
			"step": fmt.Sprintf("%d/%d", i+1, len(peers)),
		}).Debug("Searching for alternative blocks")
		fork, err := f.findForkWithPeer(ctx, pid, slot)
		if err != nil {
			log.WithFields(logrus.Fields{
				"peer":  pid,
				"error": err.Error(),
			}).Debug("No alternative blocks found for peer")
			continue
		}
		return fork, nil
	}
	return nil, errNoPeersWithAltBlocks
}

var errNoAlternateBlocks = errors.New("no alternative blocks exist within scanned range")

func findForkReqRangeSize() uint64 {
	return uint64(params.BeaconConfig().SlotsPerEpoch.Mul(2))
}

// findForkWithPeer loads some blocks from a peer in an attempt to find alternative blocks.
func (f *blocksFetcher) findForkWithPeer(ctx context.Context, pid peer.ID, slot primitives.Slot) (*forkData, error) {
	const (
		delay     = 5 * time.Second
		batchSize = 32
	)

	reqCount := findForkReqRangeSize()
	// Safe-guard, since previous epoch is used when calculating.
	if uint64(slot) < reqCount {
		return nil, fmt.Errorf("slot is too low to backtrack, min. expected %d", reqCount)
	}
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	// Locate non-skipped slot, supported by a given peer (can survive long periods of empty slots).
	// When searching for non-empty slot, start an epoch earlier - for those blocks we
	// definitely have roots. So, spotting a fork will be easier. It is not a problem if unknown
	// block of the current fork is found: we are searching for forks when FSMs are stuck, so
	// being able to progress on any fork is good.
	pidState, err := f.p2p.Peers().ChainState(pid)
	if err != nil {
		return nil, fmt.Errorf("cannot obtain peer's status: %w", err)
	}
	nonSkippedSlot, err := f.nonSkippedSlotAfterWithPeersTarget(
		ctx, slot-slotsPerEpoch, []peer.ID{pid}, slots.ToEpoch(pidState.HeadSlot))
	if err != nil {
		return nil, fmt.Errorf("cannot locate non-empty slot for a peer: %w", err)
	}

	// Request blocks starting from the first non-empty slot.
	req := &p2ppb.BeaconBlocksByRangeRequest{
		StartSlot: nonSkippedSlot,
		Count:     reqCount,
		Step:      1,
	}
	reqBlocks, err := f.requestBlocks(ctx, req, pid)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch blocks: %w", err)
	}
	if len(reqBlocks) == 0 {
		return nil, errNoAlternateBlocks
	}

	// If the first block is not connected to the current canonical chain, we'll stop processing this batch.
	// Instead, we'll work backwards from the first block until we find a common ancestor,
	// and then begin processing from there.
	first := reqBlocks[0]
	if !f.chain.HasBlock(ctx, first.Block().ParentRoot()) {
		// Backtrack on a root, to find a common ancestor from which we can resume syncing.
		fork, err := f.findAncestor(ctx, pid, first)
		if err != nil {
			return nil, fmt.Errorf("failed to find common ancestor: %w", err)
		}
		return fork, nil
	}

	// Traverse blocks, and if we've got one that doesn't have parent in DB, backtrack on it.
	// Note that we start from the second element in the array, because we know that the first element is in the db,
	// otherwise we would have gone into the findAncestor early return path above.
	for i := 1; i < len(reqBlocks); i++ {
		block := reqBlocks[i]
		parentRoot := block.Block().ParentRoot()
		// Step through blocks until we find one that is not in the chain. The goal is to find the point where the
		// chain observed in the peer diverges from the locally known chain, and then collect up the remainder of the
		// observed chain chunk to start initial-sync processing from the fork point.
		if f.chain.HasBlock(ctx, parentRoot) {
			continue
		}
		log.WithFields(logrus.Fields{
			"peer": pid,
			"slot": block.Block().Slot(),
			"root": fmt.Sprintf("%#x", parentRoot),
		}).Debug("Block with unknown parent root has been found")
		bwb, err := sortedBlockWithVerifiedBlobSlice(reqBlocks[i-1:])
		if err != nil {
			return nil, errors.Wrap(err, "invalid blocks received in findForkWithPeer")
		}
		if coreTime.PeerDASIsActive(block.Block().Slot()) {
			if err := f.fetchDataColumnsFromPeers(ctx, bwb, []peer.ID{pid}, delay, batchSize); err != nil {
				return nil, errors.Wrap(err, "unable to retrieve blobs for blocks found in findForkWithPeer")
			}
		} else {
			if err = f.fetchBlobsFromPeer(ctx, bwb, pid, []peer.ID{pid}); err != nil {
				return nil, errors.Wrap(err, "unable to retrieve blobs for blocks found in findForkWithPeer")
			}
		}
		// We need to fetch the blobs for the given alt-chain if any exist, so that we can try to verify and import
		// the blocks.

		// The caller will use the BlocksWith VerifiedBlobs in bwb as the starting point for
		// round-robin syncing the alternate chain.
		return &forkData{peer: pid, bwb: bwb}, nil
	}
	return nil, errNoAlternateBlocks
}

// findAncestor tries to figure out common ancestor slot that connects a given root to known block.
func (f *blocksFetcher) findAncestor(ctx context.Context, pid peer.ID, b interfaces.ReadOnlySignedBeaconBlock) (*forkData, error) {
	const (
		delay     = 5 * time.Second
		batchSize = 32
	)
	outBlocks := []interfaces.ReadOnlySignedBeaconBlock{b}
	for i := uint64(0); i < backtrackingMaxHops; i++ {
		parentRoot := outBlocks[len(outBlocks)-1].Block().ParentRoot()
		if f.chain.HasBlock(ctx, parentRoot) {
			// Common ancestor found, forward blocks back to processor.
			bwb, err := sortedBlockWithVerifiedBlobSlice(outBlocks)
			if err != nil {
				return nil, errors.Wrap(err, "received invalid blocks in findAncestor")
			}
			if coreTime.PeerDASIsActive(b.Block().Slot()) {
				if err := f.fetchDataColumnsFromPeers(ctx, bwb, []peer.ID{pid}, delay, batchSize); err != nil {
					return nil, errors.Wrap(err, "unable to retrieve columns for blocks found in findAncestor")
				}
			} else {
				if err = f.fetchBlobsFromPeer(ctx, bwb, pid, []peer.ID{pid}); err != nil {
					return nil, errors.Wrap(err, "unable to retrieve blobs for blocks found in findAncestor")
				}
			}
			return &forkData{
				peer: pid,
				bwb:  bwb,
			}, nil
		}
		// Request block's parent.
		req := &p2pTypes.BeaconBlockByRootsReq{parentRoot}
		blocks, err := f.requestBlocksByRoot(ctx, req, pid)
		if err != nil {
			return nil, err
		}
		if len(blocks) == 0 {
			break
		}
		outBlocks = append(outBlocks, blocks[0])
	}
	return nil, errors.New("no common ancestor found")
}

// bestFinalizedSlot returns the highest finalized slot of the majority of connected peers.
func (f *blocksFetcher) bestFinalizedSlot() primitives.Slot {
	cp := f.chain.FinalizedCheckpt()
	finalizedEpoch, _ := f.p2p.Peers().BestFinalized(
		params.BeaconConfig().MaxPeersToSync, cp.Epoch)
	return params.BeaconConfig().SlotsPerEpoch.Mul(uint64(finalizedEpoch))
}

// bestNonFinalizedSlot returns the highest non-finalized slot of enough number of connected peers.
func (f *blocksFetcher) bestNonFinalizedSlot() primitives.Slot {
	headEpoch := slots.ToEpoch(f.chain.HeadSlot())
	targetEpoch, _ := f.p2p.Peers().BestNonFinalized(flags.Get().MinimumSyncPeers*2, headEpoch)
	return params.BeaconConfig().SlotsPerEpoch.Mul(uint64(targetEpoch))
}

// calculateHeadAndTargetEpochs return node's current head epoch, along with the best known target
// epoch. For the latter peers supporting that target epoch are returned as well.
func (f *blocksFetcher) calculateHeadAndTargetEpochs() (headEpoch, targetEpoch primitives.Epoch, peers []peer.ID) {
	if f.mode == modeStopOnFinalizedEpoch {
		cp := f.chain.FinalizedCheckpt()
		headEpoch = cp.Epoch
		targetEpoch, peers = f.p2p.Peers().BestFinalized(params.BeaconConfig().MaxPeersToSync, headEpoch)

		return headEpoch, targetEpoch, peers
	}

	headEpoch = slots.ToEpoch(f.chain.HeadSlot())
	targetEpoch, peers = f.p2p.Peers().BestNonFinalized(flags.Get().MinimumSyncPeers, headEpoch)

	return headEpoch, targetEpoch, peers
}

// custodyColumnFromPeer compute all costody columns indexed by peer.
func (f *blocksFetcher) custodyDataColumnsFromPeer(peers map[peer.ID]bool) (map[peer.ID]map[uint64]bool, error) {
	peerCount := len(peers)

	custodyDataColumnsByPeer := make(map[peer.ID]map[uint64]bool, peerCount)
	for peer := range peers {
		// Get the node ID from the peer ID.
		nodeID, err := p2p.ConvertPeerIDToNodeID(peer)
		if err != nil {
			return nil, errors.Wrap(err, "convert peer ID to node ID")
		}

		// Get the custody columns count from the peer.
		custodyCount := f.p2p.DataColumnsCustodyCountFromRemotePeer(peer)

		// Get the custody columns from the peer.
		custodyDataColumns, err := peerdas.CustodyColumns(nodeID, custodyCount)
		if err != nil {
			return nil, errors.Wrap(err, "custody columns")
		}

		custodyDataColumnsByPeer[peer] = custodyDataColumns
	}

	return custodyDataColumnsByPeer, nil
}

// uint64MapToSortedSlice produces a sorted uint64 slice from a map.
func uint64MapToSortedSlice(input map[uint64]bool) []uint64 {
	output := make([]uint64, 0, len(input))
	for idx := range input {
		output = append(output, idx)
	}

	slices.Sort[[]uint64](output)
	return output
}

// `filterPeerWhichCustodyAtLeastOneDataColumn` filters peers which custody at least one data column
// specified in `neededDataColumns`. It returns also a list of descriptions for non admissible peers.
func filterPeerWhichCustodyAtLeastOneDataColumn(
	neededDataColumns map[uint64]bool,
	inputDataColumnsByPeer map[peer.ID]map[uint64]bool,
) (map[peer.ID]map[uint64]bool, []string) {
	// Get the count of needed data columns.
	neededDataColumnsCount := uint64(len(neededDataColumns))

	// Create pretty needed data columns for logs.
	var neededDataColumnsLog interface{} = "all"
	numberOfColumns := params.BeaconConfig().NumberOfColumns

	if neededDataColumnsCount < numberOfColumns {
		neededDataColumnsLog = uint64MapToSortedSlice(neededDataColumns)
	}

	outputDataColumnsByPeer := make(map[peer.ID]map[uint64]bool, len(inputDataColumnsByPeer))
	descriptions := make([]string, 0)

outerLoop:
	for peer, peerCustodyDataColumns := range inputDataColumnsByPeer {
		for neededDataColumn := range neededDataColumns {
			if peerCustodyDataColumns[neededDataColumn] {
				outputDataColumnsByPeer[peer] = peerCustodyDataColumns

				continue outerLoop
			}
		}

		peerCustodyColumnsCount := uint64(len(peerCustodyDataColumns))
		var peerCustodyColumnsLog interface{} = "all"

		if peerCustodyColumnsCount < numberOfColumns {
			peerCustodyColumnsLog = uint64MapToSortedSlice(peerCustodyDataColumns)
		}

		description := fmt.Sprintf(
			"peer %s: does not custody any needed column, custody columns: %v, needed columns: %v",
			peer, peerCustodyColumnsLog, neededDataColumnsLog,
		)

		descriptions = append(descriptions, description)
	}

	return outputDataColumnsByPeer, descriptions
}

// admissiblePeersForDataColumn returns a map of peers that:
// - custody at least one column listed in `neededDataColumns`,
// - are synced to `targetSlot`, and
// - have enough bandwidth to serve data columns corresponding to `count` blocks.
// It returns:
// - A map, where the key of the map is the peer, the value is the custody columns of the peer.
// - A map, where the key of the map is the data column, the value is the peer that custody the data column.
// - A slice of descriptions for non admissible peers.
// - An error if any.
func (f *blocksFetcher) admissiblePeersForDataColumn(
	peers []peer.ID,
	targetSlot primitives.Slot,
	neededDataColumns map[uint64]bool,
	count uint64,
) (map[peer.ID]map[uint64]bool, map[uint64][]peer.ID, []string, error) {
	// If no peer is specified, get all connected peers.
	inputPeers := peers
	if inputPeers == nil {
		inputPeers = f.p2p.Peers().Connected()
	}

	inputPeerCount := len(inputPeers)
	neededDataColumnsCount := uint64(len(neededDataColumns))

	// Create description slice for non admissible peers.
	descriptions := make([]string, 0, inputPeerCount)

	// Filter peers on bandwidth.
	peersWithSufficientBandwidth := f.hasSufficientBandwidth(inputPeers, count)

	// Convert peers with sufficient bandwidth to a map.
	peerWithSufficientBandwidthMap := make(map[peer.ID]bool, len(peersWithSufficientBandwidth))
	for _, peer := range peersWithSufficientBandwidth {
		peerWithSufficientBandwidthMap[peer] = true
	}

	for _, peer := range inputPeers {
		if !peerWithSufficientBandwidthMap[peer] {
			description := fmt.Sprintf("peer %s: does not have sufficient bandwidth", peer)
			descriptions = append(descriptions, description)
		}
	}

	// Compute the target epoch from the target slot.
	targetEpoch := slots.ToEpoch(targetSlot)

	// Filter peers with head epoch lower than our target epoch.
	peersWithAdmissibleHeadEpoch := make(map[peer.ID]bool, inputPeerCount)
	for _, peer := range peersWithSufficientBandwidth {
		peerChainState, err := f.p2p.Peers().ChainState(peer)

		if err != nil {
			description := fmt.Sprintf("peer %s: error: %s", peer, err)
			descriptions = append(descriptions, description)
			continue
		}

		if peerChainState == nil {
			description := fmt.Sprintf("peer %s: chain state is nil", peer)
			descriptions = append(descriptions, description)
			continue
		}

		peerHeadEpoch := slots.ToEpoch(peerChainState.HeadSlot)

		if peerHeadEpoch < targetEpoch {
			description := fmt.Sprintf("peer %s: peer head epoch %d < our target epoch %d", peer, peerHeadEpoch, targetEpoch)
			descriptions = append(descriptions, description)
			continue
		}

		peersWithAdmissibleHeadEpoch[peer] = true
	}

	// Compute custody columns for each peer.
	dataColumnsByPeerWithAdmissibleHeadEpoch, err := f.custodyDataColumnsFromPeer(peersWithAdmissibleHeadEpoch)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "custody columns from peer")
	}

	// Filter peers which custody at least one needed data column.
	dataColumnsByAdmissiblePeer, localDescriptions := filterPeerWhichCustodyAtLeastOneDataColumn(neededDataColumns, dataColumnsByPeerWithAdmissibleHeadEpoch)
	descriptions = append(descriptions, localDescriptions...)

	// Compute a map from needed data columns to their peers.
	admissiblePeersByDataColumn := make(map[uint64][]peer.ID, neededDataColumnsCount)
	for peer, peerCustodyDataColumns := range dataColumnsByAdmissiblePeer {
		for dataColumn := range peerCustodyDataColumns {
			admissiblePeersByDataColumn[dataColumn] = append(admissiblePeersByDataColumn[dataColumn], peer)
		}
	}

	return dataColumnsByAdmissiblePeer, admissiblePeersByDataColumn, descriptions, nil
}

// selectPeersToFetchDataColumnsFrom implements greedy algorithm in order to select peers to fetch data columns from.
// https://en.wikipedia.org/wiki/Set_cover_problem#Greedy_algorithm
func selectPeersToFetchDataColumnsFrom(
	neededDataColumns map[uint64]bool,
	dataColumnsByPeer map[peer.ID]map[uint64]bool,
) (map[peer.ID][]uint64, error) {
	dataColumnsFromSelectedPeers := make(map[peer.ID][]uint64)

	// Filter `dataColumnsByPeer` to only contain needed data columns.
	neededDataColumnsByPeer := make(map[peer.ID]map[uint64]bool, len(dataColumnsByPeer))
	for pid, dataColumns := range dataColumnsByPeer {
		for dataColumn := range dataColumns {
			if neededDataColumns[dataColumn] {
				if _, ok := neededDataColumnsByPeer[pid]; !ok {
					neededDataColumnsByPeer[pid] = make(map[uint64]bool, len(neededDataColumns))
				}

				neededDataColumnsByPeer[pid][dataColumn] = true
			}
		}
	}

	for len(neededDataColumns) > 0 {
		// Check if at least one peer remains. If not, it means that we don't have enough peers to fetch all needed data columns.
		if len(neededDataColumnsByPeer) == 0 {
			missingDataColumnsSortedSlice := uint64MapToSortedSlice(neededDataColumns)
			return dataColumnsFromSelectedPeers, errors.Errorf("no peer to fetch the following data columns: %v", missingDataColumnsSortedSlice)
		}

		// Select the peer that custody the most needed data columns (greedy selection).
		var bestPeer peer.ID
		for peer, dataColumns := range neededDataColumnsByPeer {
			if len(dataColumns) > len(neededDataColumnsByPeer[bestPeer]) {
				bestPeer = peer
			}
		}

		dataColumnsSortedSlice := uint64MapToSortedSlice(neededDataColumnsByPeer[bestPeer])
		dataColumnsFromSelectedPeers[bestPeer] = dataColumnsSortedSlice

		// Remove the selected peer from the list of peers.
		delete(neededDataColumnsByPeer, bestPeer)

		// Remove the selected peer's data columns from the list of needed data columns.
		for _, dataColumn := range dataColumnsSortedSlice {
			delete(neededDataColumns, dataColumn)
		}

		// Remove the selected peer's data columns from the list of needed data columns by peer.
		for _, dataColumn := range dataColumnsSortedSlice {
			for peer, dataColumns := range neededDataColumnsByPeer {
				delete(dataColumns, dataColumn)

				if len(dataColumns) == 0 {
					delete(neededDataColumnsByPeer, peer)
				}
			}
		}
	}

	return dataColumnsFromSelectedPeers, nil
}

// buildDataColumnSidecarsByRangeRequests builds a list of data column sidecars by range requests.
// Each request contains at most `batchSize` items.
func buildDataColumnSidecarsByRangeRequests(
	startSlot primitives.Slot,
	count uint64,
	columns []uint64,
	batchSize uint64,
) []*p2ppb.DataColumnSidecarsByRangeRequest {
	batches := make([]*p2ppb.DataColumnSidecarsByRangeRequest, 0)

	for i := uint64(0); i < count; i += batchSize {
		localStartSlot := startSlot + primitives.Slot(i)
		localCount := min(batchSize, uint64(startSlot)+count-uint64(localStartSlot))

		batch := &p2ppb.DataColumnSidecarsByRangeRequest{
			StartSlot: localStartSlot,
			Count:     localCount,
			Columns:   columns,
		}

		batches = append(batches, batch)
	}

	return batches
}
