package p2p

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/gossipcrawler"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	require2 "github.com/stretchr/testify/require"
)

// Helpers for crawledPeers tests
func newTestCrawledPeers() *crawledPeers {
	return &crawledPeers{
		peerNodeByEnode: make(map[enode.ID]*peerNode),
		peerNodeByPid:   make(map[peer.ID]*peerNode),
		peersByTopic:    make(map[string]map[*peerNode]struct{}),
	}
}

func addPeerWithTopics(t *testing.T, cp *crawledPeers, node *enode.Node, topics []string, pinged bool) *peerNode {
	t.Helper()
	pid, err := enodeToPeerID(node)
	require.NoError(t, err)
	p := &peerNode{
		isPinged: pinged,
		node:     node,
		peerID:   pid,
		topics:   make(map[string]struct{}),
	}
	cp.mu.Lock()
	cp.peerNodeByEnode[p.node.ID()] = p
	cp.peerNodeByPid[p.peerID] = p
	cp.updateTopicsUnlocked(p, topics)
	cp.mu.Unlock()
	return p
}

func TestUpdateStatusToPinged(t *testing.T) {
	localNode := createTestNodeRandom(t)
	node1 := localNode.Node()
	localNode2 := createTestNodeRandom(t)
	node2 := localNode2.Node()

	cases := []struct {
		name         string
		prep         func(*crawledPeers)
		target       *enode.Node
		expectPinged map[enode.ID]bool
	}{
		{
			name: "sets pinged for existing peer",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"a"}, false)
			},
			target: node1,
			expectPinged: map[enode.ID]bool{
				node1.ID(): true,
			},
		},
		{
			name: "idempotent when already pinged",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"a"}, true)
			},
			target: node1,
			expectPinged: map[enode.ID]bool{
				node1.ID(): true,
			},
		},
		{
			name: "no change when peer missing",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"a"}, false)
			},
			target: node2,
			expectPinged: map[enode.ID]bool{
				node1.ID(): false,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := newTestCrawledPeers()
			tc.prep(cp)
			cp.updateStatusToPinged(tc.target.ID())
			cp.mu.RLock()
			defer cp.mu.RUnlock()
			for id, exp := range tc.expectPinged {
				if p := cp.peerNodeByEnode[id]; p != nil {
					require.Equal(t, exp, p.isPinged)
				}
			}
		})
	}
}

func TestRemoveTopic(t *testing.T) {
	localNode := createTestNodeRandom(t)
	node1 := localNode.Node()
	localNode2 := createTestNodeRandom(t)
	node2 := localNode2.Node()

	topic1 := "t1"
	topic2 := "t2"

	cases := []struct {
		name  string
		prep  func(*crawledPeers)
		topic string
		check func(*testing.T, *crawledPeers)
	}{
		{
			name: "removes topic from all peers and index",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"t1", "t2"}, true)
				addPeerWithTopics(t, cp, node2, []string{"t1"}, true)
			},
			topic: topic1,
			check: func(t *testing.T, cp *crawledPeers) {
				_, ok := cp.peersByTopic[topic1]
				require.False(t, ok)
				for _, p := range cp.peerNodeByPid {
					_, has := p.topics[topic1]
					require.False(t, has)
				}
				// Ensure other topics remain
				_, ok = cp.peersByTopic[topic2]
				require.True(t, ok)
			},
		},
		{
			name: "no-op when topic missing",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"t2"}, true)
			},
			topic: topic1,
			check: func(t *testing.T, cp *crawledPeers) {
				_, ok := cp.peersByTopic[topic2]
				require.True(t, ok)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := newTestCrawledPeers()
			tc.prep(cp)
			cp.removeTopic(tc.topic)
			tc.check(t, cp)
		})
	}
}

