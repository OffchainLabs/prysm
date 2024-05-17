package p2p

import (
	ssz "github.com/ferranbt/fastssz"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/peerdas"
	"github.com/prysmaticlabs/prysm/v5/cmd/beacon-chain/flags"
	"github.com/prysmaticlabs/prysm/v5/config/params"
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
		remoteCount, err := s.CustodyCountFromRemotePeer(pid)
		if err != nil {
			return nil, err
		}
		nodeId, err := ConvertPeerIDToNodeID(pid)
		if err != nil {
			return nil, err
		}
		remoteCustodiedColumns, err := peerdas.CustodyColumns(nodeId, remoteCount)
		if err != nil {
			return nil, err
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

func (s *Service) CustodyCountFromRemotePeer(pid peer.ID) (uint64, error) {
	// Retrieve the ENR of the peer.
	peerRecord, err := s.peers.ENR(pid)
	if err != nil {
		return 0, errors.Wrap(err, "ENR")
	}
	peerCustodiedSubnetCount := params.BeaconConfig().CustodyRequirement
	if peerRecord != nil {
		// Load the `custody_subnet_count`
		custodyBytes := make([]byte, 8)
		custodyObj := CustodySubnetCount(custodyBytes)
		if err := peerRecord.Load(&custodyObj); err != nil {
			return 0, errors.Wrap(err, "load custody_subnet_count")
		}
		actualCustodyCount := ssz.UnmarshallUint64(custodyBytes)

		if actualCustodyCount > peerCustodiedSubnetCount {
			peerCustodiedSubnetCount = actualCustodyCount
		}
	}
	return peerCustodiedSubnetCount, nil
}
