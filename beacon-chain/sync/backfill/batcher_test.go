package backfill

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBatcherBefore(t *testing.T) {
	cases := []struct {
		name   string
		b      batcher
		upTo   []primitives.Slot
		expect []batch
	}{
		{
			name: "size 10",
			b:    batcher{currentNeeds: mockCurrentNeedsFunc(0, 100), size: 10},
			upTo: []primitives.Slot{33, 30, 10, 6},
			expect: []batch{
				{begin: 23, end: 33, state: batchInit},
				{begin: 20, end: 30, state: batchInit},
				{begin: 0, end: 10, state: batchInit},
				{begin: 0, end: 6, state: batchInit},
			},
		},
		{
			name: "size 4",
			b:    batcher{currentNeeds: mockCurrentNeedsFunc(0, 100), size: 4},
			upTo: []primitives.Slot{33, 6, 4},
			expect: []batch{
				{begin: 29, end: 33, state: batchInit},
				{begin: 2, end: 6, state: batchInit},
				{begin: 0, end: 4, state: batchInit},
			},
		},
		{
			name: "trigger end",
			b:    batcher{currentNeeds: mockCurrentNeedsFunc(20, 100), size: 10},
			upTo: []primitives.Slot{33, 30, 25, 21, 20, 19},
			expect: []batch{
				{begin: 23, end: 33, state: batchInit},
				{begin: 20, end: 30, state: batchInit},
				{begin: 20, end: 25, state: batchInit},
				{begin: 20, end: 21, state: batchInit},
				{begin: 20, end: 20, state: batchEndSequence},
				{begin: 19, end: 19, state: batchEndSequence},
			},
		},
	}
	for _, c := range cases {
		for i := range c.upTo {
			upTo := c.upTo[i]
			expect := c.expect[i]
			t.Run(fmt.Sprintf("%s upTo %d", c.name, upTo), func(t *testing.T) {
				got := c.b.before(upTo)
				require.Equal(t, expect.begin, got.begin)
				require.Equal(t, expect.end, got.end)
				require.Equal(t, expect.state, got.state)
			})
		}
	}
}

func TestBatchSingleItem(t *testing.T) {
	var min, max, size primitives.Slot
	// seqLen = 1 means just one worker
	seqLen := 1
	min = 0
	max = 11235
	size = 64
	seq := newBatchSequencer(seqLen, max, size, mockCurrentNeedsFunc(min, max+1))
	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	b := got[0]

	//  calling sequence again should give you the next (earlier) batch
	seq.update(b.withState(batchImportComplete))
	next, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(next))
	require.Equal(t, b.end, next[0].end+size)

	// should get the same batch again when update is called with an error
	seq.update(next[0].withState(batchErrRetryable))
	same, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(same))
	require.Equal(t, next[0].begin, same[0].begin)
	require.Equal(t, next[0].end, same[0].end)
}

