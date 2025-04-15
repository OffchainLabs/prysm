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
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/sirupsen/logrus"
)

// RequestDataColumnSidecarsByRoot carefully selects, among `peers`,
// the peers that can provide the requested data columns, requests them
// and verify them according to `newColumnsVerifier` rules.
func RequestDataColumnSidecarsByRoot(
	ctx context.Context,
	dataColumnsToFetch map[uint64]bool,
	block blocks.ROBlock,
	peers []core.PeerID,
	clock *startup.Clock,
	p2p p2p.P2P,
	ctxMap ContextByteVersions,
	newColumnsVerifier verification.NewDataColumnsVerifier,
) ([]blocks.VerifiedRODataColumn, error) {
	if len(dataColumnsToFetch) == 0 {
		return nil, nil
	}

	// Assemble the peers who can provide the needed data columns.
	dataColumnsByAdmissiblePeer, _, _, err := AdmissiblePeersForDataColumns(peers, dataColumnsToFetch, p2p)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't get admissible peers for data columns")
	}

	verifiedSidecars := make([]blocks.VerifiedRODataColumn, 0, len(dataColumnsToFetch))
	remainingMissingColumns := make(map[uint64]bool, len(dataColumnsToFetch))
	for column := range dataColumnsToFetch {
		remainingMissingColumns[column] = true
	}

	blockRoot := block.Root()

	for len(dataColumnsByAdmissiblePeer) > 0 {
		peersToFetchFrom, err := SelectPeersToFetchDataColumnsFrom(remainingMissingColumns, dataColumnsByAdmissiblePeer)
		if err != nil {
			return nil, errors.Wrap(err, "select peers to fetch data columns from")
		}

		// Request the data columns from each peer.
		successfulColumns := make(map[uint64]bool, len(remainingMissingColumns))
		for peer, peerRequestedColumns := range peersToFetchFrom {
			log := log.WithFields(logrus.Fields{"peer": peer.String(), "blockRoot": fmt.Sprintf("%#x", blockRoot)})

			// Build the requests for the data columns.
			byRootRequests := make(types.DataColumnSidecarsByRootReq, 0, len(peerRequestedColumns))
			for _, column := range peerRequestedColumns {
				byRootRequest := &eth.DataColumnIdentifier{BlockRoot: blockRoot[:], Index: column}
				byRootRequests = append(byRootRequests, byRootRequest)
			}

			// Send the requests to the peer.
			peerSidecars, err := SendDataColumnSidecarsByRootRequest(ctx, clock, p2p, peer, ctxMap, &byRootRequests)
			if err != nil {
				// Remove this peer since it failed to respond correctly.
				delete(dataColumnsByAdmissiblePeer, peer)

				log.WithFields(logrus.Fields{
					"peer":      peer.String(),
					"blockRoot": fmt.Sprintf("%#x", block.Root()),
				}).WithError(err).Debug("Failed to request data columns from peer")

				continue
			}

			// Check if returned data columns align with the block.
			if err := verify.DataColumnsAlignWithBlock(block, peerSidecars); err != nil {
				// Remove this peer since it failed to respond correctly.
				delete(dataColumnsByAdmissiblePeer, peer)
				log.WithError(err).Debug("Align with block failed")
				continue
			}

			// Verify the received sidecars.
			verifier := newColumnsVerifier(peerSidecars, verification.ByRootRequestDataColumnSidecarRequirements)

			if err := verifier.Valid(); err != nil {
				// Remove this peer if the verification failed.
				delete(dataColumnsByAdmissiblePeer, peer)
				log.WithError(err).Debug("Valid verification failed")
				continue
			}

			if err := verifier.SidecarInclusionProven(); err != nil {
				// Remove this peer if the verification failed.
				delete(dataColumnsByAdmissiblePeer, peer)
				log.WithError(err).Debug("Sidecar inclusion proof verification failed")
				continue
			}

			if err := verifier.SidecarKzgProofVerified(); err != nil {
				// Remove this peer if the verification failed.
				delete(dataColumnsByAdmissiblePeer, peer)
				log.WithError(err).Debug("Sidecar KZG proof verification failed")
				continue
			}

			// Upgrade the sidecars to verified sidecars.
			verifiedPeerSidecars, err := verifier.VerifiedRODataColumns()
			if err != nil {
				// This should never happen.
				return nil, errors.Wrap(err, "verified data columns")
			}

			// Mark columns as successful
			for _, sidecar := range verifiedPeerSidecars {
				successfulColumns[sidecar.Index] = true
			}

			// Check if all requested columns were successfully returned.
			peerMissingColumns := make(map[uint64]bool)
			for _, index := range peerRequestedColumns {
				if !successfulColumns[index] {
					peerMissingColumns[index] = true
				}
			}

			if len(peerMissingColumns) > 0 {
				// Remove this peer if some requested columns were not correctly returned.
				delete(dataColumnsByAdmissiblePeer, peer)
				log.WithField("missingColumns", uint64MapToSortedSlice(peerMissingColumns)).Debug("Peer did not provide all requested data columns")
			}

			verifiedSidecars = append(verifiedSidecars, verifiedPeerSidecars...)
		}

		// Update remaining columns for the next retry.
		for col := range successfulColumns {
			delete(remainingMissingColumns, col)
		}

		if len(remainingMissingColumns) > 0 {
			// Some columns are still missing, retry with the remaining peers.
			continue
		}

		return verifiedSidecars, nil
	}

	// If we still have remaining columns after all retries, return error
	return nil, errors.Errorf("failed to retrieve all requested data columns after retries for block root=%#x, missing columns=%v", blockRoot, uint64MapToSortedSlice(remainingMissingColumns))
}

// SaveDataColumns saves the received data columns to disk.
//
// NOTE: During the initial sync, LazilyPersistentStoreColumn caches sidecars
// and saves them to disk within IsDataAvailable. SaveDataColumns is intended
// for use when no caching is done (e.g. in the pending blocks queue).
func SaveDataColumns(sidecars []blocks.VerifiedRODataColumn, dataColumnStorage *filesystem.DataColumnStorage) error {
	if err := dataColumnStorage.Save(sidecars); err != nil {
		return errors.Wrap(err, "save data column sidecars")
	}

	return nil
}

// MissingDataColumns looks at the data columns we should store for a given block regarding `custodyGroupCount`,
// and returns the indices of the missing ones.
func MissingDataColumns(block blocks.ROBlock, nodeID enode.ID, custodyGroupCount uint64, dataColumnStorage *filesystem.DataColumnStorage) (map[uint64]bool, error) {
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

	// Compute the expected columns.
	peerInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}

	expectedColumns := peerInfo.CustodyColumns

	// Get the stored columns.
	numberOfColumns := params.BeaconConfig().NumberOfColumns
	summary := dataColumnStorage.Summary(block.Root())

	storedColumns := make(map[uint64]bool, numberOfColumns)
	for i := range numberOfColumns {
		if summary.HasIndex(i) {
			storedColumns[i] = true
		}
	}

	// Compute the missing columns.
	missingColumns := make(map[uint64]bool, len(expectedColumns))
	for column := range expectedColumns {
		if !storedColumns[column] {
			missingColumns[column] = true
		}
	}

	return missingColumns, nil
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