func TestRemovePeer(t *testing.T) {
	localNode := createTestNodeRandom(t)
	node1 := localNode.Node()
	localNode2 := createTestNodeRandom(t)
	node2 := localNode2.Node()

	cases := []struct {
		name       string
		prep       func(*crawledPeers)
		target     enode.ID
		wantTopics int
	}{
		{
			name: "removes existing peer and prunes empty topic",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"t1"}, true)
			},
			target:     node1.ID(),
			wantTopics: 0,
		},
		{
			name: "removes only targeted peer; keeps topic for other",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"t1"}, true)
				addPeerWithTopics(t, cp, node2, []string{"t1"}, true)
			},
			target:     node1.ID(),
			wantTopics: 1, // byTopic should still have t1 with one peer
		},
		{
			name: "no-op when peer missing",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"t1"}, true)
			},
			target:     node2.ID(),
			wantTopics: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := newTestCrawledPeers()
			tc.prep(cp)
			cp.removePeerByNodeId(tc.target)
			cp.mu.RLock()
			defer cp.mu.RUnlock()
			require.Len(t, cp.peersByTopic, tc.wantTopics)
		})
	}
}

func TestRemovePeerId(t *testing.T) {
	localNode := createTestNodeRandom(t)
	node1 := localNode.Node()
	localNode2 := createTestNodeRandom(t)
	node2 := localNode2.Node()

	pid1, err := enodeToPeerID(node1)
	require.NoError(t, err)
	pid2, err := enodeToPeerID(node2)
	require.NoError(t, err)

	cases := []struct {
		name       string
		prep       func(*crawledPeers)
		target     peer.ID
		wantTopics int
		wantPeers  int
	}{
		{
			name: "removes existing peer by id and prunes topic",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"t1"}, true)
			},
			target:     pid1,
			wantTopics: 0,
			wantPeers:  0,
		},
		{
			name: "removes only targeted peer id; keeps topic for other",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"t1"}, true)
				addPeerWithTopics(t, cp, node2, []string{"t1"}, true)
			},
			target:     pid1,
			wantTopics: 1,
			wantPeers:  1,
		},
		{
			name: "no-op when peer id missing",
			prep: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, node1, []string{"t1"}, true)
			},
			target:     pid2,
			wantTopics: 1,
			wantPeers:  1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := newTestCrawledPeers()
			tc.prep(cp)
			cp.removePeerByPeerId(tc.target)
			cp.mu.RLock()
			defer cp.mu.RUnlock()
			require.Len(t, cp.peersByTopic, tc.wantTopics)
			require.Len(t, cp.peerNodeByPid, tc.wantPeers)
		})
	}
}