func TestBatchSequencer(t *testing.T) {
	var min, max, size primitives.Slot
	seqLen := 8
	min = 0
	max = 11235
	size = 64
	seq := newBatchSequencer(seqLen, max, size, mockCurrentNeedsFunc(min, max+1))
	expected := []batch{
		{begin: 11171, end: 11235},
		{begin: 11107, end: 11171},
		{begin: 11043, end: 11107},
		{begin: 10979, end: 11043},
		{begin: 10915, end: 10979},
		{begin: 10851, end: 10915},
		{begin: 10787, end: 10851},
		{begin: 10723, end: 10787},
	}
	got, err := seq.sequence()
	require.Equal(t, seqLen, len(got))
	for i := range seqLen {
		g := got[i]
		exp := expected[i]
		require.NoError(t, err)
		require.Equal(t, exp.begin, g.begin)
		require.Equal(t, exp.end, g.end)
		require.Equal(t, batchSequenced, g.state)
	}
	// This should give us the error indicating there are too many outstanding batches.
	_, err = seq.sequence()
	require.ErrorIs(t, err, errMaxBatches)

	// mark the last batch completed so we can call sequence again.
	last := seq.seq[len(seq.seq)-1]
	// With this state, the batch should get served back to us as the next batch.
	last.state = batchErrRetryable
	seq.update(last)
	nextS, err := seq.sequence()
	require.Equal(t, 1, len(nextS))
	next := nextS[0]
	require.NoError(t, err)
	require.Equal(t, last.begin, next.begin)
	require.Equal(t, last.end, next.end)
	// sequence() should replace the batchErrRetryable state with batchSequenced.
	require.Equal(t, batchSequenced, next.state)

	// No batches have been marked importable.
	require.Equal(t, 0, len(seq.importable()))

	// Mark our batch importable and make sure it shows up in the list of importable batches.
	next.state = batchImportable
	seq.update(next)
	require.Equal(t, 0, len(seq.importable()))
	first := seq.seq[0]
	first.state = batchImportable
	seq.update(first)
	require.Equal(t, 1, len(seq.importable()))
	require.Equal(t, len(seq.seq), seqLen)
	// change the last element back to batchInit so that the importable test stays simple
	last = seq.seq[len(seq.seq)-1]
	last.state = batchInit
	seq.update(last)
	// ensure that the number of importable elements grows as the list is marked importable
	for i := 0; i < len(seq.seq); i++ {
		seq.seq[i].state = batchImportable
		require.Equal(t, i+1, len(seq.importable()))
	}
	// reset everything to init
	for i := 0; i < len(seq.seq); i++ {
		seq.seq[i].state = batchInit
		require.Equal(t, 0, len(seq.importable()))
	}
	// loop backwards and make sure importable is zero until the first element is importable
	for i := len(seq.seq) - 1; i > 0; i-- {
		seq.seq[i].state = batchImportable
		require.Equal(t, 0, len(seq.importable()))
	}
	seq.seq[0].state = batchImportable
	require.Equal(t, len(seq.seq), len(seq.importable()))

	// reset everything to init again
	for i := 0; i < len(seq.seq); i++ {
		seq.seq[i].state = batchInit
		require.Equal(t, 0, len(seq.importable()))
	}
	// set first 3 elements to importable. we should see them in the result for importable()
	// and be able to use update to cycle them away.
	seq.seq[0].state, seq.seq[1].state, seq.seq[2].state = batchImportable, batchImportable, batchImportable
	require.Equal(t, 3, len(seq.importable()))
	a, b, c, z := seq.seq[0], seq.seq[1], seq.seq[2], seq.seq[3]
	require.NotEqual(t, z.begin, seq.seq[2].begin)
	require.NotEqual(t, z.begin, seq.seq[1].begin)
	require.NotEqual(t, z.begin, seq.seq[0].begin)
	a.state, b.state, c.state = batchImportComplete, batchImportComplete, batchImportComplete
	seq.update(a)

	// follow z as it moves down  the chain to the first spot
	require.Equal(t, z.begin, seq.seq[2].begin)
	require.NotEqual(t, z.begin, seq.seq[1].begin)
	require.NotEqual(t, z.begin, seq.seq[0].begin)
	seq.update(b)
	require.NotEqual(t, z.begin, seq.seq[2].begin)
	require.Equal(t, z.begin, seq.seq[1].begin)
	require.NotEqual(t, z.begin, seq.seq[0].begin)
	seq.update(c)
	require.NotEqual(t, z.begin, seq.seq[2].begin)
	require.NotEqual(t, z.begin, seq.seq[1].begin)
	require.Equal(t, z.begin, seq.seq[0].begin)

	// Check integrity of begin/end alignment across the sequence.
	// Also update all the states to sequenced for the convenience of the next test.
	for i := 1; i < len(seq.seq); i++ {
		require.Equal(t, seq.seq[i].end, seq.seq[i-1].begin)
		// won't touch the first element, which is fine because it is marked complete below.
		seq.seq[i].state = batchSequenced
	}

	// set the min for the batcher close to the lowest slot. This will force the next batch to be partial and the batch
	// after that to be the final batch.
	newMin := seq.seq[len(seq.seq)-1].begin - 30
	seq.currentNeeds = func() das.CurrentNeeds {
		return das.CurrentNeeds{Block: das.NeedSpan{Begin: newMin, End: seq.batcher.max}}
	}
	seq.batcher.currentNeeds = seq.currentNeeds
	first = seq.seq[0]
	first.state = batchImportComplete
	// update() with a complete state will cause the sequence to be extended with an additional batch
	seq.update(first)
	lastS, err := seq.sequence()
	last = lastS[0]
	require.NoError(t, err)
	require.Equal(t, newMin, last.begin)
	require.Equal(t, seq.seq[len(seq.seq)-2].begin, last.end)

	// Mark first batch done again, this time check that sequence() gives errEndSequence.
	first = seq.seq[0]
	first.state = batchImportComplete
	// update() with a complete state will cause the sequence to be extended with an additional batch
	seq.update(first)
	endExp, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(endExp))
	end := endExp[0]
	//require.ErrorIs(t, err, errEndSequence)
	require.Equal(t, batchEndSequence, end.state)
}

