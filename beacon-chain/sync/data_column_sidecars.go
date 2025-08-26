package sync

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	prysmP2P "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/verification"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	leakybucket "github.com/OffchainLabs/prysm/v6/container/leaky-bucket"
	"github.com/OffchainLabs/prysm/v6/crypto/rand"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	goPeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// DataColumnSidecarsParams stores the common parameters needed to
// fetch data column sidecars from peers.
type DataColumnSidecarsParams struct {
	Ctx         context.Context                     // Context
	Tor         blockchain.TemporalOracle           // Temporal oracle, useful to get the current slot
	P2P         prysmP2P.P2P                        // P2P network interface
	RateLimiter *leakybucket.Collector              // Rate limiter for outgoing requests
	CtxMap      ContextByteVersions                 // Context map, useful to know if a message is mapped to the correct fork
	Storage     filesystem.DataColumnStorageReader  // Data columns storage
	NewVerifier verification.NewDataColumnsVerifier // Data columns verifier to check to conformity of incoming data column sidecars
}

// FetchDataColumnSidecars retrieves data column sidecars from storage and peers for the given
// blocks and requested data column indices. It employs a multi-step strategy:
//
//  1. Direct retrieval: If all requested columns are available in storage, they are
//     retrieved directly without reconstruction.
//  2. Reconstruction-based retrieval: If some requested columns are missing but sufficient
//     stored columns exist (at least the minimum required for reconstruction), the function
//     reconstructs all columns and extracts the requested indices.
//  3. Peer retrieval: If storage and reconstruction fail, missing columns are requested
//     from connected peers that are expected to custody the required data.
//
// The function returns a map of block roots to their corresponding verified read-only data
// columns. It returns an error if data column storage is unavailable, if storage/reconstruction
// operations fail unexpectedly, or if not all requested columns could be retrieved from peers.
func FetchDataColumnSidecars(
	params DataColumnSidecarsParams,
	roBlocks []blocks.ROBlock,
	indicesMap map[uint64]bool,
) (map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, error) {
	if len(roBlocks) == 0 || len(indicesMap) == 0 {
		return nil, nil
	}

	indices := sortedSliceFromMap(indicesMap)
	slotsWithCommitments := make(map[primitives.Slot]bool)
	missingIndicesByRoot := make(map[[fieldparams.RootLength]byte]map[uint64]bool)
	indicesByRootStored := make(map[[fieldparams.RootLength]byte]map[uint64]bool)
	result := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn)

	for _, roBlock := range roBlocks {
		// Filter out blocks without commitments.
		block := roBlock.Block()
		commitments, err := block.Body().BlobKzgCommitments()
		if err != nil {
			return nil, errors.Wrapf(err, "get blob kzg commitments for block root %#x", roBlock.Root())
		}
		if len(commitments) == 0 {
			continue
		}

		slotsWithCommitments[block.Slot()] = true
		root := roBlock.Root()

		// Step 1: Get the requested sidecars for this root if available in storage
		requestedColumns, err := tryGetStoredColumns(params.Storage, root, indices)
		if err != nil {
			return nil, errors.Wrapf(err, "try get direct columns for root %#x", root)
		}
		if requestedColumns != nil {
			result[root] = requestedColumns
			continue
		}

		// Step 2: If step 1 failed, reconstruct the requested sidecars from what is available in storage
		requestedColumns, err = tryGetReconstructedColumns(params.Storage, root, indices)
		if err != nil {
			return nil, errors.Wrapf(err, "try get reconstructed columns for root %#x", root)
		}
		if requestedColumns != nil {
			result[root] = requestedColumns
			continue
		}

		// Step 3a: If steps 1 and 2 failed, keep track of the sidecars that need to be queried from peers
		//          and those that are already stored.
		indicesToQueryMap, indicesStoredMap := categorizeIndices(params.Storage, root, indices)

		if len(indicesToQueryMap) > 0 {
			missingIndicesByRoot[root] = indicesToQueryMap
		}
		if len(indicesStoredMap) > 0 {
			indicesByRootStored[root] = indicesStoredMap
		}
	}

	// Early return if no sidecars need to be queried from peers.
	if len(missingIndicesByRoot) == 0 {
		return result, nil
	}

	// Step 3b: Request missing sidecars from peers.
	start, count := time.Now(), computeTotalCount(missingIndicesByRoot)
	fromPeersResult, err := tryRequestingColumnsFromPeers(params, roBlocks, slotsWithCommitments, missingIndicesByRoot)
	if err != nil {
		return nil, errors.Wrap(err, "request from peers")
	}

	log.WithFields(logrus.Fields{"duration": time.Since(start), "count": count}).Debug("Requested data column sidecars from peers")

	// Step 3c: If needed, try to reconstruct missing sidecars from storage and fetched data.
	fromReconstructionResult, err := tryReconstructFromStorageAndPeers(params.Storage, fromPeersResult, indicesMap, missingIndicesByRoot)
	if err != nil {
		return nil, errors.Wrap(err, "reconstruct from storage and peers")
	}

	for root, verifiedSidecars := range fromReconstructionResult {
		result[root] = verifiedSidecars
	}

	for root := range fromPeersResult {
		if _, ok := fromReconstructionResult[root]; ok {
			// We already have what we need from peers + reconstruction
			continue
		}

		result[root] = append(result[root], fromPeersResult[root]...)

		storedIndices := indicesByRootStored[root]
		if len(storedIndices) == 0 {
			continue
		}

		storedColumns, err := tryGetStoredColumns(params.Storage, root, sortedSliceFromMap(storedIndices))
		if err != nil {
			return nil, errors.Wrapf(err, "try get direct columns for root %#x", root)
		}

		result[root] = append(result[root], storedColumns...)
	}

	return result, nil
}