func TestUpdateCrawledIfNewer(t *testing.T) {
	newCrawler := func() (*crawledPeers, *GossipPeerCrawler, func()) {
		ctx, cancel := context.WithCancel(context.Background())
		g := &GossipPeerCrawler{
			ctx:    ctx,
			pingCh: make(chan enode.Node, 8),
		}
		cp := newTestCrawledPeers()
		return cp, g, cancel
	}

	// Helper: local node that will cause enodeToPeerID to fail (no TCP/UDP multiaddrs)
	newNodeNoPorts := func(t *testing.T) *enode.Node {
		_, privKey := createAddrAndPrivKey(t)
		db, err := enode.OpenDB("")
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		ln := enode.NewLocalNode(db, privKey)
		// Do not set TCP/UDP; keep only IP
		ln.SetStaticIP(net.ParseIP("127.0.0.1"))
		return ln.Node()
	}

	// Ensure both A nodes have the same enode.ID but differing seq
	ln := createTestNodeRandom(t)
	nodeA1 := ln.Node()
	setNodeSeq(ln, nodeA1.Seq()+1)
	nodeA2 := ln.Node()

	tests := []struct {
		name               string
		arrange            func(*crawledPeers)
		invokeNode         *enode.Node
		invokeTopics       []string
		expectedShouldPing bool
		expectErr          bool
		assert             func(*testing.T, *crawledPeers, <-chan enode.Node)
	}{
		{
			name:               "new peer with topics adds peer and pings",
			arrange:            func(cp *crawledPeers) {},
			invokeNode:         nodeA1,
			invokeTopics:       []string{"a"},
			expectedShouldPing: true,
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Len(t, cp.peerNodeByEnode, 1)
				require.Len(t, cp.peerNodeByPid, 1)
				require.Contains(t, cp.peersByTopic, "a")
				cp.mu.RUnlock()

			},
		},
		{
			name:         "new peer with empty topics is removed",
			arrange:      func(cp *crawledPeers) {},
			invokeNode:   nodeA1,
			invokeTopics: nil,
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Empty(t, cp.peerNodeByEnode)
				require.Empty(t, cp.peerNodeByPid)
				require.Empty(t, cp.peersByTopic)
				cp.mu.RUnlock()
			},
		},
		{
			name: "existing peer lower seq is ignored",
			arrange: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, nodeA2, []string{"x"}, false) // higher seq exists
			},
			invokeNode:   nodeA1, // lower seq
			invokeTopics: []string{"a", "b"},
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Contains(t, cp.peersByTopic, "x")
				require.NotContains(t, cp.peersByTopic, "a")
				cp.mu.RUnlock()
			},
		},
		{
			name: "existing peer equal seq is ignored",
			arrange: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, nodeA1, []string{"x"}, false)
			},
			invokeNode:   nodeA1,
			invokeTopics: []string{"a"},
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Contains(t, cp.peersByTopic, "x")
				require.NotContains(t, cp.peersByTopic, "a")
				cp.mu.RUnlock()
			},
		},
		{
			name: "existing peer higher seq updates topics and pings",
			arrange: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, nodeA1, []string{"x"}, false)
			},
			invokeNode:         nodeA2,
			invokeTopics:       []string{"a"},
			expectedShouldPing: true,
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.NotContains(t, cp.peersByTopic, "x")
				require.Contains(t, cp.peersByTopic, "a")
				cp.mu.RUnlock()
			},
		},
		{
			name: "existing peer higher seq but empty topics removes peer",
			arrange: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, nodeA1, []string{"x"}, false)
			},
			invokeNode:   nodeA2,
			invokeTopics: nil,
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Empty(t, cp.peerNodeByEnode)
				require.Empty(t, cp.peerNodeByPid)
				cp.mu.RUnlock()
			},
		},
		{
			name: "corrupted existing entry with nil node is ignored",
			arrange: func(cp *crawledPeers) {
				pid, _ := enodeToPeerID(nodeA1)
				cp.mu.Lock()
				pn := &peerNode{node: nil, peerID: pid, topics: map[string]struct{}{"x": {}}}
				cp.peerNodeByEnode[nodeA1.ID()] = pn
				cp.peerNodeByPid[pid] = pn
				cp.peersByTopic["x"] = map[*peerNode]struct{}{pn: {}}
				cp.mu.Unlock()
			},
			expectErr:    true,
			invokeNode:   nodeA2,
			invokeTopics: []string{"a"},
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Contains(t, cp.peersByTopic, "x")
				cp.mu.RUnlock()
			},
		},
		{
			name:         "new peer with no ports causes enodeToPeerID error; no add",
			arrange:      func(cp *crawledPeers) {},
			invokeNode:   newNodeNoPorts(t),
			invokeTopics: []string{"a"},
			expectErr:    true,
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Empty(t, cp.peerNodeByEnode)
				require.Empty(t, cp.peerNodeByPid)
				require.Empty(t, cp.peersByTopic)
				cp.mu.RUnlock()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cp, g, cancel := newCrawler()
			defer cancel()
			tc.arrange(cp)
			shouldPing, err := cp.updatePeer(tc.invokeNode, tc.invokeTopics)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, shouldPing, tc.expectedShouldPing)
			tc.assert(t, cp, g.pingCh)
		})
	}
}

