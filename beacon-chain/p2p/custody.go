package p2p

import (
	"fmt"
	"slices"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/config/params"
)

var _ DataColumnsHandler = (*Service)(nil)

// AdmissibleCustodyGroupsPeers returns a list of peers that custody a super set of the local node's custody groups.
func (s *Service) AdmissibleCustodyGroupsPeers(peers []peer.ID) ([]peer.ID, error) {
	localCustodyGroupCount := peerdas.CustodyGroupCount()
	return s.custodyGroupsAdmissiblePeers(peers, localCustodyGroupCount)
}

// AdmissibleCustodySamplingPeers returns a list of peers that custody a super set of the local node's sampling columns.
func (s *Service) AdmissibleCustodySamplingPeers(peers []peer.ID) ([]peer.ID, error) {
	localSubnetSamplingSize := peerdas.CustodyGroupSamplingSize()
	return s.custodyGroupsAdmissiblePeers(peers, localSubnetSamplingSize)
}

// custodyGroupsAdmissiblePeers filters out `peers` that do not custody a super set of our own custody groups.
func (s *Service) custodyGroupsAdmissiblePeers(peers []peer.ID, custodyGroupCount uint64) ([]peer.ID, error) {
	// Get the total number of custody groups.
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups

	// Retrieve the local node ID.
	localNodeId := s.NodeID()

	// Retrieve the local node info.
	localNodeInfo, _, err := peerdas.Info(localNodeId, custodyGroupCount)
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}

	// Retrieve the needed custody groups.
	neededCustodyGroups := localNodeInfo.CustodyGroups

	// Find the valid peers.
	validPeers := make([]peer.ID, 0, len(peers))

loop:
	for _, pid := range peers {
		// Get the custody group count of the remote peer.
		remoteCustodyGroupCount := s.CustodyGroupCountFromPeer(pid)

		// If the remote peer custodies less groups than we do, skip it.
		if remoteCustodyGroupCount < custodyGroupCount {
			continue
		}

		// Get the remote node ID from the peer ID.
		remoteNodeID, err := ConvertPeerIDToNodeID(pid)
		if err != nil {
			return nil, errors.Wrap(err, "convert peer ID to node ID")
		}

		// Retrieve the remote peer info.
		remotePeerInfo, _, err := peerdas.Info(remoteNodeID, remoteCustodyGroupCount)
		if err != nil {
			return nil, errors.Wrap(err, "peer info")
		}

		// Retrieve the custody groups of the remote peer.
		remoteCustodyGroups := remotePeerInfo.CustodyGroups
		remoteCustodyGroupsCount := uint64(len(remoteCustodyGroups))

		// If the remote peers custodies all the possible columns, add it to the list.
		if remoteCustodyGroupsCount == numberOfCustodyGroups {
			validPeers = append(validPeers, pid)
			continue
		}

		// Filter out invalid peers.
		for custodyGroup := range neededCustodyGroups {
			if !remoteCustodyGroups[custodyGroup] {
				continue loop
			}
		}

		// Add valid peer to list
		validPeers = append(validPeers, pid)
	}

	return validPeers, nil
}

// custodyGroupCountFromPeerENR retrieves the custody count from the peer ENR.
// If the ENR is not available, it defaults to the minimum number of custody groups
// an honest node custodies and serves samples from.
func (s *Service) custodyGroupCountFromPeerENR(pid peer.ID) uint64 {
	// By default, we assume the peer custodies the minimum number of groups.
	custodyRequirement := params.BeaconConfig().CustodyRequirement

	// Retrieve the ENR of the peer.
	record, err := s.peers.ENR(pid)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"peerID":       pid,
			"defaultValue": custodyRequirement,
		}).Debug("Failed to retrieve ENR for peer, defaulting to the default value")

		return custodyRequirement
	}

	// Retrieve the custody group count from the ENR.
	custodyGroupCount, err := peerdas.CustodyGroupCountFromRecord(record)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"peerID":       pid,
			"defaultValue": custodyRequirement,
		}).Debug("Failed to retrieve custody group count from ENR for peer, defaulting to the default value")

		return custodyRequirement
	}

	return custodyGroupCount
}

