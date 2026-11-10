package p2p

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers/scorers"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/patrickmn/go-cache"
)

func TestAddConnectionHandler_RecentlyDisconnectedOutboundPeer(t *testing.T) {
	const closeDelay = 100 * time.Millisecond
	previousDelay := untrackedConnCloseDelay
	untrackedConnCloseDelay = closeDelay
	t.Cleanup(func() { untrackedConnCloseDelay = previousDelay })

	tests := []struct {
		name               string
		recentlyDisconnect bool
		adopted            bool
	}{
		{name: "untracked connection is closed", recentlyDisconnect: true},
		{name: "connection adopted by a handshake is kept", recentlyDisconnect: true, adopted: true},
		{name: "other peer is handshaked", recentlyDisconnect: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h1, _, _ := createHost(t, 0)
			h2, _, _ := createHost(t, 0)
			t.Cleanup(func() {
				require.NoError(t, h1.Close())
				require.NoError(t, h2.Close())
			})

			s := &Service{
				host:                  h1,
				peers:                 peers.NewStatus(t.Context(), &peers.StatusConfig{ScorerParams: &scorers.Config{}}),
				peerDisconnectionTime: cache.New(1*time.Second, 1*time.Minute),
			}

			var statusRequests atomic.Int32
			reqFunc := func(_ context.Context, _ peer.ID) error {
				statusRequests.Add(1)
				return nil
			}
			noop := func(_ context.Context, _ peer.ID) error { return nil }
			s.AddConnectionHandler(reqFunc, noop)
			s.AddDisconnectionHandler(noop)

			if tt.recentlyDisconnect {
				require.NoError(t, s.peerDisconnectionTime.Add(h2.ID().String(), time.Now(), cache.DefaultExpiration))
			}

			require.NoError(t, h1.Connect(t.Context(), peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()}))

			if !tt.recentlyDisconnect {
				require.Eventually(t, func() bool {
					state, err := s.peers.ConnectionState(h2.ID())
					return err == nil && state == peers.Connected
				}, 5*time.Second, 10*time.Millisecond, "peer was not handshaked")
				require.Equal(t, int32(1), statusRequests.Load())
				require.Equal(t, network.Connected, h1.Network().Connectedness(h2.ID()))
				return
			}

			// The skipped connection stays open during the handshake window.
			require.Equal(t, network.Connected, h1.Network().Connectedness(h2.ID()))

			if tt.adopted {
				s.peers.SetConnectionState(h2.ID(), peers.Connected)
				time.Sleep(3 * closeDelay)
				require.Equal(t, network.Connected, h1.Network().Connectedness(h2.ID()), "adopted connection was closed")
				require.Equal(t, int32(0), statusRequests.Load())
				return
			}

			require.Eventually(t, func() bool {
				return h1.Network().Connectedness(h2.ID()) != network.Connected
			}, 5*time.Second, 10*time.Millisecond, "untracked connection was not closed")
			require.Equal(t, int32(0), statusRequests.Load())
		})
	}
}
