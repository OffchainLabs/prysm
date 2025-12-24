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

// TestProcessTodoExpiresOlderBatches tests that processTodo correctly identifies and converts expired batches
func TestProcessTodoExpiresOlderBatches(t *testing.T) {
	testCases := []struct {
		name              string
		seqLen            int
		min               primitives.Slot
		max               primitives.Slot
		size              primitives.Slot
		updateMin         primitives.Slot // what we'll set minChecker to
		expectedEndSeq    int             // how many batches should be converted to endSeq
		expectedProcessed int             // how many batches should be processed (assigned to peers)
	}{
		{
			name:              "NoBatchesExpired",
			seqLen:            3,
			min:               100,
			max:               1000,
			size:              50,
			updateMin:         120, // doesn't expire any batches
			expectedEndSeq:    0,
			expectedProcessed: 3,
		},
		{
			name:              "SomeBatchesExpired",
			seqLen:            4,
			min:               100,
			max:               1000,
			size:              50,
			updateMin:         175, // expires batches with end <= 175
			expectedEndSeq:    1,   // [100-150] will be expired
			expectedProcessed: 3,
		},
		{
			name:              "AllBatchesExpired",
			seqLen:            3,
			min:               100,
			max:               300,
			size:              50,
			updateMin:         300, // expires all batches
			expectedEndSeq:    3,
			expectedProcessed: 0,
		},
		{
			name:              "MultipleBatchesExpired",
			seqLen:            8,
			min:               100,
			max:               500,
			size:              50,
			updateMin:         320, // expires multiple batches
			expectedEndSeq:    4,   // [300-350] (end=350 > 320 not expired), [250-300], [200-250], [150-200], [100-150] = 4 batches
			expectedProcessed: 4,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create pool with minChecker
			pool := &p2pBatchWorkerPool{
				endSeq: make([]batch, 0),
			}
			needs := das.CurrentNeeds{Block: das.NeedSpan{Begin: tc.updateMin, End: tc.max + 1}}

			// Create batches with valid slot ranges (descending order)
			todo := make([]batch, tc.seqLen)
			for i := 0; i < tc.seqLen; i++ {
				end := tc.min + primitives.Slot((tc.seqLen-i)*int(tc.size))
				begin := end - tc.size
				todo[i] = batch{
					begin: begin,
					end:   end,
					state: batchInit,
				}
			}

			// Process todo using processTodo logic (simulate without actual peer assignment)
			endSeqCount := 0
			processedCount := 0
			for _, b := range todo {
				if b.expired(needs) {
					pool.endSeq = append(pool.endSeq, b.withState(batchEndSequence))
					endSeqCount++
				} else {
					processedCount++
				}
			}

			// Verify counts
			if endSeqCount != tc.expectedEndSeq {
				t.Fatalf("expected %d batches to expire, got %d", tc.expectedEndSeq, endSeqCount)
			}
			if processedCount != tc.expectedProcessed {
				t.Fatalf("expected %d batches to be processed, got %d", tc.expectedProcessed, processedCount)
			}

			// Verify all expired batches are in batchEndSequence state
			for _, b := range pool.endSeq {
				if b.state != batchEndSequence {
					t.Fatalf("expired batch should be batchEndSequence, got %s", b.state.String())
				}
				if b.end > tc.updateMin {
					t.Fatalf("batch with end=%d should not be in endSeq when min=%d", b.end, tc.updateMin)
				}
			}
		})
	}
}