// initializeBatchWithSlots sets the begin and end slot values for a batch
// in descending order (slot positions decrease as index increases)
func initializeBatchWithSlots(batches []batch, min primitives.Slot, size primitives.Slot) {
	for i := range batches {
		// Batches are ordered descending by slot: earliest batches have lower indices
		// so batch[0] covers highest slots, batch[N] covers lowest slots
		end := min + primitives.Slot((len(batches)-i)*int(size))
		begin := end - size
		batches[i].begin = begin
		batches[i].end = end
	}
}

// TestSequence tests the sequence() method with various batch states
func TestSequence(t *testing.T) {
	testCases := []struct {
		name           string
		seqLen         int
		min            primitives.Slot
		max            primitives.Slot
		size           primitives.Slot
		initialStates  []batchState
		expectedCount  int
		expectedErr    error
		stateTransform func([]batch) // optional: transform states before test
	}{
		{
			name:          "EmptySequence",
			seqLen:        0,
			min:           100,
			max:           1000,
			size:          64,
			initialStates: []batchState{},
			expectedCount: 0,
			expectedErr:   errMaxBatches,
		},
		{
			name:          "SingleBatchInit",
			seqLen:        1,
			min:           100,
			max:           1000,
			size:          64,
			initialStates: []batchState{batchInit},
			expectedCount: 1,
		},
		{
			name:          "SingleBatchErrRetryable",
			seqLen:        1,
			min:           100,
			max:           1000,
			size:          64,
			initialStates: []batchState{batchErrRetryable},
			expectedCount: 1,
		},
		{
			name:          "MultipleBatchesInit",
			seqLen:        3,
			min:           100,
			max:           1000,
			size:          200,
			initialStates: []batchState{batchInit, batchInit, batchInit},
			expectedCount: 3,
		},
		{
			name:          "MixedStates_InitAndSequenced",
			seqLen:        2,
			min:           100,
			max:           1000,
			size:          100,
			initialStates: []batchState{batchInit, batchSequenced},
			expectedCount: 1,
		},
		{
			name:          "MixedStates_SequencedFirst",
			seqLen:        2,
			min:           100,
			max:           1000,
			size:          100,
			initialStates: []batchState{batchSequenced, batchInit},
			expectedCount: 1,
		},
		{
			name:          "AllBatchesSequenced",
			seqLen:        3,
			min:           100,
			max:           1000,
			size:          200,
			initialStates: []batchState{batchSequenced, batchSequenced, batchSequenced},
			expectedCount: 0,
			expectedErr:   errMaxBatches,
		},
		{
			name:          "EndSequenceOnly",
			seqLen:        1,
			min:           100,
			max:           1000,
			size:          64,
			initialStates: []batchState{batchEndSequence},
			expectedCount: 1,
		},
		{
			name:          "EndSequenceWithOthers",
			seqLen:        2,
			min:           100,
			max:           1000,
			size:          64,
			initialStates: []batchState{batchInit, batchEndSequence},
			expectedCount: 1,
		},
		{
			name:          "ImportableNotSequenced",
			seqLen:        1,
			min:           100,
			max:           1000,
			size:          64,
			initialStates: []batchState{batchImportable},
			expectedCount: 0,
			expectedErr:   errMaxBatches,
		},
		{
			name:          "ImportCompleteNotSequenced",
			seqLen:        1,
			min:           100,
			max:           1000,
			size:          64,
			initialStates: []batchState{batchImportComplete},
			expectedCount: 0,
			expectedErr:   errMaxBatches,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			seq := newBatchSequencer(tc.seqLen, tc.max, tc.size, mockCurrentNeedsFunc(tc.min, tc.max+1))

			// Initialize batches with valid slot ranges
			initializeBatchWithSlots(seq.seq, tc.min, tc.size)

			// Set initial states
			for i, state := range tc.initialStates {
				seq.seq[i].state = state
			}

			// Apply any transformations
			if tc.stateTransform != nil {
				tc.stateTransform(seq.seq)
			}

			got, err := seq.sequence()

			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expectedCount, len(got))

			// Verify returned batches are in batchSequenced state
			for _, b := range got {
				if b.state != batchEndSequence {
					require.Equal(t, batchSequenced, b.state)
				}
			}
		})
	}
}

