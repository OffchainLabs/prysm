package sync

import (
	"math/rand"
	"slices"
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/partialattestationbroadcaster"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/partialmsgmux"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// newPartialAttWireNode runs a partial attestation broadcaster over a real
// libp2p host, wired to the sync service exactly as the beacon node does it.
func newPartialAttWireNode(t *testing.T, ts *partialAttTestSetup) (host.Host, *partialattestationbroadcaster.Broadcaster) {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	b := partialattestationbroadcaster.NewBroadcaster(t.Context(), ts.s.cfg.clock.CurrentSlot)
	mux := partialmsgmux.New()
	mux.RegisterAttestationHandler(b)
	ps, err := pubsub.NewGossipSub(t.Context(), h, mux.AppendPubSubOpts(nil)...)
	require.NoError(t, err)
	go b.Start(ts.s.processPartialAttestation)

	topic, err := ps.Join(ts.topic, pubsub.RequestPartialMessages())
	require.NoError(t, err)
	_, err = topic.Subscribe()
	require.NoError(t, err)
	return h, b
}

// The full loop over a real wire: A originates through Submit, the bundle
// crosses real gossipsub, and B validates it through the real sync pipeline
// into its pool and out to classic gossip.
func TestPartialAttestationEndToEnd(t *testing.T) {
	now := time.Now()
	db := dbtest.SetupDB(t)
	tsA := newPartialAttTestSetupAt(t, now, db)
	tsB := newPartialAttTestSetupAt(t, now, db)
	// The setups are deterministic, so both nodes hold the same chain.
	require.DeepEqual(t, tsA.data, tsB.data)
	require.Equal(t, tsA.topic, tsB.topic)

	hA, bA := newPartialAttWireNode(t, tsA)
	hB, _ := newPartialAttWireNode(t, tsB)
	require.NoError(t, hA.Connect(t.Context(), peer.AddrInfo{ID: hB.ID(), Addrs: hB.Addrs()}))

	// Originate two attestations at A as locally produced (pre-validated).
	stateA := tsA.s.cfg.chain.(*mockChain.ChainService).State
	for _, pos := range []uint64{0, 1} {
		bA.Submit(tsA.topic, &ethpb.SingleAttestation{
			CommitteeId:   0,
			AttesterIndex: tsA.committee[pos],
			Data:          tsA.data,
			Signature:     tsA.sign(t, stateA, pos),
		})
	}

	// B's pool fills through the real validation pipeline: pushed once the
	// mesh forms, or recovered via the advertise -> request path.
	require.Eventually(t, func() bool {
		return tsB.s.cfg.attPool.UnaggregatedAttestationCount() == 2
	}, 30*time.Second, 100*time.Millisecond, "attestations never reached node B's pool")

	wantRoot, err := tsA.data.HashTreeRoot()
	require.NoError(t, err)
	for _, att := range tsB.s.cfg.attPool.UnaggregatedAttestations() {
		gotRoot, err := att.GetData().HashTreeRoot()
		require.NoError(t, err)
		require.Equal(t, wantRoot, gotRoot)
	}
	// B rebroadcasts accepted attestations on classic gossip for
	// non-partial peers.
	require.Equal(t, true, tsB.p.BroadcastCalled.Load(), "node B must rebroadcast on classic gossip")
}

// Multi-hop relay over a sparse 32-node topology (chain backbone + seeded
// random edges): far nodes are reachable only through store-and-forward
// re-origination, each hop backed by real validation.
func TestPartialAttestationMultiHop(t *testing.T) {
	const nodes = 32
	now := time.Now()
	db := dbtest.SetupDB(t)

	setups := make([]*partialAttTestSetup, nodes)
	hosts := make([]host.Host, nodes)
	bcasters := make([]*partialattestationbroadcaster.Broadcaster, nodes)
	for i := range nodes {
		setups[i] = newPartialAttTestSetupAt(t, now, db)
		hosts[i], bcasters[i] = newPartialAttWireNode(t, setups[i])
	}

	// Chain backbone for guaranteed connectivity, then 4-5 random extra
	// edges per node. The seed is fixed so the topology is reproducible.
	rng := rand.New(rand.NewSource(1))
	adj := make([]map[int]bool, nodes)
	for i := range adj {
		adj[i] = make(map[int]bool)
	}
	connect := func(a, b int) {
		if a == b || adj[a][b] {
			return
		}
		adj[a][b], adj[b][a] = true, true
		require.NoError(t, hosts[a].Connect(t.Context(), peer.AddrInfo{ID: hosts[b].ID(), Addrs: hosts[b].Addrs()}))
	}
	for i := range nodes - 1 {
		connect(i, i+1)
	}
	for i := range nodes {
		for range 4 + rng.Intn(2) {
			connect(i, rng.Intn(nodes))
		}
	}

	// The topology must actually require relaying. At this density the
	// diameter is 2, so the claim is that most nodes are out of the
	// origin's reach and depend on re-origination by an intermediate.
	dist := bfsDistances(adj, 0)
	require.Equal(t, true, slices.Max(dist) >= 2, "every node neighbors the origin")
	beyond := 0
	for _, d := range dist {
		if d >= 2 {
			beyond++
		}
	}
	require.Equal(t, true, beyond > nodes/2, "topology too dense: only %d nodes beyond one hop", beyond)

	// Originate two attestations at node 0.
	state0 := setups[0].s.cfg.chain.(*mockChain.ChainService).State
	for _, pos := range []uint64{0, 1} {
		bcasters[0].Submit(setups[0].topic, &ethpb.SingleAttestation{
			CommitteeId:   0,
			AttesterIndex: setups[0].committee[pos],
			Data:          setups[0].data,
			Signature:     setups[0].sign(t, state0, pos),
		})
	}

	// Every other node's pool must fill through the real validation
	// pipeline. The origin never pools its own submissions: Submit is the
	// pre-validated ingress, pooling is its callers' job.
	filled := func() bool {
		for i := 1; i < nodes; i++ {
			if setups[i].s.cfg.attPool.UnaggregatedAttestationCount() != 2 {
				return false
			}
		}
		return true
	}
	deadline := time.Now().Add(90 * time.Second)
	for !filled() && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
	}
	if !filled() {
		for i := 1; i < nodes; i++ {
			if n := setups[i].s.cfg.attPool.UnaggregatedAttestationCount(); n != 2 {
				t.Logf("node %d (distance %d): %d attestations", i, dist[i], n)
			}
		}
		t.Fatal("attestations never reached every node's pool")
	}

	// No connections exist beyond the wired edges, so nodes past distance 1
	// can only have been reached by store-and-forward re-origination.
	idx := make(map[peer.ID]int, nodes)
	for i, h := range hosts {
		idx[h.ID()] = i
	}
	for i, h := range hosts {
		for _, pid := range h.Network().Peers() {
			j, ok := idx[pid]
			require.Equal(t, true, ok, "node %d connected to an unknown peer", i)
			require.Equal(t, true, adj[i][j], "node %d holds an unwired connection to node %d", i, j)
		}
	}
}

// bfsDistances returns hop counts from start over the adjacency sets.
func bfsDistances(adj []map[int]bool, start int) []int {
	dist := make([]int, len(adj))
	for i := range dist {
		dist[i] = -1
	}
	dist[start] = 0
	queue := []int{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for next := range adj[cur] {
			if dist[next] == -1 {
				dist[next] = dist[cur] + 1
				queue = append(queue, next)
			}
		}
	}
	return dist
}