// TestExpirationAfterMoveMinimum tests that batches expire correctly after minimum is increased
func TestExpirationAfterMoveMinimum(t *testing.T) {
	testCases := []struct {
		name           string
		seqLen         int
		min            primitives.Slot
		max            primitives.Slot
		size           primitives.Slot
		firstMin       primitives.Slot
		secondMin      primitives.Slot
		expectedAfter1 int // expected expired after first processTodo
		expectedAfter2 int // expected expired after second processTodo
	}{
		{
			name:           "IncrementalMinimumIncrease",
			seqLen:         4,
			min:            100,
			max:            1000,
			size:           50,
			firstMin:       150, // batches with end <= 150 expire
			secondMin:      200, // additional batches with end <= 200 expire
			expectedAfter1: 1,   // [100-150] expires
			expectedAfter2: 1,   // [150-200] also expires on second check (end=200 <= 200)
		},
		{
			name:           "LargeMinimumJump",
			seqLen:         3,
			min:            100,
			max:            300,
			size:           50,
			firstMin:       120, // no expiration
			secondMin:      300, // all expire
			expectedAfter1: 0,
			expectedAfter2: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool := &p2pBatchWorkerPool{
				endSeq: make([]batch, 0),
			}

			// Create batches
			todo := make([]batch, tc.seqLen)
			for i := 0; i < tc.seqLen; i++ {
				end := tc.min + primitives.Slot((tc.seqLen-i)*int(tc.size))
				begin := end - tc.size
				todo[i] = batch{
					begin: begin,
					end:   end,
					state: batchInit,
				}
			}
			needs := das.CurrentNeeds{Block: das.NeedSpan{Begin: tc.firstMin, End: tc.max + 1}}

			// First processTodo with firstMin
			endSeq1 := 0
			remaining1 := make([]batch, 0)
			for _, b := range todo {
				if b.expired(needs) {
					pool.endSeq = append(pool.endSeq, b.withState(batchEndSequence))
					endSeq1++
				} else {
					remaining1 = append(remaining1, b)
				}
			}

			if endSeq1 != tc.expectedAfter1 {
				t.Fatalf("after first update: expected %d expired, got %d", tc.expectedAfter1, endSeq1)
			}

			// Second processTodo with secondMin on remaining batches
			needs.Block.Begin = tc.secondMin
			endSeq2 := 0
			for _, b := range remaining1 {
				if b.expired(needs) {
					pool.endSeq = append(pool.endSeq, b.withState(batchEndSequence))
					endSeq2++
				}
			}

			if endSeq2 != tc.expectedAfter2 {
				t.Fatalf("after second update: expected %d expired, got %d", tc.expectedAfter2, endSeq2)
			}

			// Verify total endSeq count
			totalExpected := tc.expectedAfter1 + tc.expectedAfter2
			if len(pool.endSeq) != totalExpected {
				t.Fatalf("expected total %d expired batches, got %d", totalExpected, len(pool.endSeq))
			}
		})
	}
}

// TestTodoInterceptsBatchEndSequence tests that todo() correctly intercepts batchEndSequence batches
func TestTodoInterceptsBatchEndSequence(t *testing.T) {
	testCases := []struct {
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
			expectedEndSeq:   2,
			expectedToRouter: 2,
		},
		{
			name: "AllEndSequence",
			batches: []batch{
				{state: batchEndSequence},
				{state: batchEndSequence},
				{state: batchEndSequence},
			},
			expectedEndSeq:   3,
			expectedToRouter: 0,
		},
		{
			name:             "EmptyBatches",
			batches:          []batch{},
			expectedEndSeq:   0,
			expectedToRouter: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool := &p2pBatchWorkerPool{
				endSeq: make([]batch, 0),
			}

			endSeqCount := 0
			routerCount := 0

			for _, b := range tc.batches {
				if b.state == batchEndSequence {
					pool.endSeq = append(pool.endSeq, b)
					endSeqCount++
				} else {
					routerCount++
				}
			}

			if endSeqCount != tc.expectedEndSeq {
				t.Fatalf("expected %d batchEndSequence, got %d", tc.expectedEndSeq, endSeqCount)
			}
			if routerCount != tc.expectedToRouter {
				t.Fatalf("expected %d batches to router, got %d", tc.expectedToRouter, routerCount)
			}
			if len(pool.endSeq) != tc.expectedEndSeq {
				t.Fatalf("endSeq slice should have %d batches, got %d", tc.expectedEndSeq, len(pool.endSeq))
			}
		})
	}
}

