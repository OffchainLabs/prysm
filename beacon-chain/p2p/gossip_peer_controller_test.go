package p2p

import (
	"context"
	"crypto/rand"
	"net"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/gossipcrawler"
	"github.com/OffchainLabs/prysm/v7/crypto/ecdsa"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestGossipPeerDialer_Start(t *testing.T) {
	tests := []struct {
		name             string
		newCrawler       func(t *testing.T) *mockCrawler
		provider         gossipcrawler.SubnetTopicsProvider
		expectedConnects int
		expectStartErr   bool
	}{
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
			provider: func() map[string]int {
				return map[string]int{"topic/a": 2, "topic/b": 2}
			},
			expectedConnects: 2,
		},
		{
			name: "uses per-topic min peer counts",
			newCrawler: func(t *testing.T) *mockCrawler {
				nodes := make([]*enode.Node, 5)
				for i := range nodes {
					nodes[i] = newTestNode(t, "127.0.0.1", uint16(30110+i))
				}
				return &mockCrawler{
					consume: true,
					peers: map[string][]*enode.Node{
						// topic/mesh has 3 available peers, minPeers=2 -> should dial 2
						"topic/mesh": {nodes[0], nodes[1], nodes[2]},
						// topic/fanout has 3 available peers, minPeers=1 -> should dial 1
						"topic/fanout": {nodes[3], nodes[4]},
					},
				}
			},
			provider: func() map[string]int {
				return map[string]int{
					"topic/mesh":   2,
					"topic/fanout": 1,
				}
			},
			// Total: 2 from mesh + 1 from fanout = 3 peers dialed
			expectedConnects: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := &mockDialer{}
			listPeers := func(topic string) []peer.ID { return nil }

			dialer := NewGossipPeerDialer(t.Context(), tt.newCrawler(t), listPeers, md.DialPeers)

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

func TestGossipPeerDialer_DialPeersForTopicBlocking(t *testing.T) {
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
			dialer := NewGossipPeerDialer(t.Context(), crawler, listPeers, dialPeers)
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
		})
	}
}

