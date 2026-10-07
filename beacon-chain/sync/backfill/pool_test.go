package backfill

import (
	"context"
	"testing"

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
