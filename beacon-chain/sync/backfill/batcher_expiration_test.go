package backfill

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// dynamicNeeds provides a mutable currentNeeds callback for testing scenarios
// where the retention window changes over time.
type dynamicNeeds struct {
	blockBegin primitives.Slot
	blockEnd   primitives.Slot
	blobBegin  primitives.Slot
	blobEnd    primitives.Slot
	colBegin   primitives.Slot
	colEnd     primitives.Slot
}

func newDynamicNeeds(blockBegin, blockEnd primitives.Slot) *dynamicNeeds {
	return &dynamicNeeds{
		blockBegin: blockBegin,
		blockEnd:   blockEnd,
		blobBegin:  blockBegin,
		blobEnd:    blockEnd,
		colBegin:   blockBegin,
		colEnd:     blockEnd,
	}
}

func (d *dynamicNeeds) get() das.CurrentNeeds {
	return das.CurrentNeeds{
		Block: das.NeedSpan{Begin: d.blockBegin, End: d.blockEnd},
		Blob:  das.NeedSpan{Begin: d.blobBegin, End: d.blobEnd},
		Col:   das.NeedSpan{Begin: d.colBegin, End: d.colEnd},
	}
}

// advance moves the retention window forward by the given number of slots.
func (d *dynamicNeeds) advance(slots primitives.Slot) {
	d.blockBegin += slots
	d.blockEnd += slots
	d.blobBegin += slots
	d.blobEnd += slots
	d.colBegin += slots
	d.colEnd += slots
}

// setBlockBegin sets only the block retention start slot.
func (d *dynamicNeeds) setBlockBegin(begin primitives.Slot) {
	d.blockBegin = begin
}

// ============================================================================
// Category 1: Basic Expiration During sequence()
// ============================================================================

func TestSequenceExpiration_SingleBatchExpires_Init(t *testing.T) {
	// Single batch in batchInit expires when needs.block.begin moves past it
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(1, 200, 50, dn.get)

	// Initialize batch: [150, 200)
	seq.seq[0] = batch{begin: 150, end: 200, state: batchInit}

	// Move retention window past the batch
	dn.setBlockBegin(200)
	seq.batcher.currentNeeds = dn.get

	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, batchEndSequence, got[0].state)
}

func TestSequenceExpiration_SingleBatchExpires_ErrRetryable(t *testing.T) {
	// Single batch in batchErrRetryable expires when needs change
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(1, 200, 50, dn.get)

	seq.seq[0] = batch{begin: 150, end: 200, state: batchErrRetryable}

	// Move retention window past the batch
	dn.setBlockBegin(200)
	seq.batcher.currentNeeds = dn.get

	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, batchEndSequence, got[0].state)
}

func TestSequenceExpiration_MultipleBatchesExpire_Partial(t *testing.T) {
	// 4 batches, 2 expire when needs change
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(4, 400, 50, dn.get)

	// Batches: [350,400), [300,350), [250,300), [200,250)
	seq.seq[0] = batch{begin: 350, end: 400, state: batchInit}
	seq.seq[1] = batch{begin: 300, end: 350, state: batchInit}
	seq.seq[2] = batch{begin: 250, end: 300, state: batchInit}
	seq.seq[3] = batch{begin: 200, end: 250, state: batchInit}

	// Move retention to 300 - batches [250,300) and [200,250) should expire
	dn.setBlockBegin(300)
	seq.batcher.currentNeeds = dn.get

	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 2, len(got))

	// First two batches should be sequenced (not expired)
	require.Equal(t, batchSequenced, got[0].state)
	require.Equal(t, primitives.Slot(350), got[0].begin)
	require.Equal(t, batchSequenced, got[1].state)
	require.Equal(t, primitives.Slot(300), got[1].begin)

	// Verify expired batches are marked batchEndSequence in seq
	require.Equal(t, batchEndSequence, seq.seq[2].state)
	require.Equal(t, batchEndSequence, seq.seq[3].state)
}

func TestSequenceExpiration_AllBatchesExpire(t *testing.T) {
	// All batches expire, returns one batchEndSequence
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(3, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchInit}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchInit}
	seq.seq[2] = batch{begin: 150, end: 200, state: batchInit}

	// Move retention past all batches
	dn.setBlockBegin(350)
	seq.batcher.currentNeeds = dn.get

	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, batchEndSequence, got[0].state)
}