// tryGetStoredColumns attempts to retrieve all requested data column sidecars directly from storage
// if they are all available. Returns the sidecars if successful, and nil if at least one
// requested sidecar is not available in the storage.
func tryGetStoredColumns(storage filesystem.DataColumnStorageReader, blockRoot [fieldparams.RootLength]byte, indices []uint64) ([]blocks.VerifiedRODataColumn, error) {
	// Check if all requested indices are present in cache
	storedIndices := storage.Summary(blockRoot).Stored()
	allRequestedPresent := true
	for _, requestedIndex := range indices {
		if !storedIndices[requestedIndex] {
			allRequestedPresent = false
			break
		}
	}

	if !allRequestedPresent {
		return nil, nil
	}

	// All requested data is present, retrieve directly from DB
	requestedColumns, err := storage.Get(blockRoot, indices)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get data columns for block root %#x", blockRoot)
	}

	return requestedColumns, nil
}

// tryGetReconstructedColumns attempts to retrieve columns using reconstruction
// if sufficient columns are available. Returns the columns if successful, nil and nil if insufficient columns,
// or nil and error if an error occurs.
func tryGetReconstructedColumns(storage filesystem.DataColumnStorageReader, blockRoot [fieldparams.RootLength]byte, indices []uint64) ([]blocks.VerifiedRODataColumn, error) {
	// Check if we have enough columns for reconstruction
	summary := storage.Summary(blockRoot)
	if summary.Count() < peerdas.MinimumColumnCountToReconstruct() {
		return nil, nil
	}

	// Retrieve all stored columns for reconstruction
	allStoredColumns, err := storage.Get(blockRoot, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get all stored columns for reconstruction for block root %#x", blockRoot)
	}

	// Attempt reconstruction
	reconstructedColumns, err := peerdas.ReconstructDataColumnSidecars(allStoredColumns)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to reconstruct data columns for block root %#x", blockRoot)
	}

	// Health check: ensure we have the expected number of columns
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	if uint64(len(reconstructedColumns)) != numberOfColumns {
		return nil, errors.Errorf("reconstructed %d columns but expected %d for block root %#x", len(reconstructedColumns), numberOfColumns, blockRoot)
	}

	// Extract only the requested indices from reconstructed data using direct indexing
	requestedColumns := make([]blocks.VerifiedRODataColumn, 0, len(indices))
	for _, requestedIndex := range indices {
		if requestedIndex >= numberOfColumns {
			return nil, errors.Errorf("requested column index %d exceeds maximum %d for block root %#x", requestedIndex, numberOfColumns-1, blockRoot)
		}
		requestedColumns = append(requestedColumns, reconstructedColumns[requestedIndex])
	}

	return requestedColumns, nil
}

// categorizeIndices separates indices into those that need to be queried from peers
// and those that are already stored.
func categorizeIndices(storage filesystem.DataColumnStorageReader, blockRoot [fieldparams.RootLength]byte, indices []uint64) (map[uint64]bool, map[uint64]bool) {
	indicesToQuery := make(map[uint64]bool, len(indices))
	indicesStored := make(map[uint64]bool, len(indices))

	allStoredIndices := storage.Summary(blockRoot).Stored()
	for _, index := range indices {
		if allStoredIndices[index] {
			indicesStored[index] = true
			continue
		}
		indicesToQuery[index] = true
	}

	return indicesToQuery, indicesStored
}

