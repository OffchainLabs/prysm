package sync

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
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
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	goPeer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// DataColumnSidecarsParams stores the common parameters needed to
// fetch data column sidecars from peers.
type DataColumnSidecarsParams struct {
	Ctx                     context.Context                     // Context
	Tor                     blockchain.TemporalOracle           // Temporal oracle, useful to get the current slot
	P2P                     prysmP2P.P2P                        // P2P network interface
	RateLimiter             *leakybucket.Collector              // Rate limiter for outgoing requests
	CtxMap                  ContextByteVersions                 // Context map, useful to know if a message is mapped to the correct fork
	Storage                 filesystem.DataColumnStorageReader  // Data columns storage
	NewVerifier             verification.NewDataColumnsVerifier // Data columns verifier to check to conformity of incoming data column sidecars
	DownscorePeerOnRPCFault bool                                // Downscore a peer if it commits an RPC fault. Not responding sidecars at all is considered as a fault.
}

// FetchDataColumnSidecars retrieves data column sidecars for the given blocks and indices
// using a series of fallback strategies.
//
// For each block in `roBlocks` that has commitments, the function attempts to obtain
// all sidecars corresponding to the indices listed in `requestedIndices`.
//
// The function returns:
//   - A map from block root to the sidecars successfully retrieved.
//   - A set of block roots for which not all requested sidecars could be retrieved.
//
// Retrieval strategy (proceeds to the next step only if not all requested sidecars
// were successfully obtained at the current step):
//  1. Attempt to load the requested sidecars from storage, reconstructing them from
//     other available sidecars in storage if necessary.
//  2. Request any missing sidecars from peers. If some are still missing, attempt to
//     reconstruct them using both stored sidecars and those retrieved from peers.
//  3. Request all remaining possible sidecars from peers that are not already in storage
//     or retrieved in step 2. Stop once either all requested sidecars are retrieved,
//     or enough sidecars are available (from storage, step 2, and step 3) to reconstruct
//     the requested ones.
func FetchDataColumnSidecars(
	params DataColumnSidecarsParams,
	roBlocks []blocks.ROBlock,
	requestedIndices map[uint64]bool,
) (map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, map[[fieldparams.RootLength]byte]map[uint64]bool, error) {
	if len(roBlocks) == 0 || len(requestedIndices) == 0 {
		return nil, nil, nil
	}

	blockCount := len(roBlocks)

	// We first consider all requested roots as incomplete, and remove roots from this
	// set as we retrieve them.
	incompleteRoots := make(map[[fieldparams.RootLength]byte]bool, blockCount)
	slotsWithCommitments := make(map[primitives.Slot]bool, blockCount)
	slotByRoot := make(map[[fieldparams.RootLength]byte]primitives.Slot, blockCount)
	storedIndicesByRoot := make(map[[fieldparams.RootLength]byte]map[uint64]bool, blockCount)

	for _, roBlock := range roBlocks {
		block := roBlock.Block()

		commitments, err := block.Body().BlobKzgCommitments()
		if err != nil {
			return nil, nil, errors.Wrapf(err, "get blob kzg commitments for block root %#x", roBlock.Root())
		}

		if len(commitments) == 0 {
			continue
		}

		root := roBlock.Root()
		slot := block.Slot()

		incompleteRoots[root] = true
		slotByRoot[root] = slot
		slotsWithCommitments[slot] = true

		storedIndices := params.Storage.Summary(root).Stored()
		if len(storedIndices) > 0 {
			storedIndicesByRoot[root] = storedIndices
		}
	}

	initialMissingRootCount := len(incompleteRoots)

	// Request sidecars from storage (by reconstructing them from other available sidecars if needed).
	result, err := requestSidecarsFromStorage(params.Storage, storedIndicesByRoot, requestedIndices, incompleteRoots)
	if err != nil {
		return nil, nil, errors.Wrap(err, "request sidecars from storage")
	}

	log := log.WithField("initialMissingRootCount", initialMissingRootCount)

	if len(incompleteRoots) == 0 {
		log.WithField("finalMissingRootCount", 0).Debug("Fetched data column sidecars from storage")
		return result, nil, nil
	}

	// Request direct sidecars from peers.
	directSidecarsByRoot, err := requestDirectSidecarsFromPeers(params, slotByRoot, requestedIndices, slotsWithCommitments, storedIndicesByRoot, incompleteRoots)
	if err != nil {
		return nil, nil, errors.Wrap(err, "request direct sidecars from peers")
	}

	// Merge sidecars in storage and those received from peers. Reconstruct if needed.
	mergedSidecarsByRoot, err := mergeAvailableSidecars(params.Storage, requestedIndices, storedIndicesByRoot, incompleteRoots, directSidecarsByRoot)
	if err != nil {
		return nil, nil, errors.Wrap(err, "try merge storage and mandatory inputs")
	}

	for root, sidecars := range mergedSidecarsByRoot {
		result[root] = sidecars
	}

	if len(incompleteRoots) == 0 {
		log.WithField("finalMissingRootCount", 0).Debug("Fetched data column sidecars from storage and peers")
		return result, nil, nil
	}

	// Request all possible indirect sidecars from peers which are neither stored nor in `directSidecarsByRoot`
	indirectSidecarsByRoot, err := requestIndirectSidecarsFromPeers(params, slotByRoot, slotsWithCommitments, storedIndicesByRoot, directSidecarsByRoot, requestedIndices, incompleteRoots)
	if err != nil {
		return nil, nil, errors.Wrap(err, "request all sidecars from peers")
	}

	// Merge sidecars in storage and those received from peers. Reconstruct if needed.
	mergedSidecarsByRoot, err = mergeAvailableSidecars(params.Storage, requestedIndices, storedIndicesByRoot, incompleteRoots, indirectSidecarsByRoot)
	if err != nil {
		return nil, nil, errors.Wrap(err, "try merge storage and all inputs")
	}

	for root, sidecars := range mergedSidecarsByRoot {
		result[root] = sidecars
	}

	if len(incompleteRoots) == 0 {
		log.WithField("finalMissingRootCount", 0).Debug("Fetched data column sidecars from storage and peers using rescue mode")
		return result, nil, nil
	}

	// For remaining incomplete roots, assemble what is available.
	incompleteSidecarsByRoot, missingByRoot, err := assembleAvailableSidecars(params.Storage, requestedIndices, incompleteRoots, directSidecarsByRoot)
	if err != nil {
		return nil, nil, errors.Wrap(err, "assemble available sidecars for incomplete roots")
	}

	for root, sidecars := range incompleteSidecarsByRoot {
		result[root] = sidecars
	}

	log.WithField("finalMissingRootCount", len(incompleteRoots)).Warning("Failed to fetch data column sidecars")
	return result, missingByRoot, nil
}

