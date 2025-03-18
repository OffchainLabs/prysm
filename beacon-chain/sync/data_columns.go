package sync

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/sync/verify"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/logging"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/sirupsen/logrus"
)

// RequestDataColumnSidecarsByRoot sends a data column sidecars by root request to one
// or more peers that can provide the needed data columns.
func RequestDataColumnSidecarsByRoot(
	ctx context.Context,
	dataColumns map[uint64]bool,
	block interfaces.ReadOnlySignedBeaconBlock,
	blkRoot [32]byte,
	peers []core.PeerID,
	clock *startup.Clock,
	p2p p2p.P2P,
	ctxMap ContextByteVersions,
	newColumnsVerifier verification.NewDataColumnsVerifier,
) ([]blocks.RODataColumn, error) {
	if len(dataColumns) == 0 {
		return nil, nil
	}

	// Assemble the peers who can provide the needed data columns.
	dataColumnsByAdmissiblePeer, _, _, err := AdmissiblePeersForDataColumns(peers, dataColumns, p2p)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't get admissible peers for data columns")
	}

	sidecars := make([]blocks.RODataColumn, 0, len(dataColumns))
	remainingColumns := make(map[uint64]bool, len(dataColumns))
	for col := range dataColumns {
		remainingColumns[col] = true
	}

	for len(dataColumnsByAdmissiblePeer) > 0 {
		peersToFetchFrom, err := SelectPeersToFetchDataColumnsFrom(remainingColumns, dataColumnsByAdmissiblePeer)
		if err != nil {
			// Return an error if some columns are unavailable from the filtered set
			// of peers. Filtering out bad peers can make columns unavailable, and
			// when that happens, the caller needs to know about it.
			return nil, errors.Wrap(err, "couldn't select peers to fetch data columns from")
		}

		// Request the data columns from each peer
		successfulColumns := make(map[uint64]bool)
		for peer, dataColumns := range peersToFetchFrom {
			request, err := RequestsForDataColumnsByRoot(blkRoot, dataColumns)
			if err != nil {
				log.WithError(err).Debug("Failed to build request for data columns")
				continue
			}

			peerSidecars, err := SendDataColumnSidecarsByRootRequest(ctx, clock, p2p, peer, ctxMap, &request)
			if err != nil {
				// Remove this peer since it failed to respond correctly
				delete(dataColumnsByAdmissiblePeer, peer)
				log.WithFields(logrus.Fields{
					"peer":      peer.String(),
					"blockRoot": fmt.Sprintf("%#x", blkRoot),
					"error":     err.Error(),
				}).Debug("Failed to request data columns from peer")
				continue
			}

			// Mark columns as successful
			for _, sidecar := range peerSidecars {
				colIndex := sidecar.ColumnIndex
				successfulColumns[colIndex] = true
			}

			for _, colIndex := range dataColumns {
				if !successfulColumns[colIndex] {
					// Remove this peer if any requested column wasn't successful
					delete(dataColumnsByAdmissiblePeer, peer)
					log.WithFields(logrus.Fields{
						"peer":          peer.String(),
						"missingColumn": colIndex,
					}).Debug("Peer failed to return requested data column")
					break
				}
			}

			sidecars = append(sidecars, peerSidecars...)
		}

		// Update remaining columns for the next retry
		for col := range successfulColumns {
			delete(remainingColumns, col)
		}

		if len(remainingColumns) > 0 {
			// Some columns are still missing, retry with the remaining peers.
			continue
		}

		// All columns have been successfully retrieved, validate the received sidecars.
		roBlock, err := blocks.NewROBlock(block)
		if err != nil {
			return nil, err
		}

		wrappedBlockDataColumns := make([]verify.WrappedBlockDataColumn, 0, len(sidecars))
		for _, sidecar := range sidecars {
			wrappedBlockDataColumn := verify.WrappedBlockDataColumn{
				ROBlock:      roBlock.Block(),
				RODataColumn: sidecar,
			}

			wrappedBlockDataColumns = append(wrappedBlockDataColumns, wrappedBlockDataColumn)
		}

		if err := verify.DataColumnsAlignWithBlock(wrappedBlockDataColumns, newColumnsVerifier); err != nil {
			return nil, errors.Wrap(err, "data columns align with block")
		}

		for _, sidecar := range sidecars {
			log.WithFields(logging.DataColumnFields(sidecar)).Debug("Received data column sidecar RPC")
		}

		return sidecars, nil
	}

	// If we still have remaining columns after all retries, return error
	return nil, errors.Errorf("failed to retrieve all requested data columns after retries for block root=%#x, missing columns=%v", blkRoot, uint64MapToSortedSlice(remainingColumns))
}