// tryRequestingColumnsFromPeers attempts to request missing data column sidecars from connected peers.
// It explores the connected peers to find those that are expected to custody the requested columns
// and returns only when all requested columns are either retrieved or have been tried to be retrieved
// by all possible peers.
// WARNING: This function alters `missingIndicesByRoot` by removing successfully retrieved columns.
// After running this function, the user can check the content of the (modified) `missingIndicesByRoot` map
// to check if some sidecars are still missing.
func tryRequestingColumnsFromPeers(
	p DataColumnSidecarsParams,
	roBlocks []blocks.ROBlock,
	slotsWithCommitments map[primitives.Slot]bool,
	missingIndicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
) (map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, error) {
	// Create a new random source for peer selection.
	randomSource := rand.NewGenerator()

	// Compute slots by block root.
	slotByRoot := computeSlotByBlockRoot(roBlocks)

	// Determine all sidecars each peers are expected to custody.
	connectedPeersSlice := p.P2P.Peers().Connected()
	connectedPeers := make(map[goPeer.ID]bool, len(connectedPeersSlice))
	for _, peer := range connectedPeersSlice {
		connectedPeers[peer] = true
	}

	indicesByRootByPeer, err := computeIndicesByRootByPeer(p.P2P, slotByRoot, missingIndicesByRoot, connectedPeers)
	if err != nil {
		return nil, errors.Wrap(err, "explore peers")
	}

	verifiedColumnsByRoot := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn)
	for len(missingIndicesByRoot) > 0 && len(indicesByRootByPeer) > 0 {
		// Select peers to query the missing sidecars from.
		indicesByRootByPeerToQuery, err := selectPeers(p, randomSource, len(missingIndicesByRoot), indicesByRootByPeer)
		if err != nil {
			return nil, errors.Wrap(err, "select peers")
		}

		// Remove selected peers from the maps.
		for peer := range indicesByRootByPeerToQuery {
			delete(connectedPeers, peer)
		}

		// Fetch the sidecars from the chosen peers.
		roDataColumnsByPeer := fetchDataColumnSidecarsFromPeers(p, slotByRoot, slotsWithCommitments, indicesByRootByPeerToQuery)

		// Verify the received data column sidecars.
		verifiedRoDataColumnSidecars, err := verifyDataColumnSidecarsByPeer(p.P2P, p.NewVerifier, roDataColumnsByPeer)
		if err != nil {
			return nil, errors.Wrap(err, "verify data columns sidecars by peer")
		}

		// Remove the verified sidecars from the missing indices map and compute the new verified columns by root.
		localVerifiedColumnsByRoot := updateResults(verifiedRoDataColumnSidecars, missingIndicesByRoot)
		for root, verifiedRoDataColumns := range localVerifiedColumnsByRoot {
			verifiedColumnsByRoot[root] = append(verifiedColumnsByRoot[root], verifiedRoDataColumns...)
		}

		// Compute indices by root by peers with the updated missing indices and connected peers.
		indicesByRootByPeer, err = computeIndicesByRootByPeer(p.P2P, slotByRoot, missingIndicesByRoot, connectedPeers)
		if err != nil {
			return nil, errors.Wrap(err, "explore peers")
		}
	}

	return verifiedColumnsByRoot, nil
}