func TestSequenceExpiration_BatchAtExactBoundary(t *testing.T) {
	// Batch with end == needs.block.begin should expire
	// Because expired() checks !needs.block.at(b.end - 1)
	// If batch.end = 200 and needs.block.begin = 200, then at(199) = false → expired
	dn := newDynamicNeeds(200, 500)
	seq := newBatchSequencer(1, 250, 50, dn.get)

	// Batch [150, 200) - end is exactly at retention start
	seq.seq[0] = batch{begin: 150, end: 200, state: batchInit}

	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, batchEndSequence, got[0].state)
}

func TestSequenceExpiration_BatchJustInsideBoundary(t *testing.T) {
	// Batch with end == needs.block.begin + 1 should NOT expire
	// at(200) with begin=200 returns true
	dn := newDynamicNeeds(200, 500)
	seq := newBatchSequencer(1, 251, 50, dn.get)

	// Batch [200, 251) - end-1 = 250 which is inside [200, 500)
	seq.seq[0] = batch{begin: 200, end: 251, state: batchInit}

	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, batchSequenced, got[0].state)
}

// ============================================================================
// Category 2: Expiration During update()
// ============================================================================

func TestUpdateExpiration_UpdateCausesExpiration(t *testing.T) {
	// Update a batch while needs have changed, causing other batches to expire
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(3, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchSequenced}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchSequenced}
	seq.seq[2] = batch{begin: 150, end: 200, state: batchInit}

	// Move retention window
	dn.setBlockBegin(200)
	seq.batcher.currentNeeds = dn.get

	// Update first batch (should still be valid)
	updated := batch{begin: 250, end: 300, state: batchImportable, seq: 1}
	seq.update(updated)

	// First batch should be updated
	require.Equal(t, batchImportable, seq.seq[0].state)

	// Third batch should have expired during update
	require.Equal(t, batchEndSequence, seq.seq[2].state)
}

func TestUpdateExpiration_MultipleExpireDuringUpdate(t *testing.T) {
	// Several batches expire when needs advance significantly
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(4, 400, 50, dn.get)

	seq.seq[0] = batch{begin: 350, end: 400, state: batchSequenced}
	seq.seq[1] = batch{begin: 300, end: 350, state: batchSequenced}
	seq.seq[2] = batch{begin: 250, end: 300, state: batchInit}
	seq.seq[3] = batch{begin: 200, end: 250, state: batchInit}

	// Move retention to expire last two batches
	dn.setBlockBegin(300)
	seq.batcher.currentNeeds = dn.get

	// Update first batch
	updated := batch{begin: 350, end: 400, state: batchImportable, seq: 1}
	seq.update(updated)

	// Check that expired batches are marked
	require.Equal(t, batchEndSequence, seq.seq[2].state)
	require.Equal(t, batchEndSequence, seq.seq[3].state)
}

func TestUpdateExpiration_UpdateCompleteWhileExpiring(t *testing.T) {
	// Mark batch complete while other batches expire
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(3, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchImportable}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchSequenced}
	seq.seq[2] = batch{begin: 150, end: 200, state: batchInit}

	// Move retention to expire last batch
	dn.setBlockBegin(200)
	seq.batcher.currentNeeds = dn.get

	// Mark first batch complete
	completed := batch{begin: 250, end: 300, state: batchImportComplete, seq: 1}
	seq.update(completed)

	// Completed batch removed, third batch should have expired
	// Check that we still have 3 batches (shifted + new ones added)
	require.Equal(t, 3, len(seq.seq))

	// The batch that was at index 2 should now be expired
	foundExpired := false
	for _, b := range seq.seq {
		if b.state == batchEndSequence {
			foundExpired = true
			break
		}
	}
	require.Equal(t, true, foundExpired, "should have an expired batch")
}

func TestUpdateExpiration_ExpiredBatchNotShiftedIncorrectly(t *testing.T) {
	// Verify expired batches don't get incorrectly shifted
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(3, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchImportComplete}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchInit}
	seq.seq[2] = batch{begin: 150, end: 200, state: batchInit}

	// Move retention to expire all remaining init batches
	dn.setBlockBegin(250)
	seq.batcher.currentNeeds = dn.get

	// Update with the completed batch
	completed := batch{begin: 250, end: 300, state: batchImportComplete, seq: 1}
	seq.update(completed)

	// Verify sequence integrity
	require.Equal(t, 3, len(seq.seq))
}