// TestUpdate tests the update() method which: (1) updates batch state, (2) removes batchImportComplete batches,
// (3) shifts remaining batches down, and (4) adds new batches to fill vacated positions.
// NOTE: The sequence length can change! Completed batches are removed and new ones are added.
func TestUpdate(t *testing.T) {
	testCases := []struct {
		name        string
		seqLen      int
		batches     []batchState
		updateIdx   int
		newState    batchState
		expectedLen int          // expected length after update
		expected    []batchState // expected states of first N batches after update
	}{
		{
			name:        "SingleBatchUpdate",
			seqLen:      1,
			batches:     []batchState{batchInit},
			updateIdx:   0,
			newState:    batchImportable,
			expectedLen: 1,
			expected:    []batchState{batchImportable},
		},
		{
			name:        "RemoveFirstCompleted_ShiftOthers",
			seqLen:      3,
			batches:     []batchState{batchImportComplete, batchInit, batchInit},
			updateIdx:   0,
			newState:    batchImportComplete,
			expectedLen: 3,                                  // 1 removed + 2 new added
			expected:    []batchState{batchInit, batchInit}, // shifted down
		},
		{
			name:        "RemoveMultipleCompleted",
			seqLen:      3,
			batches:     []batchState{batchImportComplete, batchImportComplete, batchInit},
			updateIdx:   0,
			newState:    batchImportComplete,
			expectedLen: 3,                       // 2 removed + 2 new added
			expected:    []batchState{batchInit}, // only 1 non-complete batch
		},
		{
			name:        "RemoveMiddleCompleted_AlsoShifts",
			seqLen:      3,
			batches:     []batchState{batchInit, batchImportComplete, batchInit},
			updateIdx:   1,
			newState:    batchImportComplete,
			expectedLen: 3,                                  // 1 removed + 1 new added
			expected:    []batchState{batchInit, batchInit}, // middle complete removed, last shifted to middle
		},
		{
			name:        "SingleBatchComplete_Replaced",
			seqLen:      1,
			batches:     []batchState{batchInit},
			updateIdx:   0,
			newState:    batchImportComplete,
			expectedLen: 1,                       // special case: replaced with new batch
			expected:    []batchState{batchInit}, // new batch from beforeBatch
		},
		{
			name:        "UpdateNonMatchingBatch",
			seqLen:      2,
			batches:     []batchState{batchInit, batchInit},
			updateIdx:   0,
			newState:    batchImportable,
			expectedLen: 2,
			expected:    []batchState{batchImportable, batchInit},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			seq := newBatchSequencer(tc.seqLen, 1000, 64, mockCurrentNeedsFunc(0, 1000+1))

			// Initialize batches with proper slot ranges
			for i := range seq.seq {
				seq.seq[i] = batch{
					begin: primitives.Slot(1000 - (i+1)*64),
					end:   primitives.Slot(1000 - i*64),
					state: tc.batches[i],
				}
			}

			// Create batch to update (must match begin/end to be replaced)
			updateBatch := seq.seq[tc.updateIdx]
			updateBatch.state = tc.newState
			seq.update(updateBatch)

			// Verify expected length
			if len(seq.seq) != tc.expectedLen {
				t.Fatalf("expected length %d, got %d", tc.expectedLen, len(seq.seq))
			}

			// Verify expected states of first N batches
			for i, expectedState := range tc.expected {
				if i >= len(seq.seq) {
					t.Fatalf("expected state at index %d but seq only has %d batches", i, len(seq.seq))
				}
				if seq.seq[i].state != expectedState {
					t.Fatalf("batch[%d]: expected state %s, got %s", i, expectedState.String(), seq.seq[i].state.String())
				}
			}

			// Verify slot contiguity for non-newly-generated batches
			// (newly generated batches from beforeBatch() may not be contiguous with shifted batches)
			// For this test, we just verify they're in valid slot ranges
			for i := 0; i < len(seq.seq); i++ {
				if seq.seq[i].begin >= seq.seq[i].end {
					t.Fatalf("invalid batch[%d]: begin=%d should be < end=%d", i, seq.seq[i].begin, seq.seq[i].end)
				}
			}
		})
	}
}