// tryReconstructFromStorageAndPeers attempts to reconstruct missing data column sidecars
// using the data available in the storage and the data fetched from peers.
// If, for at least one root, the reconstruction is not possible, an error is returned.
func tryReconstructFromStorageAndPeers(
	storage filesystem.DataColumnStorageReader,
	fromPeersByRoot map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn,
	indices map[uint64]bool,
	missingIndicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
) (map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, error) {
	if len(missingIndicesByRoot) == 0 {
		// Nothing to do, return early.
		return nil, nil
	}

	minimumColumnsCountToReconstruct := peerdas.MinimumColumnCountToReconstruct()

	start := time.Now()
	result := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, len(missingIndicesByRoot))
	for root := range missingIndicesByRoot {
		// Check if a reconstruction is possible based on what we have from the store and fetched from peers.
		summary := storage.Summary(root)
		storedCount := summary.Count()
		fetchedCount := uint64(len(fromPeersByRoot[root]))

		if storedCount+fetchedCount < minimumColumnsCountToReconstruct {
			return nil, errors.Errorf("cannot reconstruct all needed columns for root %#x. stored: %d, fetched: %d, minimum: %d", root, storedCount, fetchedCount, minimumColumnsCountToReconstruct)
		}

		// Load all we have in the store.
		storedSidecars, err := storage.Get(root, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get stored sidecars for root %#x", root)
		}

		sidecars := make([]blocks.VerifiedRODataColumn, 0, storedCount+fetchedCount)
		sidecars = append(sidecars, storedSidecars...)
		sidecars = append(sidecars, fromPeersByRoot[root]...)

		// Attempt reconstruction.
		reconstructedSidecars, err := peerdas.ReconstructDataColumnSidecars(sidecars)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to reconstruct data columns for root %#x", root)
		}

		// Select only sidecars we need.
		for _, sidecar := range reconstructedSidecars {
			if indices[sidecar.Index] {
				result[root] = append(result[root], sidecar)
			}
		}
	}

	log.WithFields(logrus.Fields{
		"rootCount": len(missingIndicesByRoot),
		"elapsed":   time.Since(start),
	}).Debug("Reconstructed from storage and peers")

	return result, nil
}