func TestUpdateExpiration_NewBatchCreatedRespectsNeeds(t *testing.T) {
	// When new batch is created after expiration, it should respect current needs
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(2, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchImportable}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchInit}

	// Mark first batch complete to trigger new batch creation
	completed := batch{begin: 250, end: 300, state: batchImportComplete, seq: 1}
	seq.update(completed)

	// New batch should be created - verify it respects the needs
	require.Equal(t, 2, len(seq.seq))
	// New batch should have proper bounds
	for _, b := range seq.seq {
		if b.state == batchNil {
			continue
		}
		require.Equal(t, true, b.begin < b.end, "batch bounds should be valid")
	}
}

// ============================================================================
// Category 3: Progressive Slot Advancement
// ============================================================================

func TestProgressiveAdvancement_SlotAdvancesGradually(t *testing.T) {
	// Simulate gradual slot advancement with batches expiring one by one
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(4, 400, 50, dn.get)

	// Initialize batches
	seq.seq[0] = batch{begin: 350, end: 400, state: batchInit}
	seq.seq[1] = batch{begin: 300, end: 350, state: batchInit}
	seq.seq[2] = batch{begin: 250, end: 300, state: batchInit}
	seq.seq[3] = batch{begin: 200, end: 250, state: batchInit}

	// First sequence - all should be returned
	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 4, len(got))

	// Advance by 50 slots - last batch should expire
	dn.setBlockBegin(250)
	seq.batcher.currentNeeds = dn.get

	// Mark first batch importable and update
	seq.seq[0].state = batchImportable
	seq.update(seq.seq[0])

	// Last batch should now be expired
	require.Equal(t, batchEndSequence, seq.seq[3].state)

	// Advance again
	dn.setBlockBegin(300)
	seq.batcher.currentNeeds = dn.get

	seq.seq[1].state = batchImportable
	seq.update(seq.seq[1])

	// Count expired batches
	expiredCount := 0
	for _, b := range seq.seq {
		if b.state == batchEndSequence {
			expiredCount++
		}
	}
	require.Equal(t, true, expiredCount >= 2, "expected at least 2 expired batches")
}

func TestProgressiveAdvancement_SlotAdvancesInBursts(t *testing.T) {
	// Large jump in slots causes multiple batches to expire at once
	dn := newDynamicNeeds(100, 600)
	seq := newBatchSequencer(6, 500, 50, dn.get)

	// Initialize batches: [450,500), [400,450), [350,400), [300,350), [250,300), [200,250)
	for i := range 6 {
		seq.seq[i] = batch{
			begin: primitives.Slot(500 - (i+1)*50),
			end:   primitives.Slot(500 - i*50),
			state: batchInit,
		}
	}

	// Large jump - expire 4 batches at once
	dn.setBlockBegin(400)
	seq.batcher.currentNeeds = dn.get

	got, err := seq.sequence()
	require.NoError(t, err)

	// Should have 2 non-expired batches returned
	nonExpired := 0
	for _, b := range got {
		if b.state == batchSequenced {
			nonExpired++
		}
	}
	require.Equal(t, 2, nonExpired)
}

func TestProgressiveAdvancement_WorkerProcessingDuringAdvancement(t *testing.T) {
	// Batches in various processing states while needs advance
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(4, 400, 50, dn.get)

	seq.seq[0] = batch{begin: 350, end: 400, state: batchSyncBlobs}
	seq.seq[1] = batch{begin: 300, end: 350, state: batchSyncColumns}
	seq.seq[2] = batch{begin: 250, end: 300, state: batchSequenced}
	seq.seq[3] = batch{begin: 200, end: 250, state: batchInit}

	// Advance past last batch
	dn.setBlockBegin(250)
	seq.batcher.currentNeeds = dn.get

	// Call sequence - only batchInit should transition
	got, err := seq.sequence()
	require.NoError(t, err)

	// batchInit batch should have expired
	require.Equal(t, batchEndSequence, seq.seq[3].state)

	// Batches in other states should not be returned by sequence (already dispatched)
	for _, b := range got {
		require.NotEqual(t, batchSyncBlobs, b.state)
		require.NotEqual(t, batchSyncColumns, b.state)
	}
}