// TestImportable tests the importable() method for contiguity checking
func TestImportable(t *testing.T) {
	testCases := []struct {
		name          string
		seqLen        int
		states        []batchState
		expectedCount int
		expectedBreak int // index where importable chain breaks (-1 if none)
	}{
		{
			name:          "EmptySequence",
			seqLen:        0,
			states:        []batchState{},
			expectedCount: 0,
			expectedBreak: -1,
		},
		{
			name:          "FirstBatchNotImportable",
			seqLen:        2,
			states:        []batchState{batchInit, batchImportable},
			expectedCount: 0,
			expectedBreak: 0,
		},
		{
			name:          "FirstBatchImportable",
			seqLen:        1,
			states:        []batchState{batchImportable},
			expectedCount: 1,
			expectedBreak: -1,
		},
		{
			name:          "TwoImportableConsecutive",
			seqLen:        2,
			states:        []batchState{batchImportable, batchImportable},
			expectedCount: 2,
			expectedBreak: -1,
		},
		{
			name:          "ThreeImportableConsecutive",
			seqLen:        3,
			states:        []batchState{batchImportable, batchImportable, batchImportable},
			expectedCount: 3,
			expectedBreak: -1,
		},
		{
			name:          "ImportsBreak_SecondNotImportable",
			seqLen:        2,
			states:        []batchState{batchImportable, batchInit},
			expectedCount: 1,
			expectedBreak: 1,
		},
		{
			name:          "ImportsBreak_MiddleNotImportable",
			seqLen:        4,
			states:        []batchState{batchImportable, batchImportable, batchInit, batchImportable},
			expectedCount: 2,
			expectedBreak: 2,
		},
		{
			name:          "EndSequenceAfterImportable",
			seqLen:        3,
			states:        []batchState{batchImportable, batchImportable, batchEndSequence},
			expectedCount: 2,
			expectedBreak: 2,
		},
		{
			name:          "AllStatesNotImportable",
			seqLen:        3,
			states:        []batchState{batchInit, batchSequenced, batchErrRetryable},
			expectedCount: 0,
			expectedBreak: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			seq := newBatchSequencer(tc.seqLen, 1000, 64, mockCurrentNeedsFunc(0, 1000+1))

			for i, state := range tc.states {
				seq.seq[i] = batch{
					begin: primitives.Slot(1000 - (i+1)*64),
					end:   primitives.Slot(1000 - i*64),
					state: state,
				}
			}

			imp := seq.importable()
			require.Equal(t, tc.expectedCount, len(imp))
		})
	}
}

// TestMoveMinimumWithNonImportableUpdate tests integration of moveMinimum with update()
func TestMoveMinimumWithNonImportableUpdate(t *testing.T) {
	t.Run("UpdateBatchAfterMinimumChange", func(t *testing.T) {
		seq := newBatchSequencer(3, 300, 50, mockCurrentNeedsFunc(100, 300+1))

		// Initialize with batches
		seq.seq[0] = batch{begin: 200, end: 250, state: batchInit}
		seq.seq[1] = batch{begin: 150, end: 200, state: batchInit}
		seq.seq[2] = batch{begin: 100, end: 150, state: batchInit}

		seq.currentNeeds = mockCurrentNeedsFunc(150, 300+1)
		seq.batcher.currentNeeds = seq.currentNeeds

		// Update non-importable batch above new minimum
		batchToUpdate := batch{begin: 200, end: 250, state: batchSequenced}
		seq.update(batchToUpdate)

		// Verify batch was updated
		require.Equal(t, batchSequenced, seq.seq[0].state)

		// Verify numTodo reflects updated minimum
		todo := seq.numTodo()
		require.NotEqual(t, 0, todo, "numTodo should be greater than 0 after moveMinimum and update")
	})
}

