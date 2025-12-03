package p2p

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/gossipsubcrawler"
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
		pidsByTopic:     make(map[gossipsubcrawler.Topic]map[peer.ID]struct{}),
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
		topics:   make(map[gossipsubcrawler.Topic]struct{}),
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

	topic1 := gossipsubcrawler.Topic("t1")
	topic2 := gossipsubcrawler.Topic("t2")

	cases := []struct {
		name  string
		prep  func(*crawledPeers)
		topic gossipsubcrawler.Topic
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
				_, ok := cp.pidsByTopic[topic1]
				require.False(t, ok)
				for _, p := range cp.peerNodeByPid {
					_, has := p.topics[topic1]
					require.False(t, has)
				}
				// Ensure other topics remain
				_, ok = cp.pidsByTopic[topic2]
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
				_, ok := cp.pidsByTopic[topic2]
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
			require.Len(t, cp.pidsByTopic, tc.wantTopics)
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
			require.Len(t, cp.pidsByTopic, tc.wantTopics)
			require.Len(t, cp.peerNodeByPid, tc.wantPeers)
		})
	}
}

func TestUpdateCrawledIfNewer(t *testing.T) {
	newCrawler := func() (*crawledPeers, *GossipsubPeerCrawler, func()) {
		ctx, cancel := context.WithCancel(context.Background())
		g := &GossipsubPeerCrawler{
			ctx:    ctx,
			cancel: cancel,
			pingCh: make(chan enode.Node, 8),
		}
		cp := newTestCrawledPeers()
		cp.g = g
		return cp, g, cancel
	}

	// Helper: non-blocking receive from ping channel
	recvPing := func(ch <-chan enode.Node) (enode.Node, bool) {
		select {
		case n := <-ch:
			return n, true
		default:
			return enode.Node{}, false
		}
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
		name         string
		arrange      func(*crawledPeers)
		invokeNode   *enode.Node
		invokeTopics []string
		assert       func(*testing.T, *crawledPeers, <-chan enode.Node)
	}{
		{
			name:         "new peer with topics adds and pings once",
			arrange:      func(cp *crawledPeers) {},
			invokeNode:   nodeA1,
			invokeTopics: []string{"a"},
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Len(t, cp.peerNodeByEnode, 1)
				require.Len(t, cp.peerNodeByPid, 1)
				require.Contains(t, cp.pidsByTopic, gossipsubcrawler.Topic("a"))
				cp.mu.RUnlock()
				if n, ok := recvPing(ch); !ok || n.ID() != nodeA1.ID() {
					t.Fatalf("expected one ping for nodeA1")
				}
				if _, ok := recvPing(ch); ok {
					t.Fatalf("expected exactly one ping")
				}
			},
		},
		{
			name:         "new peer with empty topics is removed and not pinged",
			arrange:      func(cp *crawledPeers) {},
			invokeNode:   nodeA1,
			invokeTopics: nil,
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Empty(t, cp.peerNodeByEnode)
				require.Empty(t, cp.peerNodeByPid)
				require.Empty(t, cp.pidsByTopic)
				cp.mu.RUnlock()
				if _, ok := recvPing(ch); ok {
					t.Fatalf("did not expect ping when topics empty")
				}
			},
		},
		{
			name: "existing peer lower seq is ignored (no update, no ping)",
			arrange: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, nodeA2, []string{"x"}, false) // higher seq exists
			},
			invokeNode:   nodeA1, // lower seq
			invokeTopics: []string{"a", "b"},
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Contains(t, cp.pidsByTopic, gossipsubcrawler.Topic("x"))
				require.NotContains(t, cp.pidsByTopic, gossipsubcrawler.Topic("a"))
				cp.mu.RUnlock()
				if _, ok := recvPing(ch); ok {
					t.Fatalf("did not expect ping for lower/equal seq")
				}
			},
		},
		{
			name: "existing peer equal seq is ignored (no update, no ping)",
			arrange: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, nodeA1, []string{"x"}, false)
			},
			invokeNode:   nodeA1,
			invokeTopics: []string{"a"},
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Contains(t, cp.pidsByTopic, gossipsubcrawler.Topic("x"))
				require.NotContains(t, cp.pidsByTopic, gossipsubcrawler.Topic("a"))
				cp.mu.RUnlock()
				if _, ok := recvPing(ch); ok {
					t.Fatalf("did not expect ping for equal seq")
				}
			},
		},
		{
			name: "existing peer higher seq updates topics and pings if not pinged",
			arrange: func(cp *crawledPeers) {
				addPeerWithTopics(t, cp, nodeA1, []string{"x"}, false)
			},
			invokeNode:   nodeA2,
			invokeTopics: []string{"a"},
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.NotContains(t, cp.pidsByTopic, gossipsubcrawler.Topic("x"))
				require.Contains(t, cp.pidsByTopic, gossipsubcrawler.Topic("a"))
				cp.mu.RUnlock()
				if n, ok := recvPing(ch); !ok || n.ID() != nodeA2.ID() {
					t.Fatalf("expected one ping for updated node")
				}
			},
		},
		{
			name: "existing peer higher seq with already pinged does not ping",
			arrange: func(cp *crawledPeers) {
				p := addPeerWithTopics(t, cp, nodeA1, []string{"x"}, true)
				// ensure pinged flag set
				require.True(t, p.isPinged)
			},
			invokeNode:   nodeA2,
			invokeTopics: []string{"a"},
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Contains(t, cp.pidsByTopic, gossipsubcrawler.Topic("a"))
				cp.mu.RUnlock()
				if _, ok := recvPing(ch); ok {
					t.Fatalf("did not expect ping when already pinged")
				}
			},
		},
		{
			name: "existing peer higher seq but empty topics removes peer and doesn't ping",
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
				if _, ok := recvPing(ch); ok {
					t.Fatalf("did not expect ping when topics empty on update")
				}
			},
		},
		{
			name: "corrupted existing entry with nil node is ignored (no change, no ping)",
			arrange: func(cp *crawledPeers) {
				pid, _ := enodeToPeerID(nodeA1)
				cp.mu.Lock()
				cp.peerNodeByEnode[nodeA1.ID()] = &peerNode{node: nil, peerID: pid, topics: map[gossipsubcrawler.Topic]struct{}{gossipsubcrawler.Topic("x"): {}}}
				cp.peerNodeByPid[pid] = cp.peerNodeByEnode[nodeA1.ID()]
				cp.pidsByTopic[gossipsubcrawler.Topic("x")] = map[peer.ID]struct{}{pid: {}}
				cp.mu.Unlock()
			},
			invokeNode:   nodeA2,
			invokeTopics: []string{"a"},
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Contains(t, cp.pidsByTopic, gossipsubcrawler.Topic("x"))
				cp.mu.RUnlock()
				if _, ok := recvPing(ch); ok {
					t.Fatalf("did not expect ping for corrupted entry")
				}
			},
		},
		{
			name:         "new peer with no ports causes enodeToPeerID error; no add, no ping",
			arrange:      func(cp *crawledPeers) {},
			invokeNode:   newNodeNoPorts(t),
			invokeTopics: []string{"a"},
			assert: func(t *testing.T, cp *crawledPeers, ch <-chan enode.Node) {
				cp.mu.RLock()
				require.Empty(t, cp.peerNodeByEnode)
				require.Empty(t, cp.peerNodeByPid)
				require.Empty(t, cp.pidsByTopic)
				cp.mu.RUnlock()
				if _, ok := recvPing(ch); ok {
					t.Fatalf("did not expect ping when enodeToPeerID fails")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cp, g, cancel := newCrawler()
			defer cancel()
			tc.arrange(cp)
			cp.updateCrawledIfNewer(tc.invokeNode, tc.invokeTopics)
			tc.assert(t, cp, g.pingCh)
		})
	}
}