func TestProgressiveAdvancement_CompleteBeforeExpiration(t *testing.T) {
	// Batch completes just before it would expire
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(2, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchSequenced}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchImportable}

	// Complete the second batch BEFORE advancing needs
	completed := batch{begin: 200, end: 250, state: batchImportComplete, seq: 1}
	seq.update(completed)

	// Now advance needs past where the batch was
	dn.setBlockBegin(250)
	seq.batcher.currentNeeds = dn.get

	// The completed batch should have been removed successfully
	// Sequence should work normally
	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, true, len(got) >= 1, "expected at least 1 batch")
}

// ============================================================================
// Category 4: Batch State Transitions Under Expiration
// ============================================================================

func TestStateExpiration_NilBatchNotExpired(t *testing.T) {
	// batchNil should be initialized, not expired
	dn := newDynamicNeeds(200, 500)
	seq := newBatchSequencer(2, 300, 50, dn.get)

	// Leave seq[0] as batchNil (zero value)
	seq.seq[1] = batch{begin: 200, end: 250, state: batchInit}

	got, err := seq.sequence()
	require.NoError(t, err)

	// batchNil should have been initialized and sequenced
	foundSequenced := false
	for _, b := range got {
		if b.state == batchSequenced {
			foundSequenced = true
		}
	}
	require.Equal(t, true, foundSequenced, "expected at least one sequenced batch")
}

func TestStateExpiration_InitBatchExpires(t *testing.T) {
	// batchInit batches expire when outside retention
	dn := newDynamicNeeds(200, 500)
	seq := newBatchSequencer(1, 250, 50, dn.get)

	seq.seq[0] = batch{begin: 150, end: 200, state: batchInit}

	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, batchEndSequence, got[0].state)
}

func TestStateExpiration_SequencedBatchNotCheckedBySequence(t *testing.T) {
	// batchSequenced batches are not returned by sequence() (already dispatched)
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(2, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchSequenced}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchInit}

	// Move retention past second batch
	dn.setBlockBegin(250)
	seq.batcher.currentNeeds = dn.get

	got, err := seq.sequence()
	require.NoError(t, err)

	// Init batch should expire, sequenced batch not returned
	for _, b := range got {
		require.NotEqual(t, batchSequenced, b.state)
	}
}

func TestStateExpiration_SyncBlobsBatchNotCheckedBySequence(t *testing.T) {
	// batchSyncBlobs not returned by sequence
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(1, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchSyncBlobs}

	_, err := seq.sequence()
	require.ErrorIs(t, err, errMaxBatches) // No batch to return
}

func TestStateExpiration_SyncColumnsBatchNotCheckedBySequence(t *testing.T) {
	// batchSyncColumns not returned by sequence
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(1, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchSyncColumns}

	_, err := seq.sequence()
	require.ErrorIs(t, err, errMaxBatches)
}

func TestStateExpiration_ImportableBatchNotCheckedBySequence(t *testing.T) {
	// batchImportable not returned by sequence
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(1, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchImportable}

	_, err := seq.sequence()
	require.ErrorIs(t, err, errMaxBatches)
}

func TestStateExpiration_RetryableBatchExpires(t *testing.T) {
	// batchErrRetryable batches can expire
	dn := newDynamicNeeds(200, 500)
	seq := newBatchSequencer(1, 250, 50, dn.get)

	seq.seq[0] = batch{begin: 150, end: 200, state: batchErrRetryable}

	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, batchEndSequence, got[0].state)
}

// ============================================================================
// Category 5: Edge Cases and Boundaries
// ============================================================================

func TestEdgeCase_NeedsSpanShrinks(t *testing.T) {
	// Unusual case: retention window becomes smaller
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(3, 400, 50, dn.get)

	seq.seq[0] = batch{begin: 350, end: 400, state: batchInit}
	seq.seq[1] = batch{begin: 300, end: 350, state: batchInit}
	seq.seq[2] = batch{begin: 250, end: 300, state: batchInit}

	// Shrink window from both ends
	dn.blockBegin = 300
	dn.blockEnd = 400
	seq.batcher.currentNeeds = dn.get

	_, err := seq.sequence()
	require.NoError(t, err)

	// Third batch should have expired
	require.Equal(t, batchEndSequence, seq.seq[2].state)
}

