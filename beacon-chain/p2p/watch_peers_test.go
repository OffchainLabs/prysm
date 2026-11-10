package p2p

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers/scorers"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestEnsurePeerConnections(t *testing.T) {
	newPeers := func(t *testing.T, trusted *mockp2p.TestP2P) *peers.Status {
		status := peers.NewStatus(t.Context(), &peers.StatusConfig{PeerLimit: 30, ScorerParams: &scorers.Config{}})
		status.Add(nil, trusted.PeerID(), trusted.BHost.Addrs()[0], network.DirUnknown)
		status.SetTrustedPeers([]peer.ID{trusted.PeerID()})
		return status
	}

	t.Run("reconnects to trusted peer", func(t *testing.T) {
		local, trusted := mockp2p.NewTestP2P(t), mockp2p.NewTestP2P(t)

		ensurePeerConnections(t.Context(), local.BHost, newPeers(t, trusted), nil)
		require.NotEqual(t, 0, len(local.BHost.Network().ConnsToPeer(trusted.PeerID())))
	})

	t.Run("skips trusted peer not to reconnect to", func(t *testing.T) {
		local, trusted := mockp2p.NewTestP2P(t), mockp2p.NewTestP2P(t)

		notToReconnect := map[peer.ID]bool{trusted.PeerID(): true}
		ensurePeerConnections(t.Context(), local.BHost, newPeers(t, trusted), notToReconnect)
		require.Equal(t, 0, len(local.BHost.Network().ConnsToPeer(trusted.PeerID())))
	})
}