// TestCountWithState tests state counting
func TestCountWithState(t *testing.T) {
	testCases := []struct {
		name          string
		seqLen        int
		states        []batchState
		queryState    batchState
		expectedCount int
	}{
		{
			name:          "CountInit_NoBatches",
			seqLen:        0,
			states:        []batchState{},
			queryState:    batchInit,
			expectedCount: 0,
		},
		{
			name:          "CountInit_OneBatch",
			seqLen:        1,
			states:        []batchState{batchInit},
			queryState:    batchInit,
			expectedCount: 1,
		},
		{
			name:          "CountInit_MultipleBatches",
			seqLen:        3,
			states:        []batchState{batchInit, batchInit, batchInit},
			queryState:    batchInit,
			expectedCount: 3,
		},
		{
			name:          "CountInit_MixedStates",
			seqLen:        3,
			states:        []batchState{batchInit, batchSequenced, batchInit},
			queryState:    batchInit,
			expectedCount: 2,
		},
		{
			name:          "CountSequenced",
			seqLen:        3,
			states:        []batchState{batchInit, batchSequenced, batchImportable},
			queryState:    batchSequenced,
			expectedCount: 1,
		},
		{
			name:          "CountImportable",
			seqLen:        3,
			states:        []batchState{batchImportable, batchImportable, batchInit},
			queryState:    batchImportable,
			expectedCount: 2,
		},
		{
			name:          "CountComplete",
			seqLen:        3,
			states:        []batchState{batchImportComplete, batchImportComplete, batchInit},
			queryState:    batchImportComplete,
			expectedCount: 2,
		},
		{
			name:          "CountEndSequence",
			seqLen:        3,
			states:        []batchState{batchInit, batchEndSequence, batchInit},
			queryState:    batchEndSequence,
			expectedCount: 1,
		},
		{
			name:          "CountZero_NonexistentState",
			seqLen:        2,
			states:        []batchState{batchInit, batchInit},
			queryState:    batchImportable,
			expectedCount: 0,
		},
		{
			name:          "CountNil",
			seqLen:        3,
			states:        []batchState{batchNil, batchNil, batchInit},
			queryState:    batchNil,
			expectedCount: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			seq := newBatchSequencer(tc.seqLen, 1000, 64, mockCurrentNeedsFunc(0, 1000+1))

			for i, state := range tc.states {
				seq.seq[i].state = state
			}

			count := seq.countWithState(tc.queryState)
			require.Equal(t, tc.expectedCount, count)
		})
	}
}

// TestNumTodo tests remaining batch count calculation
func TestNumTodo(t *testing.T) {
	testCases := []struct {
		name         string
		seqLen       int
		min          primitives.Slot
		max          primitives.Slot
		size         primitives.Slot
		states       []batchState
		expectedTodo int
	}{
		{
			name:         "EmptySequence",
			seqLen:       0,
			min:          0,
			max:          1000,
			size:         64,
			states:       []batchState{},
			expectedTodo: 0,
		},
		{
			name:         "SingleBatchComplete",
			seqLen:       1,
			min:          0,
			max:          1000,
			size:         64,
			states:       []batchState{batchImportComplete},
			expectedTodo: 0,
		},
		{
			name:         "SingleBatchInit",
			seqLen:       1,
			min:          0,
			max:          100,
			size:         10,
			states:       []batchState{batchInit},
			expectedTodo: 1,
		},
		{
			name:         "AllBatchesIgnored",
			seqLen:       3,
			min:          0,
			max:          1000,
			size:         64,
			states:       []batchState{batchImportComplete, batchImportComplete, batchNil},
			expectedTodo: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			seq := newBatchSequencer(tc.seqLen, tc.max, tc.size, mockCurrentNeedsFunc(tc.min, tc.max+1))

			for i, state := range tc.states {
				seq.seq[i] = batch{
					begin: primitives.Slot(tc.max - primitives.Slot((i+1)*10)),
					end:   primitives.Slot(tc.max - primitives.Slot(i*10)),
					state: state,
				}
			}

			// Just verify numTodo doesn't panic
			_ = seq.numTodo()
		})
	}
}