// requestSidecarsFromStorage attempts to retrieve data column sidecars for each block root in `roots`
// and for all indices specified in `requestedIndices`.
//
// If not all requested sidecars can be obtained for a given root, that root is excluded from the result.
// It returns a map from each root to its successfully retrieved sidecars.
//
// WARNING: This function mutates `roots` by removing entries for which all requested sidecars
// were successfully retrieved.
func requestSidecarsFromStorage(
	storage filesystem.DataColumnStorageReader,
	storedIndicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	requestedIndicesMap map[uint64]bool,
	roots map[[fieldparams.RootLength]byte]bool,
) (map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, error) {
	requestedIndices := helpers.SortedSliceFromMap(requestedIndicesMap)

	result := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, len(roots))

	for root := range roots {
		storedIndices := storedIndicesByRoot[root]

		// Check if all requested indices are stored.
		allAvailable := true
		for index := range requestedIndicesMap {
			if !storedIndices[index] {
				allAvailable = false
				break
			}
		}

		// Skip if not all requested indices are stored.
		if !allAvailable {
			continue
		}

		// All requested indices are stored, retrieve them.
		verifiedRoSidecars, err := storage.Get(root, requestedIndices)
		if err != nil {
			return nil, errors.Wrapf(err, "storage get for block root %#x", root)
		}

		result[root] = verifiedRoSidecars
		delete(roots, root)
	}

	return result, nil
}