func TestEdgeCase_EmptySequenceAfterExpiration(t *testing.T) {
	// All batches in non-schedulable states, none can be sequenced
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(2, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchImportable}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchImportable}

	// No batchInit or batchErrRetryable to sequence
	_, err := seq.sequence()
	require.ErrorIs(t, err, errMaxBatches)
}

func TestEdgeCase_EndSequenceChainReaction(t *testing.T) {
	// When batches expire, subsequent calls should handle them correctly
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(3, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchInit}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchInit}
	seq.seq[2] = batch{begin: 150, end: 200, state: batchInit}

	// Expire all
	dn.setBlockBegin(300)
	seq.batcher.currentNeeds = dn.get

	got1, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(got1))
	require.Equal(t, batchEndSequence, got1[0].state)

	// Calling sequence again should still return batchEndSequence
	got2, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 1, len(got2))
	require.Equal(t, batchEndSequence, got2[0].state)
}

func TestEdgeCase_MixedExpirationAndCompletion(t *testing.T) {
	// Some batches complete while others expire simultaneously
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(4, 400, 50, dn.get)

	seq.seq[0] = batch{begin: 350, end: 400, state: batchImportComplete}
	seq.seq[1] = batch{begin: 300, end: 350, state: batchImportable}
	seq.seq[2] = batch{begin: 250, end: 300, state: batchInit}
	seq.seq[3] = batch{begin: 200, end: 250, state: batchInit}

	// Expire last two batches
	dn.setBlockBegin(300)
	seq.batcher.currentNeeds = dn.get

	// Update with completed batch to trigger processing
	completed := batch{begin: 350, end: 400, state: batchImportComplete, seq: 1}
	seq.update(completed)

	// Verify expired batches are marked
	expiredCount := 0
	for _, b := range seq.seq {
		if b.state == batchEndSequence {
			expiredCount++
		}
	}
	require.Equal(t, true, expiredCount >= 2, "expected at least 2 expired batches")
}

func TestEdgeCase_BatchExpiresAtSlotZero(t *testing.T) {
	// Edge case with very low slot numbers
	dn := newDynamicNeeds(50, 200)
	seq := newBatchSequencer(2, 100, 50, dn.get)

	seq.seq[0] = batch{begin: 50, end: 100, state: batchInit}
	seq.seq[1] = batch{begin: 0, end: 50, state: batchInit}

	// Move past first batch
	dn.setBlockBegin(100)
	seq.batcher.currentNeeds = dn.get

	got, err := seq.sequence()
	require.NoError(t, err)

	// Both batches should have expired
	for _, b := range got {
		require.Equal(t, batchEndSequence, b.state)
	}
}

// ============================================================================
// Category 6: Integration with numTodo/remaining
// ============================================================================

func TestNumTodo_AfterExpiration(t *testing.T) {
	// numTodo should correctly reflect expired batches
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(3, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchSequenced}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchSequenced}
	seq.seq[2] = batch{begin: 150, end: 200, state: batchInit}

	todoBefore := seq.numTodo()

	// Expire last batch
	dn.setBlockBegin(200)
	seq.batcher.currentNeeds = dn.get

	// Force expiration via sequence
	_, err := seq.sequence()
	require.NoError(t, err)

	todoAfter := seq.numTodo()

	// Todo count should have decreased
	require.Equal(t, true, todoAfter < todoBefore, "expected todo count to decrease after expiration")
}

func TestRemaining_AfterNeedsChange(t *testing.T) {
	// batcher.remaining() should use updated needs
	dn := newDynamicNeeds(100, 500)
	b := batcher{currentNeeds: dn.get, size: 50}

	remainingBefore := b.remaining(300)

	// Move retention window
	dn.setBlockBegin(250)
	b.currentNeeds = dn.get

	remainingAfter := b.remaining(300)

	// Remaining should have decreased
	require.Equal(t, true, remainingAfter < remainingBefore, "expected remaining to decrease after needs change")
}

