package testing

import (
	"context"
	"errors"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/gossipsubcrawler"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// MockPeerManager is mock of the PeerManager interface.
type MockPeerManager struct {
	Enr               *enr.Record
	PID               peer.ID
	BHost             host.Host
	DiscoveryAddr     []multiaddr.Multiaddr
	FailDiscoveryAddr bool
}

// Disconnect .
func (*MockPeerManager) Disconnect(peer.ID) error {
	return nil
}

// PeerID .
func (m *MockPeerManager) PeerID() peer.ID {
	return m.PID
}

// Host .
func (m *MockPeerManager) Host() host.Host {
	return m.BHost
}

// ENR .
func (m *MockPeerManager) ENR() *enr.Record {
	return m.Enr
}

// NodeID .
func (m MockPeerManager) NodeID() enode.ID {
	return enode.ID{}
}

// DiscoveryAddresses .
func (m *MockPeerManager) DiscoveryAddresses() ([]multiaddr.Multiaddr, error) {
	if m.FailDiscoveryAddr {
		return nil, errors.New("fail")
	}
	return m.DiscoveryAddr, nil
}

// RefreshPersistentSubnets .
func (*MockPeerManager) RefreshPersistentSubnets() {}

// FindAndDialPeersWithSubnet .
func (*MockPeerManager) FindAndDialPeersWithSubnets(ctx context.Context, fullTopicForSubnet func(uint64) string, minimumPeersPerSubnet int, subnets map[uint64]bool) error {
	return nil
}

// AddPingMethod .
func (*MockPeerManager) AddPingMethod(_ func(ctx context.Context, id peer.ID) error) {}

// Crawler.
func (*MockPeerManager) Crawler() gossipsubcrawler.Crawler {
	return nil
}