// SaveDataColumns saves the received data columns to disk.
//
// NOTE: During the initial sync, LazilyPersistentStoreColumn caches sidecars
// and saves them to disk within IsDataAvailable. SaveDataColumns is intended
// for use when no caching is done (e.g. in the pending blocks queue).
func SaveDataColumns(sidecars []blocks.RODataColumn, dataColumnStorage *filesystem.DataColumnStorage) error {
	verifiedRODataColumns := make([]blocks.VerifiedRODataColumn, 0, len(sidecars))
	for _, sidecar := range sidecars {
		verifiedRODataColumn := blocks.NewVerifiedRODataColumn(sidecar)
		verifiedRODataColumns = append(verifiedRODataColumns, verifiedRODataColumn)
	}

	if err := dataColumnStorage.Store(verifiedRODataColumns); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	return nil
}

// FindMissingDataColumns looks at the data columns we should sample from and have via custody sampling
// and that we don't actually store for a given block, and returns the corresponding data column indices.
func FindMissingDataColumns(
	root [32]byte,
	block interfaces.ReadOnlySignedBeaconBlock,
	nodeID enode.ID,
	custodyGroupCount uint64,
	dataColumnStorage *filesystem.DataColumnStorage,
) (map[uint64]bool, error) {
	// Blocks before Fulu have no data columns.
	if block.Version() < version.Fulu {
		return nil, nil
	}

	// Get the blob commitments from the block.
	commitments, err := block.Block().Body().BlobKzgCommitments()
	if err != nil {
		return nil, errors.Wrap(err, "blob KZG commitments")
	}

	// Nothing to build if there are no commitments.
	if len(commitments) == 0 {
		return nil, nil
	}

	// Retrieve the columns we store for the root.
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	summary := dataColumnStorage.Summary(root)

	storedColumns := make(map[uint64]bool, numberOfColumns)
	for i := range numberOfColumns {
		if summary.HasIndex(i) {
			storedColumns[i] = true
		}
	}

	// Retrieve the peer info.
	peerInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}

	samplingColumns := peerInfo.CustodyColumns

	// Build the request for the columns we should sample from and we don't actually store.
	missingColumns := make(map[uint64]bool, len(samplingColumns))
	for column := range samplingColumns {
		if !storedColumns[column] {
			missingColumns[column] = true
		}
	}

	return missingColumns, nil
}

func RequestsForDataColumnsByRoot(
	root [32]byte,
	missingColumns []uint64,
) (types.DataColumnSidecarsByRootReq, error) {
	req := make(types.DataColumnSidecarsByRootReq, 0, len(missingColumns))
	for _, column := range missingColumns {
		req = append(req, &eth.DataColumnIdentifier{
			BlockRoot:   root[:],
			ColumnIndex: column,
		})
	}

	return req, nil
}

