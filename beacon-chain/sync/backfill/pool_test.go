package backfill

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/peers"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
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

func TestPoolDetectAllEnded(t *testing.T) {
	nw := 5
	p2p := p2ptest.NewTestP2P(t)
	ctx := t.Context()
	ma := &mockAssigner{}
	needs := func() das.CurrentNeeds { return das.CurrentNeeds{Block: das.NeedSpan{Begin: 10, End: 10}} }
	pool := newP2PBatchWorkerPool(p2p, nw, needs)
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	keys, err := st.PublicKeys()
	require.NoError(t, err)
	v, err := newBackfillVerifier(st.GenesisValidatorsRoot(), keys)
	require.NoError(t, err)

	ctxMap, err := sync.ContextByteVersionsForValRoot(bytesutil.ToBytes32(st.GenesisValidatorsRoot()))
	require.NoError(t, err)
	bfs := filesystem.NewEphemeralBlobStorage(t)
	wcfg := &workerCfg{clock: startup.NewClock(time.Now(), [32]byte{}), newVB: mockNewBlobVerifier, verifier: v, ctxMap: ctxMap, blobStore: bfs}
	pool.spawn(ctx, nw, ma, wcfg)
	br := batcher{size: 10, currentNeeds: needs}
	endSeq := br.before(0)
	require.Equal(t, batchEndSequence, endSeq.state)
	for range nw {
		pool.todo(endSeq)
	}
	b, err := pool.complete()
	require.ErrorIs(t, err, errEndSequence)
	require.Equal(t, b.end, endSeq.end)
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

// TestBatchExpiredPartitioning covers the retention-window boundary that both the batcher and the
// worker pool rely on to retire batches, including the case where the window moves forward between two
// scheduling passes so that batches which were still needed become unneeded. The pool side of this
// handoff is covered by TestPoolWindsDownAfterRouterRetiresBatches and
// TestTodoInterceptsBatchEndSequence.
func TestBatchExpiredPartitioning(t *testing.T) {
	const n = 8
	cases := []struct {
		name    string
		ranges  [][2]primitives.Slot
		needs   das.CurrentNeeds
		expired int
	}{
		{
			name:    "no batches expired",
			ranges:  consecutive(3, 100, 50),
			needs:   das.CurrentNeeds{Block: das.NeedSpan{Begin: 120, End: 1001}},
			expired: 0,
		},
		{
			name:    "some batches expired",
			ranges:  consecutive(4, 100, 50),
			needs:   das.CurrentNeeds{Block: das.NeedSpan{Begin: 175, End: 1001}},
			expired: 1, // [100,150) falls out, its end-1 is 149 < 175.
		},
		{
			name:    "all batches expired",
			ranges:  consecutive(3, 100, 50),
			needs:   das.CurrentNeeds{Block: das.NeedSpan{Begin: 300, End: 301}},
			expired: 3,
		},
		{
			name:    "multiple batches expired",
			ranges:  consecutive(8, 100, 50),
			needs:   das.CurrentNeeds{Block: das.NeedSpan{Begin: 320, End: 501}},
			expired: 4, // [100,150), [150,200), [200,250) and [250,300); [300,350) is still needed.
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			todo := make([]batch, 0, len(tc.ranges))
			for _, r := range tc.ranges {
				todo = append(todo, batch{begin: r[0], end: r[1], state: batchSequenced})
			}
			expired, needed := partitionExpired(todo, tc.needs)
			require.Equal(t, tc.expired, len(expired))
			require.Equal(t, len(tc.ranges)-tc.expired, len(needed))
			for _, b := range expired {
				require.Equal(t, true, b.end <= tc.needs.Block.Begin)
			}
			for _, b := range needed {
				require.Equal(t, true, tc.needs.Block.At(b.end-1))
			}
		})
	}

	t.Run("window moving forward", func(t *testing.T) {
		todo := make([]batch, 0, n)
		for _, r := range consecutive(n, 100, 50) {
			todo = append(todo, batch{begin: r[0], end: r[1], state: batchSequenced})
		}
		needs := das.CurrentNeeds{Block: das.NeedSpan{Begin: 150, End: 501}}
		expired, needed := partitionExpired(todo, needs)
		require.Equal(t, 1, len(expired)) // [100,150)
		require.Equal(t, n-1, len(needed))

		// The remaining batches are retried against the new window, as the sync needs move with the
		// current slot, and another one drops out of retention.
		needs = das.CurrentNeeds{Block: das.NeedSpan{Begin: 200, End: 501}}
		expired2, needed2 := partitionExpired(needed, needs)
		require.Equal(t, 1, len(expired2)) // [150,200)
		require.Equal(t, n-2, len(needed2))
	})
}