// CustodyGroupCountFromPeer retrieves custody group count from a peer.
// It first tries to get the custody group count from the peer's metadata,
// then falls back to the ENR value if the metadata is not available, then
// falls back to the minimum number of custody groups an honest node should custodiy
// and serve samples from if ENR is not available.
func (s *Service) CustodyGroupCountFromPeer(pid peer.ID) uint64 {
	// Try to get the custody group count from the peer's metadata.
	metadata, err := s.peers.Metadata(pid)
	if err != nil {
		// On error, default to the ENR value.
		log.WithError(err).WithField("peerID", pid).Debug("Failed to retrieve metadata for peer, defaulting to the ENR value")
		return s.custodyGroupCountFromPeerENR(pid)
	}

	// If the metadata is nil, default to the ENR value.
	if metadata == nil {
		log.WithField("peerID", pid).Debug("Metadata is nil, defaulting to the ENR value")
		return s.custodyGroupCountFromPeerENR(pid)
	}

	// Get the custody subnets count from the metadata.
	custodyCount := metadata.CustodyGroupCount()

	// If the custody count is null, default to the ENR value.
	if custodyCount == 0 {
		log.WithField("peerID", pid).Debug("The custody count extracted from the metadata equals to 0, defaulting to the ENR value")
		return s.custodyGroupCountFromPeerENR(pid)
	}

	return custodyCount
}

// AdmissiblePeersForCustodyGroup returns a map of peers that:
// - custody at least one custody group listed in `neededCustodyGroups`,
//
// It returns:
// - A map, where the key of the map is the peer, the value is the custody groups of the peer.
// - A map, where the key of the map is the custody group, the value is the peer that custodies the group.
// - A slice of descriptions for non admissible peers.
// - An error if any.

func (s *Service) AdmissiblePeersForCustodyGroups(
	peers []peer.ID,
	neededCustodyGroups map[uint64]bool,
) (map[peer.ID]map[uint64]bool, map[uint64][]peer.ID, []string, error) {
	peerCount := len(peers)
	neededCustodyGroupCount := uint64(len(neededCustodyGroups))

	// Create description slice for non admissible peers.
	descriptions := make([]string, 0, peerCount)

	// Compute custody groups for each peer.
	dataColumnsByPeer, err := s.custodyGroupsFromPeer(peers)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "custody columns from peer")
	}

	// Filter peers which custody at least one needed data column.
	dataColumnsByAdmissiblePeer, localDescriptions := filterPeerWhichCustodyAtLeastOneDataColumn(neededCustodyGroups, dataColumnsByPeer)
	descriptions = append(descriptions, localDescriptions...)

	// Compute a map from needed data columns to their peers.
	admissiblePeersByDataColumn := make(map[uint64][]peer.ID, neededCustodyGroupCount)
	for peer, peerCustodyDataColumns := range dataColumnsByAdmissiblePeer {
		for dataColumn := range peerCustodyDataColumns {
			admissiblePeersByDataColumn[dataColumn] = append(admissiblePeersByDataColumn[dataColumn], peer)
		}
	}

	return dataColumnsByAdmissiblePeer, admissiblePeersByDataColumn, descriptions, nil
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

// custodyGroupsFromPeer compute all the custody groups indexed by peer.
func (s *Service) custodyGroupsFromPeer(peers []peer.ID) (map[peer.ID]map[uint64]bool, error) {
	peerCount := len(peers)

	custodyGroupsByPeer := make(map[peer.ID]map[uint64]bool, peerCount)
	for _, peer := range peers {
		// Get the node ID from the peer ID.
		nodeID, err := ConvertPeerIDToNodeID(peer)
		if err != nil {
			return nil, errors.Wrap(err, "convert peer ID to node ID")
		}

		// Get the custody group count of the peer.
		custodyGroupCount := s.CustodyGroupCountFromPeer(peer)

		// Get the custody groups of the peer.
		custodyGroups, err := peerdas.CustodyGroups(nodeID, custodyGroupCount)
		if err != nil {
			return nil, errors.Wrap(err, "custody groups")
		}

		custodyGroupsByPeer[peer] = custodyGroups
	}

	return custodyGroupsByPeer, nil
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

// uint64MapToSortedSlice produces a sorted uint64 slice from a map.
func uint64MapToSortedSlice(input map[uint64]bool) []uint64 {
	output := make([]uint64, 0, len(input))
	for idx := range input {
		output = append(output, idx)
	}

	slices.Sort[[]uint64](output)
	return output
}
