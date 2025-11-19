package backfill

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// Helper function to create a columnBatch for testing
func testColumnBatch(custodyGroups peerdas.ColumnIndices, toDownload map[[32]byte]*toDownload) *columnBatch {
	return &columnBatch{
		custodyGroups: custodyGroups,
		toDownload:    toDownload,
	}
}

// Helper function to create test toDownload entries
func testToDownload(remaining peerdas.ColumnIndices, commitments [][]byte) *toDownload {
	return &toDownload{
		remaining:   remaining,
		commitments: commitments,
	}
}

// TestColumnBatchNeeded_EmptyBatch tests that needed() returns empty indices when batch has no blocks
func TestColumnBatchNeeded_EmptyBatch(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	toDownload := make(map[[32]byte]*toDownload)

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	require.Equal(t, 0, result.Count(), "needed() should return empty indices for empty batch")
}

// TestColumnBatchNeeded_NoCustodyGroups tests that needed() returns empty indices when there are no custody groups
func TestColumnBatchNeeded_NoCustodyGroups(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndices()
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	require.Equal(t, 0, result.Count(), "needed() should return empty indices when there are no custody groups")
}

// TestColumnBatchNeeded_AllColumnsStored tests that needed() returns empty when all custody columns are already stored
func TestColumnBatchNeeded_AllColumnsStored(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	// All custody columns are already stored (remaining is empty)
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(peerdas.NewColumnIndices(), nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	require.Equal(t, 0, result.Count(), "needed() should return empty indices when all custody columns are stored")
}

// TestColumnBatchNeeded_NoColumnsStored tests that needed() returns all custody columns when none are stored
func TestColumnBatchNeeded_NoColumnsStored(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	// All custody columns need to be downloaded
	remaining := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(remaining, nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	require.Equal(t, 3, result.Count(), "needed() should return all custody columns when none are stored")
	require.Equal(t, true, result.Has(0), "result should contain column 0")
	require.Equal(t, true, result.Has(1), "result should contain column 1")
	require.Equal(t, true, result.Has(2), "result should contain column 2")
}

// TestColumnBatchNeeded_PartialDownload tests that needed() returns only the remaining columns
func TestColumnBatchNeeded_PartialDownload(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3})
	// Columns 0 and 2 are already stored, 1 and 3 still need downloading
	remaining := peerdas.NewColumnIndicesFromSlice([]uint64{1, 3})
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(remaining, nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	require.Equal(t, 2, result.Count(), "needed() should return only remaining columns")
	require.Equal(t, false, result.Has(0), "result should not contain column 0 (already stored)")
	require.Equal(t, true, result.Has(1), "result should contain column 1")
	require.Equal(t, false, result.Has(2), "result should not contain column 2 (already stored)")
	require.Equal(t, true, result.Has(3), "result should contain column 3")
}

// TestColumnBatchNeeded_NoCommitments tests handling of blocks without blob commitments
func TestColumnBatchNeeded_NoCommitments(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	// Empty toDownload map (no blocks with commitments)
	toDownload := make(map[[32]byte]*toDownload)

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	require.Equal(t, 0, result.Count(), "needed() should return empty indices when no blocks have commitments")
}

// TestColumnBatchNeeded_SingleBlock tests needed() with a single block
func TestColumnBatchNeeded_SingleBlock(t *testing.T) {
	cases := []struct {
		name          string
		custodyGroups []uint64
		remaining     []uint64
		expectedCount int
		expectedCols  []uint64
	}{
		{
			name:          "single block, all columns needed",
			custodyGroups: []uint64{0, 1, 2},
			remaining:     []uint64{0, 1, 2},
			expectedCount: 3,
			expectedCols:  []uint64{0, 1, 2},
		},
		{
			name:          "single block, partial columns needed",
			custodyGroups: []uint64{0, 1, 2, 3},
			remaining:     []uint64{1, 3},
			expectedCount: 2,
			expectedCols:  []uint64{1, 3},
		},
		{
			name:          "single block, no columns needed",
			custodyGroups: []uint64{0, 1, 2},
			remaining:     []uint64{},
			expectedCount: 0,
			expectedCols:  []uint64{},
		},
		{
			name:          "single block, remaining has non-custody columns",
			custodyGroups: []uint64{0, 1},
			remaining:     []uint64{0, 5, 10}, // 5 and 10 are not custody columns
			expectedCount: 1,
			expectedCols:  []uint64{0}, // Only custody column 0 is needed
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice(c.custodyGroups)
			remaining := peerdas.NewColumnIndicesFromSlice(c.remaining)
			toDownload := map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(remaining, nil),
			}

			cb := testColumnBatch(custodyGroups, toDownload)
			result := cb.needed()

			require.Equal(t, c.expectedCount, result.Count(), "unexpected count of needed columns")
			for _, col := range c.expectedCols {
				require.Equal(t, true, result.Has(col), "result should contain column %d", col)
			}
		})
	}
}