// selectPeers selects peers to query the sidecars.
// It begins by randomly selecting a peer in `origIndicesByRootByPeer` that has enough bandwidth,
// and assigns to it all its available sidecars. Then, it randomly select an other peer, until
// all sidecars in `missingIndicesByRoot` are covered.
func selectPeers(
	p DataColumnSidecarsParams,
	randomSource *rand.Rand,
	count int,
	origIndicesByRootByPeer map[goPeer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool,
) (map[goPeer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool, error) {
	const randomPeerTimeout = 2 * time.Minute

	// Select peers to query the missing sidecars from.
	indicesByRootByPeer := copyIndicesByRootByPeer(origIndicesByRootByPeer)
	internalIndicesByRootByPeer := copyIndicesByRootByPeer(indicesByRootByPeer)
	indicesByRootByPeerToQuery := make(map[goPeer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool)
	for len(internalIndicesByRootByPeer) > 0 {
		// Randomly select a peer with enough bandwidth.
		peer, err := func() (goPeer.ID, error) {
			ctx, cancel := context.WithTimeout(p.Ctx, randomPeerTimeout)
			defer cancel()

			peer, err := randomPeer(ctx, randomSource, p.RateLimiter, count, internalIndicesByRootByPeer)
			if err != nil {
				return "", errors.Wrap(err, "select random peer")
			}

			return peer, err
		}()
		if err != nil {
			return nil, err
		}

		// Query all the sidecars that peer can offer us.
		newIndicesByRoot, ok := internalIndicesByRootByPeer[peer]
		if !ok {
			return nil, errors.Errorf("peer %s not found in internal indices by root by peer map", peer)
		}

		indicesByRootByPeerToQuery[peer] = newIndicesByRoot

		// Remove this peer from the maps to avoid re-selection.
		delete(indicesByRootByPeer, peer)
		delete(internalIndicesByRootByPeer, peer)

		// Delete the corresponding sidecars from other peers in the internal map
		// to avoid re-selection during this iteration.
		for peer, indicesByRoot := range internalIndicesByRootByPeer {
			for root, indices := range indicesByRoot {
				newIndices := newIndicesByRoot[root]
				for index := range newIndices {
					delete(indices, index)
				}
				if len(indices) == 0 {
					delete(indicesByRoot, root)
				}
			}
			if len(indicesByRoot) == 0 {
				delete(internalIndicesByRootByPeer, peer)
			}
		}
	}

	return indicesByRootByPeerToQuery, nil
}

// updateResults updates the missing indices and verified sidecars maps based on the newly verified sidecars.
// WARNING: This function alters `missingIndicesByRoot` by removing verified sidecars.
// After running this function, the user can check the content of the (modified) `missingIndicesByRoot` map
// to check if some sidecars are still missing.
func updateResults(
	verifiedSidecars []blocks.VerifiedRODataColumn,
	missingIndicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
) map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn {
	// Copy the original map to avoid modifying it directly.
	verifiedSidecarsByRoot := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn)
	for _, verifiedSidecar := range verifiedSidecars {
		blockRoot := verifiedSidecar.BlockRoot()
		index := verifiedSidecar.Index

		// Add to the result map grouped by block root
		verifiedSidecarsByRoot[blockRoot] = append(verifiedSidecarsByRoot[blockRoot], verifiedSidecar)

		if indices, ok := missingIndicesByRoot[blockRoot]; ok {
			delete(indices, index)
			if len(indices) == 0 {
				delete(missingIndicesByRoot, blockRoot)
			}
		}
	}

	return verifiedSidecarsByRoot
}

// fetchDataColumnSidecarsFromPeers retrieves data column sidecars from peers.
func fetchDataColumnSidecarsFromPeers(
	params DataColumnSidecarsParams,
	slotByRoot map[[fieldparams.RootLength]byte]primitives.Slot,
	slotsWithCommitments map[primitives.Slot]bool,
	indicesByRootByPeer map[goPeer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool,
) map[goPeer.ID][]blocks.RODataColumn {
	var (
		wg  sync.WaitGroup
		mut sync.Mutex
	)

	roDataColumnsByPeer := make(map[goPeer.ID][]blocks.RODataColumn)
	wg.Add(len(indicesByRootByPeer))
	for peerID, indicesByRoot := range indicesByRootByPeer {
		go func(peerID goPeer.ID, indicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool) {
			defer wg.Done()

			requestedCount := 0
			for _, indices := range indicesByRoot {
				requestedCount += len(indices)
			}

			log := log.WithFields(logrus.Fields{
				"peerID":              peerID,
				"agent":               agentString(peerID, params.P2P.Host()),
				"blockCount":          len(indicesByRoot),
				"totalRequestedCount": requestedCount,
			})

			roDataColumns, err := sendDataColumnSidecarsRequest(params, slotByRoot, slotsWithCommitments, peerID, indicesByRoot)
			if err != nil {
				log.WithError(err).Warning("Failed to send data column sidecars request")
				return
			}

			mut.Lock()
			defer mut.Unlock()
			roDataColumnsByPeer[peerID] = roDataColumns
		}(peerID, indicesByRoot)
	}

	wg.Wait()

	return roDataColumnsByPeer
}

func sendDataColumnSidecarsRequest(
	params DataColumnSidecarsParams,
	slotByRoot map[[fieldparams.RootLength]byte]primitives.Slot,
	slotsWithCommitments map[primitives.Slot]bool,
	peerID goPeer.ID,
	indicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
) ([]blocks.RODataColumn, error) {
	const batchSize = 32

	rootCount := int64(len(indicesByRoot))
	requestedSidecarsCount := 0
	for _, indices := range indicesByRoot {
		requestedSidecarsCount += len(indices)
	}

	log := log.WithFields(logrus.Fields{
		"peerID":            peerID,
		"agent":             agentString(peerID, params.P2P.Host()),
		"requestedSidecars": requestedSidecarsCount,
	})

	// Try to build a by range byRangeRequest first.
	byRangeRequests, err := buildByRangeRequests(slotByRoot, slotsWithCommitments, indicesByRoot, batchSize)
	if err != nil {
		return nil, errors.Wrap(err, "craft by range request")
	}

	// If we have a valid by range request, send it.
	if len(byRangeRequests) > 0 {
		count := 0
		for _, indices := range indicesByRoot {
			count += len(indices)
		}

		start := time.Now()
		roDataColumns := make([]blocks.RODataColumn, 0, count)
		for _, request := range byRangeRequests {
			if params.RateLimiter != nil {
				params.RateLimiter.Add(peerID.String(), rootCount)
			}

			localRoDataColumns, err := SendDataColumnSidecarsByRangeRequest(params, peerID, request)
			if err != nil {
				return nil, errors.Wrapf(err, "send data column sidecars by range request to peer %s", peerID)
			}

			roDataColumns = append(roDataColumns, localRoDataColumns...)
		}

		log.WithFields(logrus.Fields{
			"respondedSidecars": len(roDataColumns),
			"requests":          len(byRangeRequests),
			"type":              "byRange",
			"duration":          time.Since(start),
		}).Debug("Received data column sidecars")

		return roDataColumns, nil
	}

	// Build identifiers for the by root request.
	byRootRequest := buildByRootRequest(indicesByRoot)

	// Send the by root request.
	start := time.Now()
	if params.RateLimiter != nil {
		params.RateLimiter.Add(peerID.String(), rootCount)
	}
	roDataColumns, err := SendDataColumnSidecarsByRootRequest(params, peerID, byRootRequest)
	if err != nil {
		return nil, errors.Wrapf(err, "send data column sidecars by root request to peer %s", peerID)
	}

	log.WithFields(logrus.Fields{
		"respondedSidecars": len(roDataColumns),
		"requests":          1,
		"type":              "byRoot",
		"duration":          time.Since(start),
	}).Debug("Received data column sidecars")

	return roDataColumns, nil
}

// buildByRangeRequests constructs a by range request from the given indices,
// only if the indices are the same all blocks and if the blocks are contiguous.
// (Missing blocks or blocks without commitments do count as contiguous)
// If one of this condition is not met, returns nil.
func buildByRangeRequests(
	slotByRoot map[[fieldparams.RootLength]byte]primitives.Slot,
	slotsWithCommitments map[primitives.Slot]bool,
	indicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	batchSize uint64,
) ([]*ethpb.DataColumnSidecarsByRangeRequest, error) {
	if len(indicesByRoot) == 0 {
		return nil, nil
	}

	var reference map[uint64]bool
	slots := make([]primitives.Slot, 0, len(slotByRoot))
	for root, indices := range indicesByRoot {
		if reference == nil {
			reference = indices
		}

		if !compareIndices(reference, indices) {
			return nil, nil
		}

		slot, ok := slotByRoot[root]
		if !ok {
			return nil, errors.Errorf("slot not found for block root %#x", root)
		}

		slots = append(slots, slot)
	}

	slices.Sort(slots)

	for i := 1; i < len(slots); i++ {
		previous, current := slots[i-1], slots[i]
		if current == previous+1 {
			continue
		}

		for j := previous + 1; j < current; j++ {
			if slotsWithCommitments[j] {
				return nil, nil
			}
		}
	}

	columns := sortedSliceFromMap(reference)
	startSlot, endSlot := slots[0], slots[len(slots)-1]
	totalCount := uint64(endSlot - startSlot + 1)

	requests := make([]*ethpb.DataColumnSidecarsByRangeRequest, 0, totalCount/batchSize)
	for start := startSlot; start <= endSlot; start += primitives.Slot(batchSize) {
		end := min(start+primitives.Slot(batchSize)-1, endSlot)
		request := &ethpb.DataColumnSidecarsByRangeRequest{
			StartSlot: start,
			Count:     uint64(end - start + 1),
			Columns:   columns,
		}

		requests = append(requests, request)
	}

	return requests, nil
}

// buildByRootRequest constructs a by root request from the given indices.
func buildByRootRequest(indicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool) p2ptypes.DataColumnsByRootIdentifiers {
	identifiers := make(p2ptypes.DataColumnsByRootIdentifiers, 0, len(indicesByRoot))
	for root, indices := range indicesByRoot {
		identifier := &eth.DataColumnsByRootIdentifier{
			BlockRoot: root[:],
			Columns:   sortedSliceFromMap(indices),
		}
		identifiers = append(identifiers, identifier)
	}

	// Sort identifiers to have a deterministic output.
	slices.SortFunc(identifiers, func(left, right *eth.DataColumnsByRootIdentifier) int {
		if cmp := bytes.Compare(left.BlockRoot, right.BlockRoot); cmp != 0 {
			return cmp
		}
		return slices.Compare(left.Columns, right.Columns)
	})

	return identifiers
}

// verifyDataColumnSidecarsByPeer verifies the received data column sidecars.
// If at least one sidecar from a peer is invalid, the peer is downscored and
// all its sidecars are rejected. (Sidecars from other peers are still accepted.)
func verifyDataColumnSidecarsByPeer(
	p2p prysmP2P.P2P,
	newVerifier verification.NewDataColumnsVerifier,
	roDataColumnsByPeer map[goPeer.ID][]blocks.RODataColumn,
) ([]blocks.VerifiedRODataColumn, error) {
	// First optimistically verify all received data columns in a single batch.
	count := 0
	for _, columns := range roDataColumnsByPeer {
		count += len(columns)
	}

	roDataColumnSidecars := make([]blocks.RODataColumn, 0, count)
	for _, columns := range roDataColumnsByPeer {
		roDataColumnSidecars = append(roDataColumnSidecars, columns...)
	}

	verifiedRoDataColumnSidecars, err := verifyByRootDataColumnSidecars(newVerifier, roDataColumnSidecars)
	if err == nil {
		// This is the happy path where all sidecars are verified.
		return verifiedRoDataColumnSidecars, nil
	}

	// An error occurred during verification, which means that at least one sidecar is invalid.
	// Reverify peer by peer to identify faulty peer(s), reject all its sidecars, and downscore it.
	verifiedRoDataColumnSidecars = make([]blocks.VerifiedRODataColumn, 0, count)
	for peer, columns := range roDataColumnsByPeer {
		peerVerifiedRoDataColumnSidecars, err := verifyByRootDataColumnSidecars(newVerifier, columns)
		if err != nil {
			// This peer has invalid sidecars.
			log := log.WithError(err).WithField("peerID", peer)
			newScore := p2p.Peers().Scorers().BadResponsesScorer().Increment(peer)
			log.Warning("Peer returned invalid data column sidecars")
			log.WithFields(logrus.Fields{"reason": "invalidDataColumnSidecars", "newScore": newScore}).Debug("Downscore peer")
		}

		verifiedRoDataColumnSidecars = append(verifiedRoDataColumnSidecars, peerVerifiedRoDataColumnSidecars...)
	}

	return verifiedRoDataColumnSidecars, nil
}

// verifyByRootDataColumnSidecars verifies the provided read-only data columns against the
// requirements for data column sidecars received via the by root request.
func verifyByRootDataColumnSidecars(newVerifier verification.NewDataColumnsVerifier, roDataColumns []blocks.RODataColumn) ([]blocks.VerifiedRODataColumn, error) {
	verifier := newVerifier(roDataColumns, verification.ByRootRequestDataColumnSidecarRequirements)

	if err := verifier.ValidFields(); err != nil {
		return nil, errors.Wrap(err, "valid fields")
	}

	if err := verifier.SidecarInclusionProven(); err != nil {
		return nil, errors.Wrap(err, "sidecar inclusion proven")
	}

	if err := verifier.SidecarKzgProofVerified(); err != nil {
		return nil, errors.Wrap(err, "sidecar KZG proof verified")
	}

	verifiedRoDataColumns, err := verifier.VerifiedRODataColumns()
	if err != nil {
		return nil, errors.Wrap(err, "verified RO data columns - should never happen")
	}

	return verifiedRoDataColumns, nil
}

// computeIndicesByRootByPeer returns a peers->root->indices map only for
// root and indices given in `indicesByBlockRoot`. It also only selects peers
// for a given root only if its head state is higher than the block slot.
func computeIndicesByRootByPeer(
	p2p prysmP2P.P2P,
	slotByBlockRoot map[[fieldparams.RootLength]byte]primitives.Slot,
	indicesByBlockRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	peers map[goPeer.ID]bool,
) (map[goPeer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool, error) {
	// First, compute custody columns for all peers
	peersByIndex := make(map[uint64]map[goPeer.ID]bool)
	headSlotByPeer := make(map[goPeer.ID]primitives.Slot)
	for peer := range peers {
		// Computes the custody columns for each peer
		nodeID, err := prysmP2P.ConvertPeerIDToNodeID(peer)
		if err != nil {
			return nil, errors.Wrapf(err, "convert peer ID to node ID for peer %s", peer)
		}

		custodyGroupCount := p2p.CustodyGroupCountFromPeer(peer)

		dasInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
		if err != nil {
			return nil, errors.Wrapf(err, "peerdas info for peer %s", peer)
		}

		for column := range dasInfo.CustodyColumns {
			if _, exists := peersByIndex[column]; !exists {
				peersByIndex[column] = make(map[goPeer.ID]bool)
			}
			peersByIndex[column][peer] = true
		}

		// Compute the head slot for each peer
		peerChainState, err := p2p.Peers().ChainState(peer)
		if err != nil {
			return nil, errors.Wrapf(err, "get chain state for peer %s", peer)
		}

		if peerChainState == nil {
			return nil, errors.Errorf("chain state is nil for peer %s", peer)
		}

		headSlotByPeer[peer] = peerChainState.HeadSlot
	}

	// For each block root and its indices, find suitable peers
	indicesByRootByPeer := make(map[goPeer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool)
	for blockRoot, indices := range indicesByBlockRoot {
		blockSlot, ok := slotByBlockRoot[blockRoot]
		if !ok {
			return nil, errors.Errorf("slot not found for block root %#x", blockRoot)
		}

		for index := range indices {
			peers := peersByIndex[index]
			for peer := range peers {
				peerHeadSlot, ok := headSlotByPeer[peer]
				if !ok {
					return nil, errors.Errorf("head slot not found for peer %s", peer)
				}

				if peerHeadSlot < blockSlot {
					continue
				}

				// Build peers->root->indices map
				if _, exists := indicesByRootByPeer[peer]; !exists {
					indicesByRootByPeer[peer] = make(map[[fieldparams.RootLength]byte]map[uint64]bool)
				}
				if _, exists := indicesByRootByPeer[peer][blockRoot]; !exists {
					indicesByRootByPeer[peer][blockRoot] = make(map[uint64]bool)
				}
				indicesByRootByPeer[peer][blockRoot][index] = true
			}
		}
	}

	return indicesByRootByPeer, nil
}

// randomPeer selects a random peer. If no peers has enough bandwidth, it will wait and retry.
// Returns the selected peer ID and any error.
func randomPeer(
	ctx context.Context,
	randomSource *rand.Rand,
	rateLimiter *leakybucket.Collector,
	count int,
	indicesByRootByPeer map[goPeer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool,
) (goPeer.ID, error) {
	const waitPeriod = 5 * time.Second

	peerCount := len(indicesByRootByPeer)
	if peerCount == 0 {
		return "", errors.New("no peers available")
	}

	for ctx.Err() == nil {
		nonRateLimitedPeers := make([]goPeer.ID, 0, len(indicesByRootByPeer))
		for peer := range indicesByRootByPeer {
			if rateLimiter == nil || rateLimiter.Remaining(peer.String()) >= int64(count) {
				nonRateLimitedPeers = append(nonRateLimitedPeers, peer)
			}
		}

		slices.Sort(nonRateLimitedPeers)

		if len(nonRateLimitedPeers) == 0 {
			log.WithFields(logrus.Fields{
				"peerCount": peerCount,
				"delay":     waitPeriod,
			}).Debug("Waiting for a peer with enough bandwidth for data column sidecars")
			time.Sleep(waitPeriod)
			continue
		}

		randomIndex := randomSource.Intn(len(nonRateLimitedPeers))
		return nonRateLimitedPeers[randomIndex], nil
	}

	return "", ctx.Err()
}

// copyIndicesByRootByPeer creates a deep copy of the given nested map.
// Returns a new map with the same structure and contents.
func copyIndicesByRootByPeer(original map[goPeer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool) map[goPeer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool {
	copied := make(map[goPeer.ID]map[[fieldparams.RootLength]byte]map[uint64]bool, len(original))
	for peer, indicesByRoot := range original {
		copied[peer] = copyIndicesByRoot(indicesByRoot)
	}

	return copied
}

// copyIndicesByRoot creates a deep copy of the given nested map.
// Returns a new map with the same structure and contents.
func copyIndicesByRoot(original map[[fieldparams.RootLength]byte]map[uint64]bool) map[[fieldparams.RootLength]byte]map[uint64]bool {
	copied := make(map[[fieldparams.RootLength]byte]map[uint64]bool, len(original))
	for root, indexMap := range original {
		copied[root] = make(map[uint64]bool, len(indexMap))
		for index, value := range indexMap {
			copied[root][index] = value
		}
	}
	return copied
}

// compareIndices compares two map[uint64]bool and returns true if they are equal.
func compareIndices(left, right map[uint64]bool) bool {
	if len(left) != len(right) {
		return false
	}

	for key, leftValue := range left {
		rightValue, exists := right[key]
		if !exists || leftValue != rightValue {
			return false
		}
	}

	return true
}

// sortedSliceFromMap converts a map[uint64]bool to a sorted slice of keys.
func sortedSliceFromMap(m map[uint64]bool) []uint64 {
	result := make([]uint64, 0, len(m))
	for k := range m {
		result = append(result, k)
	}

	slices.Sort(result)
	return result
}

// computeSlotByBlockRoot maps each block root to its corresponding slot.
func computeSlotByBlockRoot(roBlocks []blocks.ROBlock) map[[fieldparams.RootLength]byte]primitives.Slot {
	slotByBlockRoot := make(map[[fieldparams.RootLength]byte]primitives.Slot, len(roBlocks))
	for _, roBlock := range roBlocks {
		slotByBlockRoot[roBlock.Root()] = roBlock.Block().Slot()
	}
	return slotByBlockRoot
}

// computeTotalCount calculates the total count of indices across all roots.
func computeTotalCount(input map[[fieldparams.RootLength]byte]map[uint64]bool) int {
	totalCount := 0
	for _, indices := range input {
		totalCount += len(indices)
	}
	return totalCount
}