// SelectPeersToFetchDataColumnsFrom implements greedy algorithm in order to select peers to fetch data columns from.
// https://en.wikipedia.org/wiki/Set_cover_problem#Greedy_algorithm
func SelectPeersToFetchDataColumnsFrom(
	neededDataColumns map[uint64]bool,
	dataColumnsByPeer map[peer.ID]map[uint64]bool,
) (map[peer.ID][]uint64, error) {
	// Copy the provided needed data columns into a set that we will remove elements from.
	remainingDataColumns := make(map[uint64]bool, len(neededDataColumns))
	for dataColumn := range neededDataColumns {
		remainingDataColumns[dataColumn] = true
	}

	dataColumnsFromSelectedPeers := make(map[peer.ID][]uint64)

	// Filter `dataColumnsByPeer` to only contain needed data columns.
	neededDataColumnsByPeer := make(map[peer.ID]map[uint64]bool, len(dataColumnsByPeer))
	for pid, dataColumns := range dataColumnsByPeer {
		for dataColumn := range dataColumns {
			if remainingDataColumns[dataColumn] {
				if _, ok := neededDataColumnsByPeer[pid]; !ok {
					neededDataColumnsByPeer[pid] = make(map[uint64]bool, len(neededDataColumns))
				}

				neededDataColumnsByPeer[pid][dataColumn] = true
			}
		}
	}

	maxRequestDataColumnSidecars := params.BeaconConfig().MaxRequestDataColumnSidecars

	for len(remainingDataColumns) > 0 {
		// Check if at least one peer remains. If not, it means that we don't have enough peers to fetch all needed data columns.
		if len(neededDataColumnsByPeer) == 0 {
			missingDataColumnsSortedSlice := uint64MapToSortedSlice(remainingDataColumns)
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
		if uint64(len(dataColumnsSortedSlice)) > maxRequestDataColumnSidecars {
			dataColumnsSortedSlice = dataColumnsSortedSlice[:maxRequestDataColumnSidecars]
		}
		dataColumnsFromSelectedPeers[bestPeer] = dataColumnsSortedSlice

		// Remove the selected peer from the list of peers.
		delete(neededDataColumnsByPeer, bestPeer)

		// Remove the selected peer's data columns from the list of remaining data columns.
		for _, dataColumn := range dataColumnsSortedSlice {
			delete(remainingDataColumns, dataColumn)
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

// AdmissiblePeersForCustodyGroup returns a map of peers that custody at least one custody group listed in `neededCustodyGroups`.
//
// It returns:
// - A map, where the key of the map is the peer, the value is the custody groups of the peer.
// - A map, where the key of the map is the custody group, the value is a list of peers that custody the group.
// - A slice of descriptions for non admissible peers.
// - An error if any.
//
// NOTE: distributeSamplesToPeer from the DataColumnSampler implements similar logic,
// but with only one column queried in each request.
func AdmissiblePeersForDataColumns(
	peers []peer.ID,
	neededDataColumns map[uint64]bool,
	p2p p2p.P2P,
) (map[peer.ID]map[uint64]bool, map[uint64][]peer.ID, []string, error) {
	peerCount := len(peers)
	neededDataColumnsCount := uint64(len(neededDataColumns))

	// Create description slice for non admissible peers.
	descriptions := make([]string, 0, peerCount)

	// Compute custody columns for each peer.
	dataColumnsByPeer, err := custodyColumnsFromPeers(peers, p2p)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "custody columns from peers")
	}

	// Filter peers which custody at least one needed data column.
	dataColumnsByAdmissiblePeer, localDescriptions := filterPeerWhichCustodyAtLeastOneDataColumn(neededDataColumns, dataColumnsByPeer)
	descriptions = append(descriptions, localDescriptions...)

	// Compute a map from needed data columns to their peers.
	admissiblePeersByDataColumn := make(map[uint64][]peer.ID, neededDataColumnsCount)
	for peerId, peerDataColumns := range dataColumnsByAdmissiblePeer {
		for dataColumn := range neededDataColumns {
			if peerDataColumns[dataColumn] {
				admissiblePeersByDataColumn[dataColumn] = append(admissiblePeersByDataColumn[dataColumn], peerId)
			}
		}
	}

	return dataColumnsByAdmissiblePeer, admissiblePeersByDataColumn, descriptions, nil
}

// custodyGroupsFromPeer computes all the custody groups indexed by peer.
func custodyGroupsFromPeers(peers []peer.ID, p2pIface p2p.P2P) (map[peer.ID]map[uint64]bool, error) {
	peerCount := len(peers)

	custodyGroupsByPeer := make(map[peer.ID]map[uint64]bool, peerCount)
	for _, peer := range peers {
		// Get the node ID from the peer ID.
		nodeID, err := p2p.ConvertPeerIDToNodeID(peer)
		if err != nil {
			return nil, errors.Wrap(err, "convert peer ID to node ID")
		}

		// Get the custody group count of the peer.
		custodyGroupCount := p2pIface.CustodyGroupCountFromPeer(peer)

		// Get the custody groups of the peer.
		dasInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
		if err != nil {
			return nil, errors.Wrap(err, "custody groups")
		}

		custodyGroupsByPeer[peer] = dasInfo.CustodyGroups
	}

	return custodyGroupsByPeer, nil
}

// custodyColumnsFromPeers computes all the custody columns indexed by peer.
func custodyColumnsFromPeers(peers []peer.ID, p2p p2p.P2P) (map[peer.ID]map[uint64]bool, error) {
	// Get the custody groups of the peers.
	custodyGroupsByPeer, err := custodyGroupsFromPeers(peers, p2p)
	if err != nil {
		return nil, errors.Wrap(err, "custody groups from peer")
	}

	// Compute the custody columns of the peers.
	dataColumnsByPeer := make(map[peer.ID]map[uint64]bool, len(custodyGroupsByPeer))
	for peer, custodyGroups := range custodyGroupsByPeer {
		custodyColumns, err := peerdas.CustodyColumns(custodyGroups)
		if err != nil {
			return nil, errors.Wrap(err, "custody columns")
		}

		dataColumnsByPeer[peer] = custodyColumns
	}

	return dataColumnsByPeer, nil
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