// requestDirectSidecarsFromPeers tries to fetch missing data column sidecars from connected peers.
// It searches through the available peers to identify those responsible for the requested columns,
// and returns only after all columns have either been successfully retrieved or all candidate peers
// have been exhausted.
//
// It returns a map from each root to its successfully retrieved sidecars.
func requestDirectSidecarsFromPeers(
	params DataColumnSidecarsParams,
	slotByRoot map[[fieldparams.RootLength]byte]primitives.Slot,
	requestedIndices map[uint64]bool,
	slotsWithCommitments map[primitives.Slot]bool,
	storedIndicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	incompleteRoots map[[fieldparams.RootLength]byte]bool,
) (map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, error) {
	start := time.Now()

	// Create a new random source for peer selection.
	randomSource := rand.NewGenerator()

	// Determine all sidecars each peers are expected to custody.
	connectedPeersSlice := params.P2P.Peers().Connected()
	connectedPeers := make(map[goPeer.ID]bool, len(connectedPeersSlice))
	for _, peer := range connectedPeersSlice {
		connectedPeers[peer] = true
	}

	// Compute missing indices by root, excluding those already in storage.
	missingIndicesByRoot := make(map[[fieldparams.RootLength]byte]map[uint64]bool, len(incompleteRoots))
	for root := range incompleteRoots {
		storedIndices := storedIndicesByRoot[root]

		missingIndices := make(map[uint64]bool, len(requestedIndices))
		for index := range requestedIndices {
			if !storedIndices[index] {
				missingIndices[index] = true
			}
		}

		if len(missingIndices) > 0 {
			missingIndicesByRoot[root] = missingIndices
		}
	}

	initialMissingCount := computeTotalCount(missingIndicesByRoot)

	indicesByRootByPeer, err := computeIndicesByRootByPeer(params.P2P, slotByRoot, missingIndicesByRoot, connectedPeers)
	if err != nil {
		return nil, errors.Wrap(err, "explore peers")
	}

	verifiedColumnsByRoot := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn)
	for len(missingIndicesByRoot) > 0 && len(indicesByRootByPeer) > 0 {
		// Select peers to query the missing sidecars from.
		indicesByRootByPeerToQuery, err := selectPeers(params, randomSource, len(missingIndicesByRoot), indicesByRootByPeer)
		if err != nil {
			return nil, errors.Wrap(err, "select peers")
		}

		// Remove selected peers from the maps.
		for peer := range indicesByRootByPeerToQuery {
			delete(connectedPeers, peer)
		}

		// Fetch the sidecars from the chosen peers.
		roDataColumnsByPeer := fetchDataColumnSidecarsFromPeers(params, slotByRoot, slotsWithCommitments, indicesByRootByPeerToQuery)

		// Verify the received data column sidecars.
		verifiedRoDataColumnSidecars, err := verifyDataColumnSidecarsByPeer(params.P2P, params.NewVerifier, roDataColumnsByPeer)
		if err != nil {
			return nil, errors.Wrap(err, "verify data columns sidecars by peer")
		}

		// Remove the verified sidecars from the missing indices map and compute the new verified columns by root.
		localVerifiedColumnsByRoot := updateResults(verifiedRoDataColumnSidecars, missingIndicesByRoot)
		for root, verifiedRoDataColumns := range localVerifiedColumnsByRoot {
			verifiedColumnsByRoot[root] = append(verifiedColumnsByRoot[root], verifiedRoDataColumns...)
		}

		// Compute indices by root by peers with the updated missing indices and connected peers.
		indicesByRootByPeer, err = computeIndicesByRootByPeer(params.P2P, slotByRoot, missingIndicesByRoot, connectedPeers)
		if err != nil {
			return nil, errors.Wrap(err, "explore peers")
		}
	}

	log.WithFields(logrus.Fields{
		"duration":            time.Since(start),
		"initialMissingCount": initialMissingCount,
		"finalMissingCount":   computeTotalCount(missingIndicesByRoot),
	}).Debug("Requested direct data column sidecars from peers")

	return verifiedColumnsByRoot, nil
}

