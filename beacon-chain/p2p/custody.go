package p2p

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/sirupsen/logrus"
)

var _ DataColumnsHandler = (*Service)(nil)

// CustodyGroupCountFromPeer retrieves custody group count from a peer.
// It first tries to get the custody group count from the peer's metadata,
// then falls back to the ENR value if the metadata is not available, then
// falls back to the minimum number of custody groups an honest node should custodiy
// and serve samples from if ENR is not available.
func (s *Service) CustodyGroupCountFromPeer(pid peer.ID) uint64 {
	log := log.WithField("peerID", pid)
	// Try to get the custody group count from the peer's metadata.
	metadata, err := s.peers.Metadata(pid)
	if err != nil {
		// On error, default to the ENR value.
		log.WithError(err).Debug("Failed to retrieve metadata for peer, defaulting to the ENR value")
		return s.custodyGroupCountFromPeerENR(pid)
	}

	// If the metadata is nil, default to the ENR value.
	if metadata == nil {
		log.Debug("Metadata is nil, defaulting to the ENR value")
		return s.custodyGroupCountFromPeerENR(pid)
	}

	// Get the custody subnets count from the metadata.
	custodyCount := metadata.CustodyGroupCount()

	// If the custody count is null, default to the ENR value.
	if custodyCount == 0 {
		log.Debug("The custody count extracted from the metadata equals to 0, defaulting to the ENR value")
		return s.custodyGroupCountFromPeerENR(pid)
	}

	return custodyCount
}

// custodyGroupCountFromPeerENR retrieves the custody count from the peer's ENR.
// If the ENR is not available, it defaults to the minimum number of custody groups
// an honest node custodies and serves samples from.
func (s *Service) custodyGroupCountFromPeerENR(pid peer.ID) uint64 {
	// By default, we assume the peer custodies the minimum number of groups.
	custodyRequirement := params.BeaconConfig().CustodyRequirement

	log := log.WithFields(logrus.Fields{
		"peerID":       pid,
		"defaultValue": custodyRequirement,
	})

	// Retrieve the ENR of the peer.
	record, err := s.peers.ENR(pid)
	if err != nil {
		log.WithError(err).Debug("Failed to retrieve ENR for peer, defaulting to the default value")

		return custodyRequirement
	}

	// Retrieve the custody group count from the ENR.
	custodyGroupCount, err := peerdas.CustodyGroupCountFromRecord(record)
	if err != nil {
		log.WithError(err).Debug("Failed to retrieve custody group count from ENR for peer, defaulting to the default value")

		return custodyRequirement
	}

	return custodyGroupCount
}
