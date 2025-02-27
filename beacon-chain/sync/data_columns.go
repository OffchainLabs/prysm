package sync

import (
	"context"

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
)

// RequestDataColumnSidecars sends a data column sidecars by root request to one
// or more peers that can provide the needed data columns.
func RequestDataColumnSidecars(
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
	dataColumnsByAdmissiblePeer, _, _, err := p2p.AdmissiblePeersForDataColumns(peers, nil)
	if err != nil {
		return nil, err
	}
	peersToFetchFrom, err := SelectPeersToFetchDataColumnsFrom(dataColumns, dataColumnsByAdmissiblePeer)
	if err != nil {
		return nil, err
	}
	if len(peersToFetchFrom) == 0 {
		return nil, errors.Wrapf(errNoPeersForPending, "block root=%#x", blkRoot)
	}

	// Request the data columns from each peer.
	sidecars := make([]blocks.RODataColumn, 0, len(dataColumns))
	for peer, dataColumns := range peersToFetchFrom {
		dataColumnsSet := make(map[uint64]bool, len(dataColumns))
		for _, dataColumn := range dataColumns {
			dataColumnsSet[dataColumn] = true
		}
		request, err := RequestsForDataColumnsByRoot(blkRoot, dataColumnsSet)
		if err != nil {
			return nil, errors.Wrap(err, "build requests for missing data columns")
		}

		peerSidecars, err := SendDataColumnSidecarsByRootRequest(ctx, clock, p2p, peer, ctxMap, &request)
		if err != nil {
			return nil, err
		}
		sidecars = append(sidecars, peerSidecars...)
	}

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

// SaveDataColumns saves the received data columns to disk.
//
// NOTE: During the initial sync, LazilyPersistentStoreColumn caches sidecars
// and saves them to disk within IsDataAvailable. SaveDataColumns is intended
// for use when no caching is done (e.g. in the pending blocks queue).
func SaveDataColumns(sidecars []blocks.RODataColumn, blobStorage *filesystem.BlobStorage) error {
	for i := range sidecars {
		verfiedCol := blocks.NewVerifiedRODataColumn(sidecars[i])
		if err := blobStorage.SaveDataColumn(verfiedCol); err != nil {
			return err
		}
	}

	return nil
}

// FindMissingDataColumns looks at the data columns we should sample from and have via custody sampling
// and that we don't actually store for a given block, and returns the corresponding data column indices.
func FindMissingDataColumns(
	root [32]byte,
	block interfaces.ReadOnlySignedBeaconBlock,
	nodeID enode.ID,
	blobStorage *filesystem.BlobStorage,
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
	summary := blobStorage.Summary(root)

	storedColumns := make(map[uint64]bool, numberOfColumns)
	for i := range numberOfColumns {
		if summary.HasDataColumnIndex(i) {
			storedColumns[i] = true
		}
	}

	// Retrieve the number of groups we should sample from.
	samplingGroupSize := peerdas.CustodyGroupSamplingSize()

	// Retrieve the peer info.
	peerInfo, _, err := peerdas.Info(nodeID, samplingGroupSize)
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
	missingColumns map[uint64]bool,
) (types.DataColumnSidecarsByRootReq, error) {
	req := make(types.DataColumnSidecarsByRootReq, 0, len(missingColumns))
	for column := range missingColumns {
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