// requestIndirectSidecarsFromPeers requests, for all roots in `missingIndicesbyRootOrig`,
// for all possible peers, taking into account sidecars available in `inputs` and in the storage,
// all possible sidecars until either, for each root:
// - all indices in `indices` are available, or
// - enough sidecars are available to trigger a reconstruction, or
// - all peers are exhausted.
func requestIndirectSidecarsFromPeers(
	p DataColumnSidecarsParams,
	slotByRoot map[[fieldparams.RootLength]byte]primitives.Slot,
	slotsWithCommitments map[primitives.Slot]bool,
	storedIndicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	alreadyAvailableByRoot map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn,
	requestedIndices map[uint64]bool,
	roots map[[fieldparams.RootLength]byte]bool,
) (map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, error) {
	start := time.Now()

	numberOfColumns := params.BeaconConfig().NumberOfColumns
	minimumColumnCountToReconstruct := peerdas.MinimumColumnCountToReconstruct()

	// Create a new random source for peer selection.
	randomSource := rand.NewGenerator()

	// For each root compute all possible data column sidecar indices excluding
	// those already stored or already available.
	indicesToRetrieveByRoot := make(map[[fieldparams.RootLength]byte]map[uint64]bool)
	for root := range roots {
		alreadyAvailableIndices := make(map[uint64]bool, len(alreadyAvailableByRoot[root]))
		for _, sidecar := range alreadyAvailableByRoot[root] {
			alreadyAvailableIndices[sidecar.Index] = true
		}

		storedIndices := storedIndicesByRoot[root]
		indicesToRetrieve := make(map[uint64]bool, numberOfColumns)
		for index := range numberOfColumns {
			if !(storedIndices[index] || alreadyAvailableIndices[index]) {
				indicesToRetrieve[index] = true
			}
		}

		if len(indicesToRetrieve) > 0 {
			indicesToRetrieveByRoot[root] = indicesToRetrieve
		}
	}

	initialToRetrieveRootCount := len(indicesToRetrieveByRoot)

	// Determine all sidecars each peers are expected to custody.
	connectedPeersSlice := p.P2P.Peers().Connected()
	connectedPeers := make(map[goPeer.ID]bool, len(connectedPeersSlice))
	for _, peer := range connectedPeersSlice {
		connectedPeers[peer] = true
	}

	// Compute which peers have which of the missing indices.
	indicesByRootByPeer, err := computeIndicesByRootByPeer(p.P2P, slotByRoot, indicesToRetrieveByRoot, connectedPeers)
	if err != nil {
		return nil, errors.Wrap(err, "explore peers")
	}

	// Already add into results all sidecars present in `alreadyAvailableByRoot`.
	result := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn)
	for root := range roots {
		alreadyAvailable := alreadyAvailableByRoot[root]
		result[root] = append(result[root], alreadyAvailable...)
	}

	for len(indicesToRetrieveByRoot) > 0 && len(indicesByRootByPeer) > 0 {
		// Select peers to query the missing sidecars from.
		indicesByRootByPeerToQuery, err := selectPeers(p, randomSource, len(indicesToRetrieveByRoot), indicesByRootByPeer)
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

		// Add to results all verified sidecars.
		localVerifiedColumnsByRoot := updateResults(verifiedRoDataColumnSidecars, indicesToRetrieveByRoot)
		for root, verifiedRoDataColumns := range localVerifiedColumnsByRoot {
			result[root] = append(result[root], verifiedRoDataColumns...)
		}

		// Unlabel a root as to retrieve if enough sidecars are retrieved to enable a reconstruction,
		// or if all requested sidecars are now available for this root.
		for root, indicesToRetrieve := range indicesToRetrieveByRoot {
			storedIndices := storedIndicesByRoot[root]
			storedCount := uint64(len(storedIndices))
			resultCount := uint64(len(result[root]))

			if storedCount+resultCount >= minimumColumnCountToReconstruct {
				delete(indicesToRetrieveByRoot, root)
				continue
			}

			allRequestedIndicesAvailable := true
			for index := range requestedIndices {
				if indicesToRetrieve[index] {
					// Still need this index.
					allRequestedIndicesAvailable = false
					break
				}
			}

			if allRequestedIndicesAvailable {
				delete(indicesToRetrieveByRoot, root)
			}
		}

		// Compute indices by root by peers with the updated missing indices and connected peers.
		indicesByRootByPeer, err = computeIndicesByRootByPeer(p.P2P, slotByRoot, indicesToRetrieveByRoot, connectedPeers)
		if err != nil {
			return nil, errors.Wrap(err, "explore peers")
		}
	}

	log.WithFields(logrus.Fields{
		"duration":                   time.Since(start),
		"initialToRetrieveRootCount": initialToRetrieveRootCount,
		"finalToRetrieveRootCount":   len(indicesToRetrieveByRoot),
	}).Debug("Requested all data column sidecars from peers")

	return result, nil
}

