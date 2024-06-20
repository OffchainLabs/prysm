package p2p

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/config/params"
)

func (s *Service) GetValidCustodyPeers(peers []peer.ID) ([]peer.ID, error) {
	custodiedColumns, err := peerdas.CustodyColumns(s.NodeID(), peerdas.CustodySubnetCount())
	if err != nil {
		return nil, err
	}
	var validPeers []peer.ID
	for _, pid := range peers {
		remoteCount := s.CustodyCountFromRemotePeer(pid)

		nodeId, err := ConvertPeerIDToNodeID(pid)
		if err != nil {
			return nil, errors.Wrap(err, "convert peer ID to node ID")
		}
		remoteCustodiedColumns, err := peerdas.CustodyColumns(nodeId, remoteCount)
		if err != nil {
			return nil, errors.Wrap(err, "custody columns")
		}
		invalidPeer := false
		for c := range custodiedColumns {
			if !remoteCustodiedColumns[c] {
				invalidPeer = true
				break
			}
		}
		if invalidPeer {
			continue
		}
		copiedId := pid
		// Add valid peer to list
		validPeers = append(validPeers, copiedId)
	}
	return validPeers, nil
}

func (s *Service) CustodyCountFromRemotePeer(pid peer.ID) uint64 {
	// By default, we assume the peer custodies the minimum number of subnets.
	peerCustodyCountCount := params.BeaconConfig().CustodyRequirement

	// Retrieve the ENR of the peer.
	peerRecord, err := s.peers.ENR(pid)
	if err != nil {
		log.WithError(err).WithField("peerID", pid).Error("Failed to retrieve ENR for peer")
		return peerCustodyCountCount
	}

	if peerRecord == nil {
		// This is the case for inbound peers. So we don't log an error for this.
		log.WithField("peerID", pid).Debug("No ENR found for peer")
		return peerCustodyCountCount
	}

	// Load the `custody_subnet_count`
	var csc CustodySubnetCount
	if err := peerRecord.Load(&csc); err != nil {
		log.WithField("peerID", pid).Error("Cannot load the custody_subnet_count from peer")
		return peerCustodyCountCount
	}

	log.WithFields(logrus.Fields{
		"peerID":       pid,
		"custodyCount": csc,
	}).Debug("Custody count read from peer's ENR")

	return uint64(csc)
}