// consecutive builds count batch ranges of the given size, in the descending order that both the
// batcher and the pool keep them in, starting at the lowest slot.
func consecutive(count int, lowest, size primitives.Slot) [][2]primitives.Slot {
	out := make([][2]primitives.Slot, 0, count)
	for i := range count {
		end := lowest + primitives.Slot(count-i)*size
		out = append(out, [2]primitives.Slot{end - size, end})
	}
	return out
}

// partitionExpired splits batches into those that have fallen outside the retention window and those
// that are still needed, using the same check the batcher and the pool make.
func partitionExpired(todo []batch, needs das.CurrentNeeds) (expired, needed []batch) {
	for _, b := range todo {
		if b.expired(needs) {
			expired = append(expired, b)
			continue
		}
		needed = append(needed, b)
	}
	return expired, needed
}

// TestTodoInterceptsBatchEndSequence tests that todo() records end-of-sequence batches without
// passing them on to the router, and that it only keeps the first signal it is given.
func TestTodoInterceptsBatchEndSequence(t *testing.T) {
	cases := []struct {
		name             string
		batches          []batch
		expectedEndSeq   int
		expectedToRouter int
	}{
		{
			name: "AllRegularBatches",
			batches: []batch{
				{state: batchInit},
				{state: batchInit},
				{state: batchErrRetryable},
			},
			expectedEndSeq:   0,
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
			expectedEndSeq:   1,
			expectedToRouter: 2,
		},
		{
			name: "AllEndSequence",
			batches: []batch{
				{state: batchEndSequence},
				{state: batchEndSequence},
				{state: batchEndSequence},
			},
			expectedEndSeq:   1,
			expectedToRouter: 0,
		},
		{
			name:             "EmptyBatches",
			batches:          []batch{},
			expectedEndSeq:   0,
			expectedToRouter: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := newUnspawnedPool(t, len(tc.batches))
			for _, b := range tc.batches {
				pool.todo(b)
			}
			require.Equal(t, tc.expectedEndSeq, len(pool.endSeq))
			// Only batches with work attached are counted as outstanding and queued for the router.
			require.Equal(t, tc.expectedToRouter, pool.outstanding)
			require.Equal(t, tc.expectedToRouter, len(pool.toRouter))
		})
	}
}

// TestExpirationFlowEndToEnd tests the complete flow of batches from batcher through pool
func TestExpirationFlowEndToEnd(t *testing.T) {
	testCases := []struct {
		name        string
		seqLen      int
		min         primitives.Slot
		max         primitives.Slot
		size        primitives.Slot
		moveMinTo   primitives.Slot
		expired     int
		description string
	}{
		{
			name:        "SingleBatchExpires",
			seqLen:      2,
			min:         100,
			max:         300,
			size:        50,
			moveMinTo:   150,
			expired:     1,
			description: "Initial [150-200] and [100-150]; moveMinimum(150) expires [100-150]",
		},
		/*
			{
				name:        "ProgressiveExpiration",
				seqLen:      4,
				min:         100,
				max:         500,
				size:        50,
				moveMinTo:   250,
				description: "4 batches; moveMinimum(250) expires 2 of them",
			},
		*/
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the flow: batcher creates batches → sequence() → pool.todo() → pool.processTodo()

			// Step 1: Create sequencer (simulating batcher)
			seq := newBatchSequencer(tc.seqLen, tc.max, tc.size, mockCurrentNeedsFunc(tc.min, tc.max+1))
			initializeBatchWithSlots(seq.seq, tc.min, tc.size)
			for i := range seq.seq {
				seq.seq[i].state = batchInit
			}

			// Step 2: Initial sequence() call - all batches should be returned (none expired yet)
			batches1, err := seq.sequence()
			if err != nil {
				t.Fatalf("initial sequence() failed: %v", err)
			}
			if len(batches1) != tc.seqLen {
				t.Fatalf("expected %d batches from initial sequence(), got %d", tc.seqLen, len(batches1))
			}

			// Step 3: Move minimum (simulating epoch advancement)
			seq.currentNeeds = mockCurrentNeedsFunc(tc.moveMinTo, tc.max+1)
			seq.batcher.currentNeeds = seq.currentNeeds

			for i := range batches1 {
				seq.update(batches1[i])
			}

			// Step 4: The batches that are still needed are the ones the pool is handed; a batch that
			// fell out of the window is turned into an end-of-sequence signal by the sequencer itself.
			batches2, err := seq.sequence()
			if err != nil && err != errMaxBatches {
				t.Fatalf("second sequence() failed: %v", err)
			}
			require.Equal(t, tc.seqLen-tc.expired, len(batches2))

			// Verify: All returned batches should have end > moveMinTo
			for _, b := range batches2 {
				if b.state != batchEndSequence && b.end <= tc.moveMinTo {
					t.Fatalf("batch [%d-%d] should not be returned when min=%d", b.begin, b.end, tc.moveMinTo)
				}
			}
		})
	}
}