func TestPeersForTopic(t *testing.T) {
	t.Parallel()

	newCrawler := func(filter gossipsubcrawler.PeerFilterFunc) (*GossipsubPeerCrawler, *crawledPeers) {
		g := &GossipsubPeerCrawler{
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
	topic := gossipsubcrawler.Topic("top")

	cases := []struct {
		name    string
		filter  gossipsubcrawler.PeerFilterFunc
		setup   func(t *testing.T, g *GossipsubPeerCrawler, cp *crawledPeers)
		wantIDs []enode.ID
	}{
		{
			name:    "no peers for topic returns empty",
			filter:  func(*enode.Node) bool { return true },
			setup:   func(t *testing.T, g *GossipsubPeerCrawler, cp *crawledPeers) {},
			wantIDs: nil,
		},
		{
			name:   "excludes unpinged peers",
			filter: func(*enode.Node) bool { return true },
			setup: func(t *testing.T, g *GossipsubPeerCrawler, cp *crawledPeers) {
				// Add one pinged and one not pinged on same topic
				addPeerWithTopics(t, cp, n1, []string{string(topic)}, true)
				addPeerWithTopics(t, cp, n2, []string{string(topic)}, false)
			},
			wantIDs: []enode.ID{n1.ID()},
		},
		{
			name:   "applies peer filter to exclude",
			filter: func(n *enode.Node) bool { return n.ID() != n2.ID() },
			setup: func(t *testing.T, g *GossipsubPeerCrawler, cp *crawledPeers) {
				addPeerWithTopics(t, cp, n1, []string{string(topic)}, true)
				addPeerWithTopics(t, cp, n2, []string{string(topic)}, true)
			},
			wantIDs: []enode.ID{n1.ID()},
		},
		{
			name:   "ignores dangling peerID in byTopic",
			filter: func(*enode.Node) bool { return true },
			setup: func(t *testing.T, g *GossipsubPeerCrawler, cp *crawledPeers) {
				addPeerWithTopics(t, cp, n1, []string{string(topic)}, true)
				// Add n2 then remove it from byPeerId to simulate dangling
				p2 := addPeerWithTopics(t, cp, n2, []string{string(topic)}, true)
				cp.mu.Lock()
				delete(cp.peerNodeByPid, p2.peerID)
				cp.mu.Unlock()
			},
			wantIDs: []enode.ID{n1.ID()},
		},
		{
			name:   "sorted by score descending",
			filter: func(*enode.Node) bool { return true },
			setup: func(t *testing.T, g *GossipsubPeerCrawler, cp *crawledPeers) {
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
			cp.g = g
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
	filter := gossipsubcrawler.PeerFilterFunc(func(n *enode.Node) bool { return true })

	// Create crawler with small intervals
	scorer := func(peer.ID) float64 { return 0 }
	g, err := NewGossipsubPeerCrawler(&Service{}, mockListener, 2*time.Second, 10*time.Millisecond, 4, filter, scorer)
	require.NoError(t, err)

	// Assign a simple topic extractor
	topic := "test/topic"
	topicExtractor := func(ctx context.Context, n *enode.Node) ([]string, error) {
		return []string{topic}, nil
	}

	// Run ping loop in background and perform a single crawl
	require.NoError(t, g.Start(topicExtractor))
	defer g.Stop()

	// Verify that the peer has been indexed under the topic and marked as pinged
	require2.Eventually(t, func() bool {
		g.crawledPeers.mu.RLock()
		defer g.crawledPeers.mu.RUnlock()

		peersByTopic := g.crawledPeers.pidsByTopic[gossipsubcrawler.Topic(topic)]
		if len(peersByTopic) == 0 {
			return false
		}
		// Fetch the single peerNode and check status
		for pid := range peersByTopic {
			pn := g.crawledPeers.peerNodeByPid[pid]
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
	filter := gossipsubcrawler.PeerFilterFunc(func(n *enode.Node) bool { return false })

	scorer := func(peer.ID) float64 { return 0 }
	g, err := NewGossipsubPeerCrawler(&Service{}, mockListener, 2*time.Second, 10*time.Millisecond, 2, filter, scorer)
	if err != nil {
		t.Fatalf("NewGossipsubPeerCrawler error: %v", err)
	}

	topic := "test/topic"
	g.topicExtractor = func(ctx context.Context, n *enode.Node) ([]string, error) { return []string{topic}, nil }

	g.crawl()

	// Verify no peers are indexed, because filter rejected the node
	g.crawledPeers.mu.RLock()
	defer g.crawledPeers.mu.RUnlock()
	if len(g.crawledPeers.peerNodeByEnode) != 0 || len(g.crawledPeers.peerNodeByPid) != 0 || len(g.crawledPeers.pidsByTopic) != 0 {
		t.Fatalf("expected no peers indexed, got byEnode=%d byPeerId=%d byTopic=%d",
			len(g.crawledPeers.peerNodeByEnode), len(g.crawledPeers.peerNodeByPid), len(g.crawledPeers.pidsByTopic))
	}
}

func TestCrawler_RemoveTopic_RemovesTopicFromIndexes(t *testing.T) {
	t.Parallel()

	localNode := createTestNodeRandom(t)
	node := localNode.Node()
	iterator := p2ptest.NewMockIterator([]*enode.Node{node})
	mockListener := p2ptest.NewMockListener(localNode, iterator)
	mockListener.PingFunc = func(*enode.Node) error { return nil }

	filter := gossipsubcrawler.PeerFilterFunc(func(n *enode.Node) bool { return true })

	scorer := func(peer.ID) float64 { return 0 }
	g, err := NewGossipsubPeerCrawler(&Service{}, mockListener, 2*time.Second, 10*time.Millisecond, 2, filter, scorer)
	if err != nil {
		t.Fatalf("NewGossipsubPeerCrawler error: %v", err)
	}

	topic1 := "test/topic1"
	topic2 := "test/topic2"
	g.topicExtractor = func(ctx context.Context, n *enode.Node) ([]string, error) { return []string{topic1, topic2}, nil }

	// Single crawl to index topics
	g.crawl()

	// Remove one topic and assert it is pruned from all indexes
	g.RemoveTopic(gossipsubcrawler.Topic(topic1))

	g.crawledPeers.mu.RLock()
	defer g.crawledPeers.mu.RUnlock()

	if _, ok := g.crawledPeers.pidsByTopic[gossipsubcrawler.Topic(topic1)]; ok {
		t.Fatalf("expected topic1 to be removed from byTopic")
	}

	// Ensure peer still exists and retains topic2
	for _, pn := range g.crawledPeers.peerNodeByEnode {
		if _, has1 := pn.topics[gossipsubcrawler.Topic(topic1)]; has1 {
			t.Fatalf("expected topic1 to be removed from peer topics")
		}
		if _, has2 := pn.topics[gossipsubcrawler.Topic(topic2)]; !has2 {
			t.Fatalf("expected topic2 to remain for peer")
		}
	}
}