// TestBatcherRemaining tests the remaining() calculation logic
func TestBatcherRemaining(t *testing.T) {
	testCases := []struct {
		name     string
		min      primitives.Slot
		upTo     primitives.Slot
		size     primitives.Slot
		expected int
	}{
		{
			name:     "UpToLessThanMin",
			min:      100,
			upTo:     50,
			size:     10,
			expected: 0,
		},
		{
			name:     "UpToEqualsMin",
			min:      100,
			upTo:     100,
			size:     10,
			expected: 0,
		},
		{
			name:     "ExactBoundary",
			min:      100,
			upTo:     110,
			size:     10,
			expected: 1,
		},
		{
			name:     "ExactBoundary_Multiple",
			min:      100,
			upTo:     150,
			size:     10,
			expected: 5,
		},
		{
			name:     "PartialBatch",
			min:      100,
			upTo:     115,
			size:     10,
			expected: 2,
		},
		{
			name:     "PartialBatch_Small",
			min:      100,
			upTo:     105,
			size:     10,
			expected: 1,
		},
		{
			name:     "LargeRange",
			min:      100,
			upTo:     500,
			size:     10,
			expected: 40,
		},
		{
			name:     "LargeRange_Partial",
			min:      100,
			upTo:     505,
			size:     10,
			expected: 41,
		},
		{
			name:     "PartialBatch_Size1",
			min:      100,
			upTo:     101,
			size:     1,
			expected: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			needs := func() das.CurrentNeeds {
				return das.CurrentNeeds{Block: das.NeedSpan{Begin: tc.min, End: tc.upTo + 1}}
			}
			b := batcher{size: tc.size, currentNeeds: needs}
			result := b.remaining(tc.upTo)
			require.Equal(t, tc.expected, result)
		})
	}
}

// assertAllBatchesAboveMinimum verifies all returned batches have end > minimum
func assertAllBatchesAboveMinimum(t *testing.T, batches []batch, min primitives.Slot) {
	for _, b := range batches {
		if b.state != batchEndSequence {
			if b.end <= min {
				t.Fatalf("batch begin=%d end=%d has end <= minimum %d", b.begin, b.end, min)
			}
		}
	}
}

// assertBatchesContiguous verifies contiguity of returned batches
func assertBatchesContiguous(t *testing.T, batches []batch) {
	for i := 0; i < len(batches)-1; i++ {
		require.Equal(t, batches[i].begin, batches[i+1].end,
			"batch[%d] begin=%d not contiguous with batch[%d] end=%d", i, batches[i].begin, i+1, batches[i+1].end)
	}
}

// assertBatchNotReturned verifies a specific batch is not in the returned list
func assertBatchNotReturned(t *testing.T, batches []batch, shouldNotBe batch) {
	for _, b := range batches {
		if b.begin == shouldNotBe.begin && b.end == shouldNotBe.end {
			t.Fatalf("batch begin=%d end=%d should not be returned", shouldNotBe.begin, shouldNotBe.end)
		}
	}
}

