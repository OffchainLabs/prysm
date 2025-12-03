package p2p

import (
	"context"
	"crypto/rand"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/gossipsubcrawler"
	"github.com/OffchainLabs/prysm/v7/crypto/ecdsa"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestGossipsubPeerDialer_Start(t *testing.T) {
	tests := []struct {
		name             string
		newCrawler       func(t *testing.T) *mockCrawler
		provider         gossipsubcrawler.SubnetTopicsProvider
		expectedConnects int
		expectStartErr   bool
	}{
		{
			name:           "nil provider errors",
			newCrawler:     func(t *testing.T) *mockCrawler { return &mockCrawler{} },
			provider:       nil,
			expectStartErr: true,
		},
		{
			name: "dials unique peers across topics",
			newCrawler: func(t *testing.T) *mockCrawler {
				nodeA := newTestNode(t, "127.0.0.1", 30101)
				nodeB := newTestNode(t, "127.0.0.1", 30102)
				return &mockCrawler{
					consume: true,
					peers: map[string][]*enode.Node{
						"topic/a": {nodeA, nodeB},
						"topic/b": {nodeA},
					},
				}
			},
			provider: func() []string {
				return []string{"topic/a", "topic/b"}
			},
			expectedConnects: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := &mockDialer{}
			listPeers := func(topic string) []peer.ID { return nil }

			dialer := NewGossipsubPeerDialer(tt.newCrawler(t), listPeers, md.DialPeers)
			defer dialer.Stop()

			err := dialer.Start(tt.provider)
			if tt.expectStartErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				return md.dialCount() >= tt.expectedConnects
			}, 2*time.Second, 20*time.Millisecond)

			require.Equal(t, tt.expectedConnects, md.dialCount())
		})
	}
}

func TestGossipsubPeerDialer_DialPeersForTopicBlocking(t *testing.T) {
	tests := []struct {
		name             string
		connectedPeers   int
		newCrawler       func(t *testing.T) *mockCrawler
		targetPeers      int
		ctx              func() (context.Context, context.CancelFunc)
		expectedConnects int
		expectErr        bool
	}{
		{
			name:           "returns immediately when enough peers",
			connectedPeers: 1,
			newCrawler: func(t *testing.T) *mockCrawler {
				return &mockCrawler{}
			},
			targetPeers:      1,
			ctx:              func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			expectedConnects: 0,
			expectErr:        false,
		},
		{
			name:           "dials when peers are missing",
			connectedPeers: 0,
			newCrawler: func(t *testing.T) *mockCrawler {
				nodeA := newTestNode(t, "127.0.0.1", 30201)
				nodeB := newTestNode(t, "127.0.0.1", 30202)
				return &mockCrawler{
					peers: map[string][]*enode.Node{
						"topic/a": {nodeA, nodeB},
					},
				}
			},
			targetPeers: 2,
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 1*time.Second)
			},
			expectedConnects: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := &mockDialer{}
			var mu sync.Mutex
			connected := make([]peer.ID, 0)
			for i := 0; i < tt.connectedPeers; i++ {
				connected = append(connected, peer.ID(string(rune(i))))
			}

			listPeers := func(topic string) []peer.ID {
				mu.Lock()
				defer mu.Unlock()
				return connected
			}

			dialPeers := func(ctx context.Context, max int, nodes []*enode.Node) uint {
				cnt := md.DialPeers(ctx, max, nodes)
				mu.Lock()
				defer mu.Unlock()
				for range nodes {
					// Just add a dummy peer ID to simulate connection success
					connected = append(connected, peer.ID("dummy"))
				}
				return cnt
			}

			crawler := tt.newCrawler(t)
			dialer := NewGossipsubPeerDialer(crawler, listPeers, dialPeers)
			topic := "topic/a"

			ctx, cancel := tt.ctx()
			defer cancel()

			err := dialer.DialPeersForTopicBlocking(ctx, topic, tt.targetPeers)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.expectedConnects, md.dialCount())
			dialer.Stop()
		})
	}
}

