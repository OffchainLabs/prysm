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
func (s *Service) AdmissiblePeersForDataColumns(
	peers []peer.ID,
	neededDataColumns map[uint64]bool,
) (map[peer.ID]map[uint64]bool, map[uint64][]peer.ID, []string, error) {
	peerCount := len(peers)
	neededDataColumnsCount := uint64(len(neededDataColumns))

	// Create description slice for non admissible peers.
	descriptions := make([]string, 0, peerCount)

	// Compute custody columns for each peer.
	dataColumnsByPeer, err := s.custodyColumnsFromPeers(peers)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "couldn't get custody columns from peers")
	}

	// Filter peers which custody at least one needed data column.
	dataColumnsByAdmissiblePeer, localDescriptions := filterPeerWhichCustodyAtLeastOneDataColumn(neededDataColumns, dataColumnsByPeer)
	descriptions = append(descriptions, localDescriptions...)

	// Compute a map from needed data columns to their peers.
	admissiblePeersByDataColumn := make(map[uint64][]peer.ID, neededDataColumnsCount)
	for peer := range dataColumnsByAdmissiblePeer {
		for dataColumn := range neededDataColumns {
			if dataColumnsByAdmissiblePeer[peer][dataColumn] {
				admissiblePeersByDataColumn[dataColumn] = append(admissiblePeersByDataColumn[dataColumn], peer)
			}
		}
	}

	return dataColumnsByAdmissiblePeer, admissiblePeersByDataColumn, descriptions, nil
}

// custodyGroupsFromPeer computes all the custody groups indexed by peer.
func (s *Service) custodyGroupsFromPeers(peers []peer.ID) (map[peer.ID]map[uint64]bool, error) {
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
		dasInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
		if err != nil {
			return nil, errors.Wrap(err, "custody groups")
		}

		custodyGroupsByPeer[peer] = dasInfo.CustodyGroups
	}

	return custodyGroupsByPeer, nil
}

func (s *Service) custodyColumnsFromPeers(peers []peer.ID) (map[peer.ID]map[uint64]bool, error) {
	custodyGroupsByPeer, err := s.custodyGroupsFromPeers(peers)
	if err != nil {
		return nil, errors.Wrap(err, "custody groups from peer")
	}

	return convertCustodyGroupsToDataColumnsByPeer(custodyGroupsByPeer)
}

func convertCustodyGroupsToDataColumnsByPeer(custodyGroupsByPeer map[peer.ID]map[uint64]bool) (map[peer.ID]map[uint64]bool, error) {
	dataColumnsByPeer := make(map[peer.ID]map[uint64]bool, len(custodyGroupsByPeer))
	for peer, custodyGroups := range custodyGroupsByPeer {
		custodyColumns, err := peerdas.CustodyColumns(custodyGroups)
		if err != nil {
			return nil, errors.Wrap(err, "couldn't get custody columns from groups")
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

// uint64MapToSortedSlice produces a sorted uint64 slice from a map.
func uint64MapToSortedSlice(input map[uint64]bool) []uint64 {
	output := make([]uint64, 0, len(input))
	for idx := range input {
		output = append(output, idx)
	}

	slices.Sort[[]uint64](output)
	return output
}