func TestCountWithState_AfterExpiration(t *testing.T) {
	// State counts should be accurate after expiration
	dn := newDynamicNeeds(100, 500)
	seq := newBatchSequencer(3, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchInit}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchInit}
	seq.seq[2] = batch{begin: 150, end: 200, state: batchInit}

	require.Equal(t, 3, seq.countWithState(batchInit))
	require.Equal(t, 0, seq.countWithState(batchEndSequence))

	// Expire all batches
	dn.setBlockBegin(300)
	seq.batcher.currentNeeds = dn.get

	_, err := seq.sequence()
	require.NoError(t, err)

	require.Equal(t, 0, seq.countWithState(batchInit))
	require.Equal(t, 3, seq.countWithState(batchEndSequence))
}

// ============================================================================
// Category 7: Fork Transition Scenarios (Blob/Column specific)
// ============================================================================

func TestForkTransition_BlobNeedsChange(t *testing.T) {
	// Test when blob retention is different from block retention
	dn := newDynamicNeeds(100, 500)
	// Set blob begin to be further ahead
	dn.blobBegin = 200

	seq := newBatchSequencer(3, 300, 50, dn.get)

	seq.seq[0] = batch{begin: 250, end: 300, state: batchInit}
	seq.seq[1] = batch{begin: 200, end: 250, state: batchInit}
	seq.seq[2] = batch{begin: 150, end: 200, state: batchInit}

	// Sequence should work based on block needs
	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 3, len(got))
}

func TestForkTransition_ColumnNeedsChange(t *testing.T) {
	// Test when column retention is different from block retention
	dn := newDynamicNeeds(100, 500)
	// Set column begin to be further ahead
	dn.colBegin = 300

	seq := newBatchSequencer(3, 400, 50, dn.get)

	seq.seq[0] = batch{begin: 350, end: 400, state: batchInit}
	seq.seq[1] = batch{begin: 300, end: 350, state: batchInit}
	seq.seq[2] = batch{begin: 250, end: 300, state: batchInit}

	// Batch expiration is based on block needs, not column needs
	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 3, len(got))
}

func TestForkTransition_BlockNeedsVsBlobNeeds(t *testing.T) {
	// Blocks still needed but blobs have shorter retention
	dn := newDynamicNeeds(100, 500)
	dn.blobBegin = 300 // Blobs only needed from slot 300
	dn.blobEnd = 500

	seq := newBatchSequencer(3, 400, 50, dn.get)

	seq.seq[0] = batch{begin: 350, end: 400, state: batchInit}
	seq.seq[1] = batch{begin: 300, end: 350, state: batchInit}
	seq.seq[2] = batch{begin: 250, end: 300, state: batchInit}

	// All batches should be returned (block expiration, not blob)
	got, err := seq.sequence()
	require.NoError(t, err)
	require.Equal(t, 3, len(got))

	// Now change block needs to match blob needs
	dn.blockBegin = 300
	seq.batcher.currentNeeds = dn.get

	// Re-sequence - last batch should expire
	seq.seq[0].state = batchInit
	seq.seq[1].state = batchInit
	seq.seq[2].state = batchInit

	got2, err := seq.sequence()
	require.NoError(t, err)

	// Should have 2 non-expired batches
	nonExpired := 0
	for _, b := range got2 {
		if b.state == batchSequenced {
			nonExpired++
		}
	}
	require.Equal(t, 2, nonExpired)
}

func TestForkTransition_AllResourceTypesAdvance(t *testing.T) {
	// Block, blob, and column spans all advance together
	dn := newDynamicNeeds(100, 500)

	seq := newBatchSequencer(4, 400, 50, dn.get)

	// Batches: [350,400), [300,350), [250,300), [200,250)
	for i := range 4 {
		seq.seq[i] = batch{
			begin: primitives.Slot(400 - (i+1)*50),
			end:   primitives.Slot(400 - i*50),
			state: batchInit,
		}
	}

	// Advance all needs together by 200 slots
	// blockBegin moves from 100 to 300
	dn.advance(200)
	seq.batcher.currentNeeds = dn.get

	got, err := seq.sequence()
	require.NoError(t, err)

	// Count non-expired
	nonExpired := 0
	for _, b := range got {
		if b.state == batchSequenced {
			nonExpired++
		}
	}

	// With begin=300, batches [200,250) and [250,300) should have expired
	// Batches [350,400) and [300,350) remain valid
	require.Equal(t, 2, nonExpired)
}
