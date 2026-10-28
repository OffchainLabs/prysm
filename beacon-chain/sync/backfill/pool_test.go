package backfill

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
)

type mockAssigner struct {
	err    error
	assign []peer.ID
}

// Assign satisfies the PeerAssigner interface so that mockAssigner can be used in tests
// in place of the concrete p2p implementation of PeerAssigner.
func (m mockAssigner) Assign(filter peers.AssignmentFilter) ([]peer.ID, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.assign, nil
}

var _ PeerAssigner = &mockAssigner{}

func mockNewBlobVerifier(_ blocks.ROBlob, _ []verification.Requirement) verification.BlobVerifier {
	return &verification.MockBlobVerifier{}
}

func TestPoolReturnsExpiredBatch(t *testing.T) {
	nw := 2
	p2p := p2ptest.NewTestP2P(t)
	needs := func() das.CurrentNeeds { return das.CurrentNeeds{Block: das.NeedSpan{Begin: 100, End: 200}} }
	pool := newP2PBatchWorkerPool(p2p, nw, needs)
	ma := &mockAssigner{assign: []peer.ID{"peer"}}
	pool.spawn(t.Context(), nw, ma, &workerCfg{})
	expired := batch{begin: 10, end: 20, state: batchSequenced}
	pool.todo(expired)
	b, err := pool.complete()
	require.NoError(t, err)
	require.Equal(t, batchEndSequence, b.state)
	require.Equal(t, expired.begin, b.begin)
	require.Equal(t, expired.end, b.end)
}

type mockPool struct {
	spawnCalled  []int
	finishedChan chan batch
	finishedErr  chan error
	todoChan     chan batch
}

func (m *mockPool) spawn(_ context.Context, _ int, _ PeerAssigner, _ *workerCfg) {
}

func (m *mockPool) todo(b batch) {
	m.todoChan <- b
}

func (m *mockPool) complete() (batch, error) {
	select {
	case b := <-m.finishedChan:
		return b, nil
	case err := <-m.finishedErr:
		return batch{}, err
	}
}

var _ batchWorkerPool = &mockPool{}

// TestTodoInterceptsBatchEndSequence tests that todo() drops batchEndSequence batches instead of routing them to workers
func TestTodoInterceptsBatchEndSequence(t *testing.T) {
	testCases := []struct {
		name             string
		batches          []batch
		expectedToRouter int
	}{
		{
			name: "AllRegularBatches",
			batches: []batch{
				{state: batchInit},
				{state: batchInit},
				{state: batchErrRetryable},
			},
			expectedToRouter: 3,
		},
		{
			name: "MixedBatches",
			batches: []batch{
				{state: batchInit},
				{state: batchEndSequence},
				{state: batchInit},
				{state: batchEndSequence},
			},
			expectedToRouter: 2,
		},
		{
			name: "AllEndSequence",
			batches: []batch{
				{state: batchEndSequence},
				{state: batchEndSequence},
				{state: batchEndSequence},
			},
			expectedToRouter: 0,
		},
		{
			name:             "EmptyBatches",
			batches:          []batch{},
			expectedToRouter: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool := &p2pBatchWorkerPool{toRouter: make(chan batch, len(tc.batches))}
			for _, b := range tc.batches {
				pool.todo(b)
			}
			require.Equal(t, tc.expectedToRouter, len(pool.toRouter))
		})
	}
}

// TestProcessTodoExpiresEnvelopesWithoutPeers asserts that envelope attempt-budget expiry runs
// independently of peer assignment: a queued batch whose envelope budget has lapsed is routed
// onward to import even when no peer can be assigned, instead of stalling block backfill.
func TestProcessTodoExpiresEnvelopesWithoutPeers(t *testing.T) {
	ctx := t.Context()
	pool := newP2PBatchWorkerPool(p2ptest.NewTestP2P(t), 2, mockCurrentNeedsFunc(0, 1<<32))

	c := makeEnvChain(t, envChainCfg{start: 10, n: 3})
	cfg := testEnvSyncCfg(t, c, &downscoreRecorder{})
	es, err := newEnvelopeSync(ctx, c.blks, cfg)
	require.NoError(t, err)
	b := batch{begin: 10, end: 13, blocks: c.blks, columns: &columnSync{}, envelopes: es, state: batchSyncEnvelopes}

	// No peer is ever assignable in this test.
	noPeers := &mockAssigner{err: peers.ErrInsufficientSuitable}

	// Before any attempt the budget is not anchored, so the batch must stay queued: giving up
	// before the first attempt would skip slots that were never requested.
	todo, err := pool.processTodo([]batch{b}, noPeers, map[peer.ID]bool{})
	require.NoError(t, err)
	require.Equal(t, 1, len(todo))

	// One empty attempt anchors the budget; then the budget lapses while no peer is available.
	f := &scriptedFetcher{}
	es.fetchPass(ctx, "peer-a", f.fetch)
	require.Equal(t, true, es.unresolved() > 0)
	now := time.Now()
	es.now = func() time.Time { return now.Add(time.Hour) }

	todo, err = pool.processTodo(todo, noPeers, map[peer.ID]bool{})
	require.NoError(t, err)
	require.Equal(t, 0, len(todo))
	select {
	case done := <-pool.fromRouter:
		require.Equal(t, batchImportable, done.state)
		require.Equal(t, 0, done.envelopes.unresolved())
		require.Equal(t, 2, len(done.envelopes.skips[envSkipPeerExhausted]))
	default:
		t.Fatal("expected the expired batch to be routed for import")
	}
}

// TestEnvelopeRetryExclusions asserts the envelope stage prefers a peer other than the previous
// assignment without mutating the shared busy set.
func TestEnvelopeRetryExclusions(t *testing.T) {
	c := makeEnvChain(t, envChainCfg{start: 10, n: 2})
	es, err := newEnvelopeSync(t.Context(), c.blks, testEnvSyncCfg(t, c, &downscoreRecorder{}))
	require.NoError(t, err)

	busy := map[peer.ID]bool{"busy-peer": true}
	// Wrong state or no prior attempt: no exclusions.
	require.IsNil(t, batch{state: batchSequenced, envelopes: es}.envelopeRetryExclusions(busy))
	require.IsNil(t, batch{state: batchSyncEnvelopes, envelopes: es}.envelopeRetryExclusions(busy))
	require.IsNil(t, batch{state: batchSyncEnvelopes}.envelopeRetryExclusions(busy))

	es.peer = "prev-peer"
	avoid := batch{state: batchSyncEnvelopes, envelopes: es}.envelopeRetryExclusions(busy)
	require.Equal(t, 2, len(avoid))
	require.Equal(t, true, avoid["prev-peer"])
	require.Equal(t, true, avoid["busy-peer"])
	require.Equal(t, 1, len(busy)) // the shared busy set is untouched
}