// TestColumnBatchNeeded_MultipleBlocks_SameNeeds tests multiple blocks all needing the same columns
func TestColumnBatchNeeded_MultipleBlocks_SameNeeds(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	// All three blocks need the same columns
	remaining := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(remaining.Copy(), nil),
		[32]byte{0x02}: testToDownload(remaining.Copy(), nil),
		[32]byte{0x03}: testToDownload(remaining.Copy(), nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	require.Equal(t, 3, result.Count(), "needed() should return all custody columns")
	require.Equal(t, true, result.Has(0), "result should contain column 0")
	require.Equal(t, true, result.Has(1), "result should contain column 1")
	require.Equal(t, true, result.Has(2), "result should contain column 2")
}

// TestColumnBatchNeeded_MultipleBlocks_DifferentNeeds tests multiple blocks needing different columns
func TestColumnBatchNeeded_MultipleBlocks_DifferentNeeds(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3, 4})
	// Block 1 needs columns 0, 1
	// Block 2 needs columns 2, 3
	// Block 3 needs columns 4
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
		[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{2, 3}), nil),
		[32]byte{0x03}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{4}), nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	require.Equal(t, 5, result.Count(), "needed() should return union of all needed columns")
	require.Equal(t, true, result.Has(0), "result should contain column 0")
	require.Equal(t, true, result.Has(1), "result should contain column 1")
	require.Equal(t, true, result.Has(2), "result should contain column 2")
	require.Equal(t, true, result.Has(3), "result should contain column 3")
	require.Equal(t, true, result.Has(4), "result should contain column 4")
}

// TestColumnBatchNeeded_MixedBlockStates tests blocks in different download states
func TestColumnBatchNeeded_MixedBlockStates(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3})
	// Block 1: all columns complete (empty remaining)
	// Block 2: partially complete (columns 1, 3 remaining)
	// Block 3: nothing downloaded yet (all custody columns remaining)
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(peerdas.NewColumnIndices(), nil),
		[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{1, 3}), nil),
		[32]byte{0x03}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3}), nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	// Should return all custody columns that appear in at least one block's remaining set
	require.Equal(t, 4, result.Count(), "needed() should return all columns that are needed by at least one block")
	require.Equal(t, true, result.Has(0), "result should contain column 0")
	require.Equal(t, true, result.Has(1), "result should contain column 1")
	require.Equal(t, true, result.Has(2), "result should contain column 2")
	require.Equal(t, true, result.Has(3), "result should contain column 3")
}

// TestColumnBatchNeeded_EarlyExitOptimization tests the early exit optimization when all custody columns are found
func TestColumnBatchNeeded_EarlyExitOptimization(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1})
	// Block 1 has both custody columns in remaining
	// Block 2 also has columns in remaining, but they shouldn't affect the result
	// The algorithm should exit early after finding all custody columns in block 1
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
		[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	// Should find both custody columns
	require.Equal(t, 2, result.Count(), "needed() should find all custody columns")
	require.Equal(t, true, result.Has(0), "result should contain column 0")
	require.Equal(t, true, result.Has(1), "result should contain column 1")
}

// TestColumnBatchNeeded_AfterUnset tests that needed() updates correctly after Unset() is called
func TestColumnBatchNeeded_AfterUnset(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	remaining := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(remaining, nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)

	// Initial state: all columns needed
	result := cb.needed()
	require.Equal(t, 3, result.Count(), "initially, all custody columns should be needed")

	// Simulate downloading column 1
	remaining.Unset(1)

	// After Unset: column 1 should no longer be needed
	result = cb.needed()
	require.Equal(t, 2, result.Count(), "after Unset(1), only 2 columns should be needed")
	require.Equal(t, true, result.Has(0), "result should still contain column 0")
	require.Equal(t, false, result.Has(1), "result should not contain column 1 after Unset")
	require.Equal(t, true, result.Has(2), "result should still contain column 2")

	// Simulate downloading all remaining columns
	remaining.Unset(0)
	remaining.Unset(2)

	// After all Unsets: no columns needed
	result = cb.needed()
	require.Equal(t, 0, result.Count(), "after all columns downloaded, none should be needed")
}

// TestColumnBatchNeeded_MultipleBlocks_AfterPartialUnset tests partial completion across multiple blocks
func TestColumnBatchNeeded_MultipleBlocks_AfterPartialUnset(t *testing.T) {
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	remaining1 := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	remaining2 := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(remaining1, nil),
		[32]byte{0x02}: testToDownload(remaining2, nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)

	// Initial state: all columns needed from both blocks
	result := cb.needed()
	require.Equal(t, 3, result.Count(), "initially, all custody columns should be needed")

	// Download column 0 from block 1 only
	remaining1.Unset(0)

	// Column 0 is still needed because block 2 still needs it
	result = cb.needed()
	require.Equal(t, 3, result.Count(), "column 0 still needed by block 2")
	require.Equal(t, true, result.Has(0), "column 0 still in needed set")

	// Download column 0 from block 2 as well
	remaining2.Unset(0)

	// Now column 0 is no longer needed by any block
	result = cb.needed()
	require.Equal(t, 2, result.Count(), "column 0 no longer needed by any block")
	require.Equal(t, false, result.Has(0), "column 0 should not be in needed set")
	require.Equal(t, true, result.Has(1), "column 1 still needed")
	require.Equal(t, true, result.Has(2), "column 2 still needed")
}

// TestColumnBatchNeeded_LargeColumnIndices tests with realistic column indices for PeerDAS
func TestColumnBatchNeeded_LargeColumnIndices(t *testing.T) {
	// Simulate a realistic scenario with larger column indices
	custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{5, 16, 27, 38, 49, 60, 71, 82, 93, 104, 115, 126})
	remaining := peerdas.NewColumnIndicesFromSlice([]uint64{5, 16, 27, 38, 49, 60, 71, 82, 93, 104, 115, 126})
	toDownload := map[[32]byte]*toDownload{
		[32]byte{0x01}: testToDownload(remaining, nil),
	}

	cb := testColumnBatch(custodyGroups, toDownload)
	result := cb.needed()

	require.Equal(t, 12, result.Count(), "should handle larger column indices correctly")
	require.Equal(t, true, result.Has(5), "result should contain column 5")
	require.Equal(t, true, result.Has(126), "result should contain column 126")
}