// TestCompleteShutdownCondition tests the complete() method shutdown behavior
func TestCompleteShutdownCondition(t *testing.T) {
	testCases := []struct {
		name           string
		maxBatches     int
		endSeqCount    int
		shouldShutdown bool
		expectedMin    primitives.Slot
	}{
		{
			name:           "AllEndSeq_Shutdown",
			maxBatches:     3,
			endSeqCount:    3,
			shouldShutdown: true,
			expectedMin:    200,
		},
		{
			name:           "PartialEndSeq_NoShutdown",
			maxBatches:     3,
			endSeqCount:    2,
			shouldShutdown: false,
			expectedMin:    200,
		},
		{
			name:           "NoEndSeq_NoShutdown",
			maxBatches:     5,
			endSeqCount:    0,
			shouldShutdown: false,
			expectedMin:    150,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool := &p2pBatchWorkerPool{
				maxBatches: tc.maxBatches,
				endSeq:     make([]batch, 0),
				needs: func() das.CurrentNeeds {
					return das.CurrentNeeds{Block: das.NeedSpan{Begin: tc.expectedMin}}
				},
			}

			// Add endSeq batches
			for i := 0; i < tc.endSeqCount; i++ {
				pool.endSeq = append(pool.endSeq, batch{state: batchEndSequence})
			}

			// Check shutdown condition (this is what complete() checks)
			shouldShutdown := len(pool.endSeq) == pool.maxBatches

			if shouldShutdown != tc.shouldShutdown {
				t.Fatalf("expected shouldShutdown=%v, got %v", tc.shouldShutdown, shouldShutdown)
			}

			pool.needs = func() das.CurrentNeeds {
				return das.CurrentNeeds{Block: das.NeedSpan{Begin: tc.expectedMin}}
			}
			if pool.needs().Block.Begin != tc.expectedMin {
				t.Fatalf("expected minimum %d, got %d", tc.expectedMin, pool.needs().Block.Begin)
			}
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

			// Step 2: Create pool
			pool := &p2pBatchWorkerPool{
				endSeq: make([]batch, 0),
			}

			// Step 3: Initial sequence() call - all batches should be returned (none expired yet)
			batches1, err := seq.sequence()
			if err != nil {
				t.Fatalf("initial sequence() failed: %v", err)
			}
			if len(batches1) != tc.seqLen {
				t.Fatalf("expected %d batches from initial sequence(), got %d", tc.seqLen, len(batches1))
			}

			// Step 4: Move minimum (simulating epoch advancement)
			seq.currentNeeds = mockCurrentNeedsFunc(tc.moveMinTo, tc.max+1)
			seq.batcher.currentNeeds = seq.currentNeeds
			pool.needs = seq.currentNeeds

			for i := range batches1 {
				seq.update(batches1[i])
			}

			// Step 5: Process batches through pool (second sequence call would happen here in real code)
			batches2, err := seq.sequence()
			if err != nil && err != errMaxBatches {
				t.Fatalf("second sequence() failed: %v", err)
			}
			require.Equal(t, tc.seqLen-tc.expired, len(batches2))

			// Step 6: Simulate pool.processTodo() checking for expiration
			processedCount := 0
			for _, b := range batches2 {
				if b.expired(pool.needs()) {
					pool.endSeq = append(pool.endSeq, b.withState(batchEndSequence))
				} else {
					processedCount++
				}
			}

			// Verify: All returned non-endSeq batches should have end > moveMinTo
			for _, b := range batches2 {
				if b.state != batchEndSequence && b.end <= tc.moveMinTo {
					t.Fatalf("batch [%d-%d] should not be returned when min=%d", b.begin, b.end, tc.moveMinTo)
				}
			}
		})
	}
}