// mergeAvailableSidecars retrieves missing data column sidecars by combining
// what is available in storage with the sidecars provided in `alreadyAvailableByRoot`,
// reconstructing them when necessary.
//
// The function works in two modes depending on sidecar availability:
//   - If all requested sidecars are already available (no reconstruction needed),
//     it simply returns them directly from storage and inputs.
//   - If storage + inputs together provide enough sidecars to reconstruct all requested ones,
//     it reconstructs and returns the requested sidecars.
//
// If a root cannot yield all requested sidecars, that root is omitted from the result.
//
// Note: It is assumed that no sidecar in `alreadyAvailableByRoot` is already present in storage.
//
// WARNING: This function mutates `roots`, removing any block roots
// for which all requested sidecars were successfully retrieved.
func mergeAvailableSidecars(
	storage filesystem.DataColumnStorageReader,
	requestedIndices map[uint64]bool,
	storedIndicesByRoot map[[fieldparams.RootLength]byte]map[uint64]bool,
	roots map[[fieldparams.RootLength]byte]bool,
	alreadyAvailableByRoot map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn,
) (map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, error) {
	minimumColumnsCountToReconstruct := peerdas.MinimumColumnCountToReconstruct()

	result := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, len(roots))
	for root := range roots {
		storedIndices := storedIndicesByRoot[root]
		alreadyAvailable := alreadyAvailableByRoot[root]

		// Compute already available indices.
		alreadyAvailableIndices := make(map[uint64]bool, len(alreadyAvailable))
		for _, sidecar := range alreadyAvailable {
			alreadyAvailableIndices[sidecar.Index] = true
		}

		// Check if reconstruction is needed.
		isReconstructionNeeded := false
		for index := range requestedIndices {
			if !(storedIndices[index] || alreadyAvailableIndices[index]) {
				isReconstructionNeeded = true
				break
			}
		}

		// Check if reconstruction is possible.
		storedCount := uint64(len(storedIndices))
		alreadyAvailableCount := uint64(len(alreadyAvailableIndices))
		isReconstructionPossible := storedCount+alreadyAvailableCount >= minimumColumnsCountToReconstruct

		// Skip if the reconstruction is needed and not possible.
		if isReconstructionNeeded && !isReconstructionPossible {
			continue
		}

		// Reconstruct if reconstruction is needed and possible.
		if isReconstructionNeeded && isReconstructionPossible {
			// Load all we have in the store.
			stored, err := storage.Get(root, nil)
			if err != nil {
				return nil, errors.Wrapf(err, "storage get for root %#x", root)
			}

			allAvailable := make([]blocks.VerifiedRODataColumn, 0, storedCount+alreadyAvailableCount)
			allAvailable = append(allAvailable, stored...)
			allAvailable = append(allAvailable, alreadyAvailable...)

			// Attempt reconstruction.
			reconstructedSidecars, err := peerdas.ReconstructDataColumnSidecars(allAvailable)
			if err != nil {
				return nil, errors.Wrapf(err, "reconstruct data column sidecars for root %#x", root)
			}

			// Select only sidecars we need.
			for _, sidecar := range reconstructedSidecars {
				if requestedIndices[sidecar.Index] {
					result[root] = append(result[root], sidecar)
				}
			}

			delete(roots, root)
			continue
		}

		// Reconstruction is not needed, simply assemble what is available in storage and already available.
		allAvailable, err := assembleAvailableSidecarsForRoot(storage, alreadyAvailableByRoot, root, requestedIndices)
		if err != nil {
			return nil, errors.Wrap(err, "assemble available sidecars")
		}

		result[root] = allAvailable
		delete(roots, root)
	}

	return result, nil
}

// assembleAvailableSidecars assembles all sidecars available in storage
// and in `alreadyAvailableByRoot` corresponding to `roots`.
// It also returns all missing indices by root.
func assembleAvailableSidecars(
	storage filesystem.DataColumnStorageReader,
	requestedIndices map[uint64]bool,
	roots map[[fieldparams.RootLength]byte]bool,
	alreadyAvailableByRoot map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn,
) (map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, map[[fieldparams.RootLength]byte]map[uint64]bool, error) {
	// Assemble results.
	result := make(map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn, len(roots))
	for root := range roots {
		allAvailable, err := assembleAvailableSidecarsForRoot(storage, alreadyAvailableByRoot, root, requestedIndices)
		if err != nil {
			return nil, nil, errors.Wrap(err, "assemble sidecars for root")
		}

		if len(allAvailable) > 0 {
			result[root] = allAvailable
		}
	}

	// Compute still missing sidecars.
	missingByRoot := make(map[[fieldparams.RootLength]byte]map[uint64]bool, len(roots))
	for root := range roots {
		missing := make(map[uint64]bool, len(requestedIndices))
		for index := range requestedIndices {
			missing[index] = true
		}

		allAvailable := result[root]
		for _, sidecar := range allAvailable {
			delete(missing, sidecar.Index)
		}

		if len(missing) > 0 {
			missingByRoot[root] = missing
		}
	}

	return result, missingByRoot, nil
}