func TestPeersForTopic(t *testing.T) {
	t.Parallel()

	newCrawler := func(filter gossipcrawler.PeerFilterFunc) (*GossipPeerCrawler, *crawledPeers) {
		g := &GossipPeerCrawler{
			peerFilter:   filter,
			scorer:       func(peer.ID) float64 { return 0 },
			crawledPeers: newTestCrawledPeers(),
		}
		return g, g.crawledPeers
	}

	// Prepare nodes
	ln1 := createTestNodeRandom(t)
	ln2 := createTestNodeRandom(t)
	ln3 := createTestNodeRandom(t)
	n1, n2, n3 := ln1.Node(), ln2.Node(), ln3.Node()
	topic := "top"

	cases := []struct {
		name    string
		filter  gossipcrawler.PeerFilterFunc
		setup   func(t *testing.T, g *GossipPeerCrawler, cp *crawledPeers)
		wantIDs []enode.ID
	}{
		{
			name:    "no peers for topic returns empty",
			filter:  func(*enode.Node) bool { return true },
			setup:   func(t *testing.T, g *GossipPeerCrawler, cp *crawledPeers) {},
			wantIDs: nil,
		},
		{
			name:   "excludes unpinged peers",
			filter: func(*enode.Node) bool { return true },
			setup: func(t *testing.T, g *GossipPeerCrawler, cp *crawledPeers) {
				// Add one pinged and one not pinged on same topic
				addPeerWithTopics(t, cp, n1, []string{string(topic)}, true)
				addPeerWithTopics(t, cp, n2, []string{string(topic)}, false)
			},
			wantIDs: []enode.ID{n1.ID()},
		},
		{
			name:   "applies peer filter to exclude",
			filter: func(n *enode.Node) bool { return n.ID() != n2.ID() },
			setup: func(t *testing.T, g *GossipPeerCrawler, cp *crawledPeers) {
				addPeerWithTopics(t, cp, n1, []string{string(topic)}, true)
				addPeerWithTopics(t, cp, n2, []string{string(topic)}, true)
			},
			wantIDs: []enode.ID{n1.ID()},
		},
		{
			name:   "ignores peerNode with nil node",
			filter: func(*enode.Node) bool { return true },
			setup: func(t *testing.T, g *GossipPeerCrawler, cp *crawledPeers) {
				addPeerWithTopics(t, cp, n1, []string{string(topic)}, true)
				// Add n2 then set its node to nil to simulate corrupted entry
				p2 := addPeerWithTopics(t, cp, n2, []string{string(topic)}, true)
				cp.mu.Lock()
				p2.node = nil
				cp.mu.Unlock()
			},
			wantIDs: []enode.ID{n1.ID()},
		},
		{
			name:   "sorted by score descending",
			filter: func(*enode.Node) bool { return true },
			setup: func(t *testing.T, g *GossipPeerCrawler, cp *crawledPeers) {
				// Add three pinged peers
				p1 := addPeerWithTopics(t, cp, n1, []string{string(topic)}, true)
				p2 := addPeerWithTopics(t, cp, n2, []string{string(topic)}, true)
				p3 := addPeerWithTopics(t, cp, n3, []string{string(topic)}, true)
				// Provide a deterministic scoring function
				scores := map[peer.ID]float64{
					p1.peerID: 3.0,
					p2.peerID: 2.0,
					p3.peerID: 1.0,
				}
				g.scorer = func(id peer.ID) float64 { return scores[id] }
			},
			wantIDs: []enode.ID{n1.ID(), n2.ID(), n3.ID()},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, cp := newCrawler(tc.filter)
			tc.setup(t, g, cp)
			got := g.PeersForTopic(topic)
			var gotIDs []enode.ID
			for _, n := range got {
				gotIDs = append(gotIDs, n.ID())
			}
			if tc.wantIDs == nil {
				require.Empty(t, gotIDs)
				return
			}
			require.Equal(t, tc.wantIDs, gotIDs)
		})
	}
}

