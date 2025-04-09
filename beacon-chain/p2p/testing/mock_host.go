package testing

import (
	"context"

	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
)

// MockHost is a fake implementation of libp2p2's Host interface.
type MockHost struct {
	Addresses []ma.Multiaddr
}

// ID --
func (*MockHost) ID() peer.ID {
	return ""
}

// Peerstore --
func (*MockHost) Peerstore() peerstore.Peerstore {
	return nil
}

// Addrs --
func (m *MockHost) Addrs() []ma.Multiaddr {
	return m.Addresses
}

// Network --
func (*MockHost) Network() network.Network {
	return nil
}

// Mux --
func (*MockHost) Mux() protocol.Switch {
	return nil
}

// Connect --
func (*MockHost) Connect(_ context.Context, _ peer.AddrInfo) error {
	return nil
}

// SetStreamHandler --
func (*MockHost) SetStreamHandler(_ protocol.ID, _ network.StreamHandler) {}

// SetStreamHandlerMatch --
func (*MockHost) SetStreamHandlerMatch(protocol.ID, func(id protocol.ID) bool, network.StreamHandler) {
}

// RemoveStreamHandler --
func (*MockHost) RemoveStreamHandler(_ protocol.ID) {}

// NewStream --
func (*MockHost) NewStream(_ context.Context, _ peer.ID, _ ...protocol.ID) (network.Stream, error) {
	return nil, nil
}

// Close --
func (*MockHost) Close() error {
	return nil
}

// ConnManager --
func (*MockHost) ConnManager() connmgr.ConnManager {
	return nil
}

// EventBus --
func (*MockHost) EventBus() event.Bus {
	return nil
}