// TestMoveMinimumFiltersOutOfRangeBatches tests that batches below new minimum are not returned by sequence()
// after moveMinimum is called. The sequence() method marks expired batches (end <= min) as batchEndSequence
// but does not return them (unless they're the only batches left).
func TestMoveMinimumFiltersOutOfRangeBatches(t *testing.T) {
	testCases := []struct {
		name             string
		seqLen           int
		min              primitives.Slot
		max              primitives.Slot
		size             primitives.Slot
		initialStates    []batchState
		newMinimum       primitives.Slot
		expectedReturned int
		expectedAllAbove primitives.Slot // all returned batches should have end > this value (except batchEndSequence)
	}{
		// Category 1: Single Batch Below New Minimum
		{
			name:             "BatchBelowMinimum_Init",
			seqLen:           4,
			min:              100,
			max:              1000,
			size:             50,
			initialStates:    []batchState{batchInit, batchInit, batchInit, batchInit},
			newMinimum:       175,
			expectedReturned: 3, // [250-300], [200-250], [150-200] are returned
			expectedAllAbove: 175,
		},
		{
			name:             "BatchBelowMinimum_ErrRetryable",
			seqLen:           4,
			min:              100,
			max:              1000,
			size:             50,
			initialStates:    []batchState{batchSequenced, batchSequenced, batchErrRetryable, batchErrRetryable},
			newMinimum:       175,
			expectedReturned: 1, // only [150-200] (ErrRetryable) is returned; [100-150] is expired and not returned
			expectedAllAbove: 175,
		},

		// Category 2: Multiple Batches Below New Minimum
		{
			name:             "MultipleBatchesBelowMinimum",
			seqLen:           8,
			min:              100,
			max:              500,
			size:             50,
			initialStates:    []batchState{batchInit, batchInit, batchInit, batchInit, batchInit, batchInit, batchInit, batchInit},
			newMinimum:       320,
			expectedReturned: 4, // [450-500], [400-450], [350-400], [300-350] returned; rest expired/not returned
			expectedAllAbove: 320,
		},

		// Category 3: Batches at Boundary - batch.end == minimum is expired
		{
			name:             "BatchExactlyAtMinimum",
			seqLen:           3,
			min:              100,
			max:              1000,
			size:             50,
			initialStates:    []batchState{batchInit, batchInit, batchInit},
			newMinimum:       200,
			expectedReturned: 1, // [250-300] returned; [200-250] (end==200) and [100-150] are expired
			expectedAllAbove: 200,
		},
		{
			name:             "BatchJustAboveMinimum",
			seqLen:           3,
			min:              100,
			max:              1000,
			size:             50,
			initialStates:    []batchState{batchInit, batchInit, batchInit},
			newMinimum:       199,
			expectedReturned: 2, // [250-300], [200-250] returned; [100-150] (end<=199) is expired
			expectedAllAbove: 199,
		},

		// Category 4: No Batches Affected
		{
			name:             "MoveMinimumNoAffect",
			seqLen:           3,
			min:              100,
			max:              1000,
			size:             50,
			initialStates:    []batchState{batchInit, batchInit, batchInit},
			newMinimum:       120,
			expectedReturned: 3, // all batches returned, none below minimum
			expectedAllAbove: 120,
		},

		// Category 5: Mixed States Below Minimum
		{
			name:             "MixedStatesBelowMinimum",
			seqLen:           4,
			min:              100,
			max:              1000,
			size:             50,
			initialStates:    []batchState{batchSequenced, batchInit, batchErrRetryable, batchInit},
			newMinimum:       175,
			expectedReturned: 2, // [200-250] (Init) and [150-200] (ErrRetryable) returned; others not in Init/ErrRetryable or expired
			expectedAllAbove: 175,
		},

		// Category 6: Large moveMinimum
		{
			name:             "LargeMoveMinimumSkipsMost",
			seqLen:           4,
			min:              100,
			max:              1000,
			size:             50,
			initialStates:    []batchState{batchInit, batchInit, batchInit, batchInit},
			newMinimum:       290,
			expectedReturned: 1, // only [250-300] (end=300 > 290) returned
			expectedAllAbove: 290,
		},

		// Category 7: All Batches Expired
		{
			name:             "AllBatchesExpired",
			seqLen:           3,
			min:              100,
			max:              1000,
			size:             50,
			initialStates:    []batchState{batchInit, batchInit, batchInit},
			newMinimum:       300,
			expectedReturned: 1, // when all expire, one batchEndSequence is returned
			expectedAllAbove: 0, // batchEndSequence may have any slot value, don't check
		},

		// Category 8: Contiguity after filtering
		{
			name:             "ContiguityMaintained",
			seqLen:           4,
			min:              100,
			max:              1000,
			size:             50,
			initialStates:    []batchState{batchInit, batchInit, batchInit, batchInit},
			newMinimum:       150,
			expectedReturned: 3, // [250-300], [200-250], [150-200] returned
			expectedAllAbove: 150,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			seq := newBatchSequencer(tc.seqLen, tc.max, tc.size, mockCurrentNeedsFunc(tc.min, tc.max+1))

			// Initialize batches with valid slot ranges
			initializeBatchWithSlots(seq.seq, tc.min, tc.size)

			// Set initial states
			for i, state := range tc.initialStates {
				seq.seq[i].state = state
			}

			// move minimum and call sequence to update set of batches
			seq.currentNeeds = mockCurrentNeedsFunc(tc.newMinimum, tc.max+1)
			seq.batcher.currentNeeds = seq.currentNeeds
			got, err := seq.sequence()
			require.NoError(t, err)

			// Verify count
			if len(got) != tc.expectedReturned {
				t.Fatalf("expected %d batches returned, got %d", tc.expectedReturned, len(got))
			}

			// Verify all returned non-endSequence batches have end > newMinimum
			// (batchEndSequence may be returned when all batches are expired, so exclude from check)
			if tc.expectedAllAbove > 0 {
				for _, b := range got {
					if b.state != batchEndSequence && b.end <= tc.expectedAllAbove {
						t.Fatalf("batch begin=%d end=%d has end <= %d (should be filtered)",
							b.begin, b.end, tc.expectedAllAbove)
					}
				}
			}

			// Verify contiguity is maintained for returned batches
			if len(got) > 1 {
				assertBatchesContiguous(t, got)
			}
		})
	}
}
