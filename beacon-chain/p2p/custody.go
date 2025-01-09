package p2p

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/config/params"
)

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

	// Retrieve the needed custody groups.
	neededCustodyGroups, err := peerdas.CustodyGroups(localNodeId, custodyGroupCount)
	if err != nil {
		return nil, errors.Wrap(err, "custody groups")
	}

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

		// Get the custody groups of the remote peer.
		remoteCustodyGroups, err := peerdas.CustodyGroups(remoteNodeID, remoteCustodyGroupCount)
		if err != nil {
			return nil, errors.Wrap(err, "custody groups")
		}

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