func TestCrawler_AddsAndPingsPeer(t *testing.T) {
	// Create a test node with valid ENR entries (IP/TCP/UDP)
	localNode := createTestNodeRandom(t)
	node := localNode.Node()

	// Prepare a mock iterator returning our single node
	iterator := p2ptest.NewMockIterator([]*enode.Node{node})
	// Prepare a mock listener with successful Ping
	mockListener := p2ptest.NewMockListener(localNode, iterator)
	mockListener.PingFunc = func(*enode.Node) error { return nil }

	// Inject a permissive peer filter
	filter := gossipcrawler.PeerFilterFunc(func(n *enode.Node) bool { return true })

	// Create crawler with small intervals
	scorer := func(peer.ID) float64 { return 0 }
	g, err := NewGossipPeerCrawler(t.Context(), &Service{}, mockListener, 2*time.Second, 10*time.Millisecond, 4, filter, scorer)
	require.NoError(t, err)

	// Assign a simple topic extractor
	topic := "test/topic"
	topicExtractor := func(ctx context.Context, n *enode.Node) ([]string, error) {
		return []string{topic}, nil
	}

	// Run ping loop in background and perform a single crawl
	require.NoError(t, g.Start(topicExtractor))

	// Verify that the peer has been indexed under the topic and marked as pinged
	require2.Eventually(t, func() bool {
		g.crawledPeers.mu.RLock()
		defer g.crawledPeers.mu.RUnlock()

		peers := g.crawledPeers.peersByTopic[topic]
		if len(peers) == 0 {
			return false
		}
		// Fetch the single peerNode and check status
		for pn := range peers {
			if pn == nil {
				return false
			}
			return pn.isPinged
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
}

func TestCrawler_SkipsPeer_WhenFilterRejects(t *testing.T) {
	t.Parallel()

	localNode := createTestNodeRandom(t)
	node := localNode.Node()
	iterator := p2ptest.NewMockIterator([]*enode.Node{node})
	mockListener := p2ptest.NewMockListener(localNode, iterator)
	mockListener.PingFunc = func(*enode.Node) error { return nil }

	// Reject all peers via injected filter
	filter := gossipcrawler.PeerFilterFunc(func(n *enode.Node) bool { return false })

	scorer := func(peer.ID) float64 { return 0 }
	g, err := NewGossipPeerCrawler(t.Context(), &Service{}, mockListener, 2*time.Second, 10*time.Millisecond, 2, filter, scorer)
	if err != nil {
		t.Fatalf("NewGossipPeerCrawler error: %v", err)
	}

	topic := "test/topic"
	g.topicExtractor = func(ctx context.Context, n *enode.Node) ([]string, error) { return []string{topic}, nil }

	g.crawl()

	// Verify no peers are indexed, because filter rejected the node
	g.crawledPeers.mu.RLock()
	defer g.crawledPeers.mu.RUnlock()
	if len(g.crawledPeers.peerNodeByEnode) != 0 || len(g.crawledPeers.peerNodeByPid) != 0 || len(g.crawledPeers.peersByTopic) != 0 {
		t.Fatalf("expected no peers indexed, got byEnode=%d byPeerId=%d byTopic=%d",
			len(g.crawledPeers.peerNodeByEnode), len(g.crawledPeers.peerNodeByPid), len(g.crawledPeers.peersByTopic))
	}
}

func TestCrawler_RemoveTopic_RemovesTopicFromIndexes(t *testing.T) {
	t.Parallel()

	localNode := createTestNodeRandom(t)
	node := localNode.Node()
	iterator := p2ptest.NewMockIterator([]*enode.Node{node})
	mockListener := p2ptest.NewMockListener(localNode, iterator)
	mockListener.PingFunc = func(*enode.Node) error { return nil }

	filter := gossipcrawler.PeerFilterFunc(func(n *enode.Node) bool { return true })

	scorer := func(peer.ID) float64 { return 0 }
	g, err := NewGossipPeerCrawler(t.Context(), &Service{}, mockListener, 2*time.Second, 10*time.Millisecond, 2, filter, scorer)
	if err != nil {
		t.Fatalf("NewGossipPeerCrawler error: %v", err)
	}

	topic1 := "test/topic1"
	topic2 := "test/topic2"
	g.topicExtractor = func(ctx context.Context, n *enode.Node) ([]string, error) { return []string{topic1, topic2}, nil }

	// Single crawl to index topics
	g.crawl()

	// Remove one topic and assert it is pruned from all indexes
	g.RemoveTopic(topic1)

	g.crawledPeers.mu.RLock()
	defer g.crawledPeers.mu.RUnlock()

	if _, ok := g.crawledPeers.peersByTopic[topic1]; ok {
		t.Fatalf("expected topic1 to be removed from byTopic")
	}

	// Ensure peer still exists and retains topic2
	for _, pn := range g.crawledPeers.peerNodeByEnode {
		if _, has1 := pn.topics[topic1]; has1 {
			t.Fatalf("expected topic1 to be removed from peer topics")
		}
		if _, has2 := pn.topics[topic2]; !has2 {
			t.Fatalf("expected topic2 to remain for peer")
		}
	}
}