func TestGossipsubPeerDialer_peersForTopic(t *testing.T) {
	tests := []struct {
		name        string
		connected   int
		targetCount int
		buildPeers  func(t *testing.T) ([]*enode.Node, []*enode.Node)
	}{
		{
			name:        "returns nil when enough peers already connected",
			connected:   1,
			targetCount: 1,
			buildPeers: func(t *testing.T) ([]*enode.Node, []*enode.Node) {
				return []*enode.Node{newTestNode(t, "127.0.0.1", 30301)}, nil
			},
		},
		{
			name:        "returns crawler peers when none connected",
			connected:   0,
			targetCount: 2,
			buildPeers: func(t *testing.T) ([]*enode.Node, []*enode.Node) {
				nodeA := newTestNode(t, "127.0.0.1", 30311)
				nodeB := newTestNode(t, "127.0.0.1", 30312)
				return []*enode.Node{nodeA, nodeB}, []*enode.Node{nodeA, nodeB}
			},
		},
		{
			name:        "truncates peers when more than needed",
			connected:   0,
			targetCount: 1,
			buildPeers: func(t *testing.T) ([]*enode.Node, []*enode.Node) {
				nodeA := newTestNode(t, "127.0.0.1", 30321)
				nodeB := newTestNode(t, "127.0.0.1", 30322)
				nodeC := newTestNode(t, "127.0.0.1", 30323)
				return []*enode.Node{nodeA, nodeB, nodeC}, []*enode.Node{nodeA}
			},
		},
		{
			name:        "only returns missing peers",
			connected:   1,
			targetCount: 3,
			buildPeers: func(t *testing.T) ([]*enode.Node, []*enode.Node) {
				nodeA := newTestNode(t, "127.0.0.1", 30331)
				nodeB := newTestNode(t, "127.0.0.1", 30332)
				nodeC := newTestNode(t, "127.0.0.1", 30333)
				return []*enode.Node{nodeA, nodeB, nodeC}, []*enode.Node{nodeA, nodeB}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listPeers := func(topic string) []peer.ID {
				peers := make([]peer.ID, tt.connected)
				for i := 0; i < tt.connected; i++ {
					peers[i] = peer.ID(string(rune(i))) // Fake peer ID
				}
				return peers
			}

			crawlerPeers, expected := tt.buildPeers(t)
			crawler := &mockCrawler{
				peers:   map[string][]*enode.Node{"topic/test": crawlerPeers},
				consume: false,
			}
			dialer := NewGossipsubPeerDialer(crawler, listPeers, func(ctx context.Context, maxConcurrentDials int, nodes []*enode.Node) uint { return 0 })

			got := dialer.peersForTopic("topic/test", tt.targetCount)
			if expected == nil {
				require.Nil(t, got)
				return
			}

			require.Equal(t, len(expected), len(got))

			for i := range expected {
				require.Equal(t, expected[i], got[i])
			}
		})
	}
}

type mockCrawler struct {
	mu      sync.Mutex
	peers   map[string][]*enode.Node
	consume bool
}

func (m *mockCrawler) Start(gossipsubcrawler.TopicExtractor) error {
	return nil
}

func (m *mockCrawler) Stop()                      {}
func (m *mockCrawler) RemovePeerByPeerId(peer.ID) {}
func (m *mockCrawler) RemoveTopic(string)         {}
func (m *mockCrawler) PeersForTopic(topic string) []*enode.Node {
	m.mu.Lock()
	defer m.mu.Unlock()

	nodes := m.peers[topic]
	if len(nodes) == 0 {
		return nil
	}

	copied := slices.Clone(nodes)
	if m.consume {
		m.peers[topic] = nil
	}
	return copied
}

type mockDialer struct {
	mu    sync.Mutex
	dials []*enode.Node
}

func (m *mockDialer) DialPeers(ctx context.Context, maxConcurrentDials int, nodes []*enode.Node) uint {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dials = append(m.dials, nodes...)
	return uint(len(nodes))
}

func (m *mockDialer) dialCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.dials)
}

func newTestNode(t *testing.T, ip string, tcpPort uint16) *enode.Node {
	priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)

	return newTestNodeWithPriv(t, priv, ip, tcpPort)
}

func newTestNodeWithPriv(t *testing.T, priv crypto.PrivKey, ip string, tcpPort uint16) *enode.Node {
	t.Helper()

	db, err := enode.OpenDB("")
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})

	convertedKey, err := ecdsa.ConvertFromInterfacePrivKey(priv)
	require.NoError(t, err)

	localNode := enode.NewLocalNode(db, convertedKey)
	localNode.SetStaticIP(net.ParseIP(ip))
	localNode.Set(enr.TCP(tcpPort))
	localNode.Set(enr.UDP(tcpPort))

	return localNode.Node()
}