// assembleAvailableSidecarsForRoot assembles all sidecars available in storage
// and in `alreadyAvailableByRoot` corresponding to `root` and `indices`.
func assembleAvailableSidecarsForRoot(
	storage filesystem.DataColumnStorageReader,
	alreadyAvailableByRoot map[[fieldparams.RootLength]byte][]blocks.VerifiedRODataColumn,
	root [fieldparams.RootLength]byte,
	indices map[uint64]bool,
) ([]blocks.VerifiedRODataColumn, error) {
	stored, err := storage.Get(root, helpers.SortedSliceFromMap(indices))
	if err != nil {
		return nil, errors.Wrapf(err, "storage get for root %#x", root)
	}

	alreadyAvailable := alreadyAvailableByRoot[root]

	allAvailable := make([]blocks.VerifiedRODataColumn, 0, len(stored)+len(alreadyAvailable))
	allAvailable = append(allAvailable, stored...)
	allAvailable = append(allAvailable, alreadyAvailable...)

	return allAvailable, nil
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
				log.WithError(err).Debug("Failed to send data column sidecars request")
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

		if logrus.GetLevel() >= logrus.DebugLevel {
			prettyByRangeRequests := make([]map[string]any, 0, len(byRangeRequests))
			for _, request := range byRangeRequests {
				prettyRequest := map[string]any{
					"startSlot": request.StartSlot,
					"count":     request.Count,
					"columns":   helpers.PrettySlice(request.Columns),
				}

				prettyByRangeRequests = append(prettyByRangeRequests, prettyRequest)
			}

			log.WithFields(logrus.Fields{
				"respondedSidecars": len(roDataColumns),
				"requestCount":      len(byRangeRequests),
				"type":              "byRange",
				"duration":          time.Since(start),
				"requests":          prettyByRangeRequests,
			}).Debug("Received data column sidecars")
		}

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

	columns := helpers.SortedSliceFromMap(reference)
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
		identifier := &ethpb.DataColumnsByRootIdentifier{
			BlockRoot: root[:],
			Columns:   helpers.SortedSliceFromMap(indices),
		}
		identifiers = append(identifiers, identifier)
	}

	// Sort identifiers to have a deterministic output.
	slices.SortFunc(identifiers, func(left, right *ethpb.DataColumnsByRootIdentifier) int {
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
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	// First, compute custody columns for all peers
	peersByIndex := make(map[uint64]map[goPeer.ID]bool)
	headSlotByPeer := make(map[goPeer.ID]primitives.Slot)
	for peer := range peers {
		log := log.WithField("peerID", peer)

		// Computes the custody columns for each peer
		nodeID, err := prysmP2P.ConvertPeerIDToNodeID(peer)
		if err != nil {
			log.WithError(err).Debug("Failed to convert peer ID to node ID")
			continue
		}

		custodyGroupCount := p2p.CustodyGroupCountFromPeer(peer)
		dasInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
		if err != nil {
			log.WithError(err).Debug("Failed to get peer DAS info")
			continue
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
			log.WithError(err).Debug("Failed to get peer chain state")
			continue
		}

		if peerChainState == nil {
			log.Debug("Peer chain state is nil")
			continue
		}

		// Our view of the head slot of a peer is not updated in real time.
		// We add an epoch to take into account the fact the real head slot of the peer
		// is higher than our view of it.
		headSlotByPeer[peer] = peerChainState.HeadSlot + slotsPerEpoch
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

		if len(nonRateLimitedPeers) > 0 {
			slices.Sort(nonRateLimitedPeers)
			randomIndex := randomSource.Intn(len(nonRateLimitedPeers))
			return nonRateLimitedPeers[randomIndex], nil
		}

		log.WithFields(logrus.Fields{
			"peerCount": peerCount,
			"delay":     waitPeriod,
		}).Debug("Waiting for a peer with enough bandwidth for data column sidecars")

		select {
		case <-time.After(waitPeriod):
		case <-ctx.Done():
		}
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

// computeTotalCount calculates the total count of indices across all roots.
func computeTotalCount(input map[[fieldparams.RootLength]byte]map[uint64]bool) int {
	totalCount := 0
	for _, indices := range input {
		totalCount += len(indices)
	}
	return totalCount
}
