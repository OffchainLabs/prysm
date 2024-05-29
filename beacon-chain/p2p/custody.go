package p2p

import (
	ssz "github.com/ferranbt/fastssz"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/sirupsen/logrus"
)

func (s *Service) GetValidCustodyPeers(peers []peer.ID) ([]peer.ID, error) {
	custodiedSubnetCount := params.BeaconConfig().CustodyRequirement
	if flags.Get().SubscribeToAllSubnets {
		custodiedSubnetCount = params.BeaconConfig().DataColumnSidecarSubnetCount
	}
	custodiedColumns, err := peerdas.CustodyColumns(s.NodeID(), custodiedSubnetCount)
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
	custodyObj := CustodySubnetCount(make([]byte, 8))
	if err := peerRecord.Load(&custodyObj); err != nil {
		log.WithField("peerID", pid).Error("Cannot load the custody_subnet_count from peer")
		return peerCustodyCountCount
	}

	// Unmarshal the custody count from the peer's ENR.
	peerCustodyCountFromRecord := ssz.UnmarshallUint64(custodyObj)
	log.WithFields(logrus.Fields{
		"peerID":       pid,
		"custodyCount": peerCustodyCountFromRecord,
	}).Debug("Custody count read from peer's ENR")

	return peerCustodyCountFromRecord
}