func TestGossipPeerDialer_peersForTopic(t *testing.T) {
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
			dialer := NewGossipPeerDialer(t.Context(), crawler, listPeers, func(ctx context.Context,
				maxConcurrentDials int, nodes []*enode.Node) uint {
				return 0
			})

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

func TestGossipPeerDialer_selectPeersForTopics(t *testing.T) {
	tests := []struct {
		name           string
		connectedPeers map[string]int // topic -> connected peer count
		topicsProvider func() map[string]int
		buildPeers     func(t *testing.T) (map[string][]*enode.Node, []*enode.Node)
	}{
		{
			name:           "prioritizes multi-topic peer over single-topic peers",
			connectedPeers: map[string]int{},
			topicsProvider: func() map[string]int {
				return map[string]int{
					"topic/a": 1,
					"topic/b": 1,
					"topic/c": 1,
				}
			},
			buildPeers: func(t *testing.T) (map[string][]*enode.Node, []*enode.Node) {
				// Peer X serves all 3 topics
				nodeX := newTestNode(t, "127.0.0.1", 30401)
				// Peer Y serves only topic/a
				nodeY := newTestNode(t, "127.0.0.1", 30402)
				// Peer Z serves only topic/b
				nodeZ := newTestNode(t, "127.0.0.1", 30403)

				crawlerPeers := map[string][]*enode.Node{
					"topic/a": {nodeX, nodeY},
					"topic/b": {nodeX, nodeZ},
					"topic/c": {nodeX},
				}
				// Only nodeX should be dialed (satisfies all 3 topics)
				return crawlerPeers, []*enode.Node{nodeX}
			},
		},
		{
			name:           "cross-topic decrement works correctly",
			connectedPeers: map[string]int{},
			topicsProvider: func() map[string]int {
				return map[string]int{
					"topic/a": 2, // Need 2 peers
					"topic/b": 1, // Need 1 peer
				}
			},
			buildPeers: func(t *testing.T) (map[string][]*enode.Node, []*enode.Node) {
				// Peer X serves both topics
				nodeX := newTestNode(t, "127.0.0.1", 30411)
				// Peer Y serves only topic/a
				nodeY := newTestNode(t, "127.0.0.1", 30412)

				crawlerPeers := map[string][]*enode.Node{
					"topic/a": {nodeX, nodeY},
					"topic/b": {nodeX},
				}
				// nodeX covers topic/b fully, and 1 of 2 for topic/a
				// nodeY covers remaining 1 for topic/a
				return crawlerPeers, []*enode.Node{nodeX, nodeY}
			},
		},
		{
			name:           "no redundant dials when one peer satisfies all",
			connectedPeers: map[string]int{},
			topicsProvider: func() map[string]int {
				return map[string]int{
					"topic/a": 1,
					"topic/b": 1,
					"topic/c": 1,
				}
			},
			buildPeers: func(t *testing.T) (map[string][]*enode.Node, []*enode.Node) {
				nodeX := newTestNode(t, "127.0.0.1", 30421)
				crawlerPeers := map[string][]*enode.Node{
					"topic/a": {nodeX},
					"topic/b": {nodeX},
					"topic/c": {nodeX},
				}
				// Only 1 dial needed for all 3 topics
				return crawlerPeers, []*enode.Node{nodeX}
			},
		},
		{
			name: "skips topics with enough peers already",
			connectedPeers: map[string]int{
				"topic/a": 2, // Already has 2
			},
			topicsProvider: func() map[string]int {
				return map[string]int{
					"topic/a": 2, // min 2, already have 2
					"topic/b": 1, // min 1, have 0
				}
			},
			buildPeers: func(t *testing.T) (map[string][]*enode.Node, []*enode.Node) {
				nodeX := newTestNode(t, "127.0.0.1", 30431)
				nodeY := newTestNode(t, "127.0.0.1", 30432)
				crawlerPeers := map[string][]*enode.Node{
					"topic/a": {nodeX},
					"topic/b": {nodeY},
				}
				// Only nodeY should be dialed (topic/a already satisfied)
				return crawlerPeers, []*enode.Node{nodeY}
			},
		},
		{
			name:           "returns nil when all topics satisfied",
			connectedPeers: map[string]int{"topic/a": 2, "topic/b": 1},
			topicsProvider: func() map[string]int {
				return map[string]int{
					"topic/a": 2,
					"topic/b": 1,
				}
			},
			buildPeers: func(t *testing.T) (map[string][]*enode.Node, []*enode.Node) {
				nodeX := newTestNode(t, "127.0.0.1", 30441)
				crawlerPeers := map[string][]*enode.Node{
					"topic/a": {nodeX},
					"topic/b": {nodeX},
				}
				// No dials needed
				return crawlerPeers, nil
			},
		},
		{
			name:           "handles empty crawler response",
			connectedPeers: map[string]int{},
			topicsProvider: func() map[string]int {
				return map[string]int{"topic/a": 1}
			},
			buildPeers: func(t *testing.T) (map[string][]*enode.Node, []*enode.Node) {
				return map[string][]*enode.Node{}, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listPeers := func(topic string) []peer.ID {
				count := tt.connectedPeers[topic]
				peers := make([]peer.ID, count)
				for i := range count {
					peers[i] = peer.ID(topic + string(rune(i)))
				}
				return peers
			}

			crawlerPeers, expected := tt.buildPeers(t)
			crawler := &mockCrawler{
				peers:   crawlerPeers,
				consume: false,
			}

			dialer := NewGossipPeerDialer(t.Context(), crawler, listPeers, func(ctx context.Context,
				maxConcurrentDials int, nodes []*enode.Node) uint {
				return 0
			})
			dialer.topicsProvider = tt.topicsProvider

			got := dialer.selectPeersForTopics()

			if expected == nil {
				require.Nil(t, got)
				return
			}

			require.Equal(t, len(expected), len(got), "expected %d peers, got %d", len(expected), len(got))

			// Verify all expected nodes are present (order may vary for equal topic counts)
			expectedIDs := make(map[enode.ID]struct{})
			for _, n := range expected {
				expectedIDs[n.ID()] = struct{}{}
			}
			for _, n := range got {
				_, ok := expectedIDs[n.ID()]
				require.True(t, ok, "unexpected peer %s in result", n.ID())
			}
		})
	}
}

type mockCrawler struct {
	mu      sync.Mutex
	peers   map[string][]*enode.Node
	consume bool
}

func (m *mockCrawler) Start(gossipcrawler.TopicExtractor) error {
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

func (m *mockDialer) dialedNodes() []*enode.Node {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.dials)
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
