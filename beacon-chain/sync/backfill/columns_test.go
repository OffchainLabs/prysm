package backfill

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/libp2p/go-libp2p/core/peer"
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

// TestBuildColumnBatch tests the buildColumnBatch function
func TestBuildColumnBatch(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	// Setup Fulu fork epoch if not already set
	denebEpoch := params.BeaconConfig().DenebForkEpoch
	if params.BeaconConfig().FuluForkEpoch == params.BeaconConfig().FarFutureEpoch {
		params.BeaconConfig().FuluForkEpoch = denebEpoch + 4096*2
	}
	fuluEpoch := params.BeaconConfig().FuluForkEpoch

	fuluSlot, err := slots.EpochStart(fuluEpoch)
	require.NoError(t, err)
	denebSlot, err := slots.EpochStart(denebEpoch)
	require.NoError(t, err)

	t.Run("empty blocks returns nil", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		cb, err := buildColumnBatch(ctx, batch{}, verifiedROBlocks{}, p, store)
		require.NoError(t, err)
		require.Equal(t, true, cb == nil)
	})

	t.Run("pre-Fulu batch end returns nil", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		// Create blocks in Deneb
		blks, _ := testBlobGen(t, denebSlot, 2)
		b := batch{
			begin: denebSlot,
			end:   denebSlot + 10,
		}

		cb, err := buildColumnBatch(ctx, b, blks, p, store)
		require.NoError(t, err)
		require.Equal(t, true, cb == nil)
	})

	t.Run("pre-Fulu last block returns nil", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		// Create blocks before Fulu but batch end after
		blks, _ := testBlobGen(t, denebSlot, 2)
		b := batch{
			begin: denebSlot,
			end:   fuluSlot + 10,
		}

		cb, err := buildColumnBatch(ctx, b, blks, p, store)
		require.NoError(t, err)
		require.Equal(t, true, cb == nil)
	})

	t.Run("boundary: batch end exactly at Fulu epoch", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		// Create blocks at Fulu start
		blks, _ := testBlobGen(t, fuluSlot, 2)
		b := batch{
			begin: fuluSlot,
			end:   fuluSlot,
		}

		cb, err := buildColumnBatch(ctx, b, blks, p, store)
		require.NoError(t, err)
		require.NotNil(t, cb, "batch at Fulu boundary should not be nil")
	})

	t.Run("boundary: last block exactly at Fulu epoch", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		// Create blocks at Fulu start
		blks, _ := testBlobGen(t, fuluSlot, 1)
		b := batch{
			begin: fuluSlot,
			end:   fuluSlot + 100,
		}

		cb, err := buildColumnBatch(ctx, b, blks, p, store)
		require.NoError(t, err)
		require.NotNil(t, cb, "last block at Fulu boundary should not be nil")
	})

	t.Run("mixed epochs: first block pre-Fulu, last block post-Fulu", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		// Create blocks spanning the fork: 2 before, 2 after
		preFuluCount := 2
		postFuluCount := 2
		startSlot := fuluSlot - primitives.Slot(preFuluCount)

		allBlocks := make([]blocks.ROBlock, 0, preFuluCount+postFuluCount)
		preBlocks, _ := testBlobGen(t, startSlot, preFuluCount)
		postBlocks, _ := testBlobGen(t, fuluSlot, postFuluCount)
		allBlocks = append(allBlocks, preBlocks...)
		allBlocks = append(allBlocks, postBlocks...)

		b := batch{
			begin: startSlot,
			end:   fuluSlot + primitives.Slot(postFuluCount),
		}

		cb, err := buildColumnBatch(ctx, b, allBlocks, p, store)
		require.NoError(t, err)
		require.NotNil(t, cb, "mixed epoch batch should not be nil")
		// Should only include Fulu blocks
		require.Equal(t, postFuluCount, len(cb.toDownload), "should only include Fulu blocks")
	})

	t.Run("boundary: first block exactly at Fulu epoch", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		// Create blocks starting exactly at Fulu
		blks, _ := testBlobGen(t, fuluSlot, 3)
		b := batch{
			begin: fuluSlot,
			end:   fuluSlot + 100,
		}

		cb, err := buildColumnBatch(ctx, b, blks, p, store)
		require.NoError(t, err)
		require.NotNil(t, cb, "first block at Fulu should not be nil")
		require.Equal(t, 3, len(cb.toDownload), "should include all 3 blocks")
	})

	t.Run("single Fulu block with commitments", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		blks, _ := testBlobGen(t, fuluSlot, 1)
		b := batch{
			begin: fuluSlot,
			end:   fuluSlot + 10,
		}

		cb, err := buildColumnBatch(ctx, b, blks, p, store)
		require.NoError(t, err)
		require.NotNil(t, cb)
		require.Equal(t, fuluSlot, cb.first, "first slot should be set")
		require.Equal(t, fuluSlot, cb.last, "last slot should equal first for single block")
		require.Equal(t, 1, len(cb.toDownload))
	})

	t.Run("multiple blocks: first and last assignment", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		blks, _ := testBlobGen(t, fuluSlot, 5)
		b := batch{
			begin: fuluSlot,
			end:   fuluSlot + 10,
		}

		cb, err := buildColumnBatch(ctx, b, blks, p, store)
		require.NoError(t, err)
		require.NotNil(t, cb)
		require.Equal(t, fuluSlot, cb.first, "first should be slot of first block with commitments")
		require.Equal(t, fuluSlot+4, cb.last, "last should be slot of last block with commitments")
	})

	t.Run("blocks without commitments are skipped", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		// Create blocks with commitments
		blksWithCmts, _ := testBlobGen(t, fuluSlot, 2)

		// Create a block without commitments (manually)
		blkNoCmt, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, fuluSlot+2, 0)

		// Mix them together
		allBlocks := []blocks.ROBlock{
			blksWithCmts[0],
			blkNoCmt, // no commitments - should be skipped via continue
			blksWithCmts[1],
		}

		b := batch{
			begin: fuluSlot,
			end:   fuluSlot + 10,
		}

		cb, err := buildColumnBatch(ctx, b, allBlocks, p, store)
		require.NoError(t, err)
		require.NotNil(t, cb)
		// Should only have 2 blocks (those with commitments)
		require.Equal(t, 2, len(cb.toDownload), "should skip blocks without commitments")
	})
}

// TestColumnSync_BlockColumns tests the blockColumns method
func TestColumnSync_BlockColumns(t *testing.T) {
	t.Run("nil columnBatch returns nil", func(t *testing.T) {
		cs := &columnSync{
			columnBatch: nil,
		}
		result := cs.blockColumns([32]byte{0x01})
		require.Equal(t, true, result == nil)
	})

	t.Run("existing block root returns toDownload", func(t *testing.T) {
		root := [32]byte{0x01}
		expected := &toDownload{
			remaining:   peerdas.NewColumnIndicesFromSlice([]uint64{1, 2, 3}),
			commitments: [][]byte{{0xaa}, {0xbb}},
		}
		cs := &columnSync{
			columnBatch: &columnBatch{
				toDownload: map[[32]byte]*toDownload{
					root: expected,
				},
			},
		}
		result := cs.blockColumns(root)
		require.Equal(t, expected, result)
	})

	t.Run("non-existing block root returns nil", func(t *testing.T) {
		cs := &columnSync{
			columnBatch: &columnBatch{
				toDownload: map[[32]byte]*toDownload{
					[32]byte{0x01}: {
						remaining: peerdas.NewColumnIndicesFromSlice([]uint64{1}),
					},
				},
			},
		}
		result := cs.blockColumns([32]byte{0x99})
		require.Equal(t, true, result == nil)
	})
}

// TestColumnSync_ColumnsNeeded tests the columnsNeeded method
func TestColumnSync_ColumnsNeeded(t *testing.T) {
	t.Run("nil columnBatch returns empty indices", func(t *testing.T) {
		cs := &columnSync{
			columnBatch: nil,
		}
		result := cs.columnsNeeded()
		require.Equal(t, 0, result.Count())
	})

	t.Run("delegates to needed() when columnBatch exists", func(t *testing.T) {
		custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
		remaining := peerdas.NewColumnIndicesFromSlice([]uint64{1, 2})
		cs := &columnSync{
			columnBatch: &columnBatch{
				custodyGroups: custodyGroups,
				toDownload: map[[32]byte]*toDownload{
					[32]byte{0x01}: {
						remaining: remaining,
					},
				},
			},
		}
		result := cs.columnsNeeded()
		require.Equal(t, 2, result.Count())
		require.Equal(t, true, result.Has(1))
		require.Equal(t, true, result.Has(2))
	})
}

// TestValidatingColumnRequest_CountedValidation tests the countedValidation method
func TestValidatingColumnRequest_CountedValidation(t *testing.T) {
	mockPeer := peer.ID("test-peer")

	t.Run("unexpected block root returns error", func(t *testing.T) {
		// Create a data column with a specific block root
		params := []util.DataColumnParam{
			{
				Index:          0,
				Slot:           100,
				ProposerIndex:  1,
				KzgCommitments: [][]byte{{0xaa}},
			},
		}
		roCols, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

		vcr := &validatingColumnRequest{
			columnSync: &columnSync{
				columnBatch: &columnBatch{
					toDownload: map[[32]byte]*toDownload{
						// Different root from what the column has
						[32]byte{0x99}: {
							remaining: peerdas.NewColumnIndicesFromSlice([]uint64{0}),
						},
					},
				},
				peer: mockPeer,
			},
			bisector: newColumnBisector(func(peer.ID, string, error) {}),
		}

		err := vcr.countedValidation(roCols[0])
		require.ErrorIs(t, err, errUnexpectedBlockRoot)
	})

	t.Run("column not in remaining set returns nil (skipped)", func(t *testing.T) {
		blockRoot := [32]byte{0x01}
		params := []util.DataColumnParam{
			{
				Index:          5, // Not in remaining set
				Slot:           100,
				ProposerIndex:  1,
				ParentRoot:     blockRoot[:],
				KzgCommitments: [][]byte{{0xaa}},
			},
		}
		roCols, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

		remaining := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}) // 5 not included
		vcr := &validatingColumnRequest{
			columnSync: &columnSync{
				columnBatch: &columnBatch{
					toDownload: map[[32]byte]*toDownload{
						roCols[0].BlockRoot(): {
							remaining:   remaining,
							commitments: [][]byte{{0xaa}},
						},
					},
				},
				peer: mockPeer,
			},
			bisector: newColumnBisector(func(peer.ID, string, error) {}),
		}

		err := vcr.countedValidation(roCols[0])
		require.NoError(t, err, "should return nil when column not needed")
		// Verify remaining was not modified
		require.Equal(t, 3, remaining.Count())
	})

	t.Run("commitment length mismatch returns error", func(t *testing.T) {
		blockRoot := [32]byte{0x01}
		params := []util.DataColumnParam{
			{
				Index:          0,
				Slot:           100,
				ProposerIndex:  1,
				ParentRoot:     blockRoot[:],
				KzgCommitments: [][]byte{{0xaa}, {0xbb}}, // 2 commitments
			},
		}
		roCols, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

		vcr := &validatingColumnRequest{
			columnSync: &columnSync{
				columnBatch: &columnBatch{
					toDownload: map[[32]byte]*toDownload{
						roCols[0].BlockRoot(): {
							remaining:   peerdas.NewColumnIndicesFromSlice([]uint64{0}),
							commitments: [][]byte{{0xaa}}, // Only 1 commitment - mismatch!
						},
					},
				},
				peer: mockPeer,
			},
			bisector: newColumnBisector(func(peer.ID, string, error) {}),
		}

		err := vcr.countedValidation(roCols[0])
		require.ErrorIs(t, err, errCommitmentLengthMismatch)
	})

	t.Run("commitment value mismatch returns error", func(t *testing.T) {
		blockRoot := [32]byte{0x01}
		params := []util.DataColumnParam{
			{
				Index:          0,
				Slot:           100,
				ProposerIndex:  1,
				ParentRoot:     blockRoot[:],
				KzgCommitments: [][]byte{{0xaa}, {0xbb}},
			},
		}
		roCols, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

		vcr := &validatingColumnRequest{
			columnSync: &columnSync{
				columnBatch: &columnBatch{
					toDownload: map[[32]byte]*toDownload{
						roCols[0].BlockRoot(): {
							remaining: peerdas.NewColumnIndicesFromSlice([]uint64{0}),
							// Different commitment values
							commitments: [][]byte{{0xaa}, {0xcc}},
						},
					},
				},
				peer: mockPeer,
			},
			bisector: newColumnBisector(func(peer.ID, string, error) {}),
		}

		err := vcr.countedValidation(roCols[0])
		require.ErrorIs(t, err, errCommitmentValueMismatch)
	})

	t.Run("successful validation updates state correctly", func(t *testing.T) {
		currentSlot := primitives.Slot(200)

		// Create a valid data column
		blockRoot := [32]byte{0x01}
		commitment := make([]byte, 48) // KZG commitments are 48 bytes
		commitment[0] = 0xaa
		params := []util.DataColumnParam{
			{
				Index:          0,
				Slot:           100,
				ProposerIndex:  1,
				ParentRoot:     blockRoot[:],
				KzgCommitments: [][]byte{commitment},
			},
		}
		roCols, verifiedCols := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

		// Mock storage and verifier
		colStore := filesystem.NewEphemeralDataColumnStorage(t)
		p2p := p2ptest.NewTestP2P(t)

		remaining := peerdas.NewColumnIndicesFromSlice([]uint64{0})
		bisector := newColumnBisector(func(peer.ID, string, error) {})

		vcr := &validatingColumnRequest{
			columnSync: &columnSync{
				columnBatch: &columnBatch{
					toDownload: map[[32]byte]*toDownload{
						roCols[0].BlockRoot(): {
							remaining:   remaining,
							commitments: [][]byte{{0xaa}},
						},
					},
				},
				store:   das.NewLazilyPersistentStoreColumn(colStore, testNewDataColumnsVerifier(), p2p.NodeID(), 1, bisector),
				current: currentSlot,
				peer:    mockPeer,
			},
			bisector: bisector,
		}

		// Add peer columns tracking
		vcr.bisector.addPeerColumns(mockPeer, roCols[0])

		// First save the verified column so Persist can work
		err := colStore.Save([]blocks.VerifiedRODataColumn{verifiedCols[0]})
		require.NoError(t, err)

		// Update the columnBatch toDownload to use the correct commitment size
		vcr.columnSync.columnBatch.toDownload[roCols[0].BlockRoot()].commitments = [][]byte{commitment}

		// Now test validation - it should mark the column as downloaded
		require.Equal(t, true, remaining.Has(0), "column 0 should be in remaining before validation")

		err = vcr.countedValidation(roCols[0])
		require.NoError(t, err)

		// Verify that remaining.Unset was called (column 0 should be removed)
		require.Equal(t, false, remaining.Has(0), "column 0 should be removed from remaining after validation")
		require.Equal(t, 0, remaining.Count(), "remaining should be empty")
	})
}

// TestNewColumnSync tests the newColumnSync function
func TestNewColumnSync(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	denebEpoch := params.BeaconConfig().DenebForkEpoch
	if params.BeaconConfig().FuluForkEpoch == params.BeaconConfig().FarFutureEpoch {
		params.BeaconConfig().FuluForkEpoch = denebEpoch + 4096*2
	}
	fuluEpoch := params.BeaconConfig().FuluForkEpoch
	fuluSlot, err := slots.EpochStart(fuluEpoch)
	require.NoError(t, err)

	t.Run("returns nil columnBatch when buildColumnBatch returns nil", func(t *testing.T) {
		ctx := context.Background()
		p2p := p2ptest.NewTestP2P(t)
		colStore := filesystem.NewEphemeralDataColumnStorage(t)
		current := primitives.Slot(100)

		cfg := &workerCfg{
			colStore:  colStore,
			downscore: func(peer.ID, string, error) {},
		}

		// Empty blocks should result in nil columnBatch
		cs, err := newColumnSync(ctx, batch{}, verifiedROBlocks{}, current, p2p, verifiedROBlocks{}, cfg)
		require.NoError(t, err)
		require.NotNil(t, cs, "columnSync should not be nil")
		require.Equal(t, true, cs.columnBatch == nil, "columnBatch should be nil for empty blocks")
	})

	t.Run("successful initialization with Fulu blocks", func(t *testing.T) {
		ctx := context.Background()
		p2p := p2ptest.NewTestP2P(t)
		colStore := filesystem.NewEphemeralDataColumnStorage(t)
		current := fuluSlot + 100

		blks, _ := testBlobGen(t, fuluSlot, 2)
		b := batch{
			begin:  fuluSlot,
			end:    fuluSlot + 10,
			blocks: blks,
		}

		cfg := &workerCfg{
			colStore:  colStore,
			downscore: func(peer.ID, string, error) {},
		}

		cs, err := newColumnSync(ctx, b, blks, current, p2p, verifiedROBlocks{}, cfg)
		require.NoError(t, err)
		require.NotNil(t, cs)
		require.NotNil(t, cs.columnBatch, "columnBatch should be initialized")
		require.NotNil(t, cs.store, "store should be initialized")
		require.NotNil(t, cs.bisector, "bisector should be initialized")
		require.Equal(t, current, cs.current)
	})
}

// TestCurrentCustodiedColumns tests the currentCustodiedColumns function
func TestCurrentCustodiedColumns(t *testing.T) {
	t.Run("successful column indices retrieval", func(t *testing.T) {
		ctx := context.Background()
		p2p := p2ptest.NewTestP2P(t)

		indices, err := currentCustodiedColumns(ctx, p2p)
		require.NoError(t, err)
		require.NotNil(t, indices)
		// Should have some custody columns based on default settings
		require.Equal(t, true, indices.Count() > 0, "should have at least some custody columns")
	})
}

// TestValidatingColumnRequest_Validate tests the validate method
func TestValidatingColumnRequest_Validate(t *testing.T) {
	mockPeer := peer.ID("test-peer")

	t.Run("validate wraps countedValidation and records metrics", func(t *testing.T) {
		// Create a valid data column that won't be in the remaining set (so it skips Persist)
		blockRoot := [32]byte{0x01}
		commitment := make([]byte, 48)
		commitment[0] = 0xaa
		params := []util.DataColumnParam{
			{
				Index:          5, // Not in remaining set, so will skip Persist
				Slot:           100,
				ProposerIndex:  1,
				ParentRoot:     blockRoot[:],
				KzgCommitments: [][]byte{commitment},
			},
		}
		roCols, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

		remaining := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}) // Column 5 not here
		vcr := &validatingColumnRequest{
			columnSync: &columnSync{
				columnBatch: &columnBatch{
					toDownload: map[[32]byte]*toDownload{
						roCols[0].BlockRoot(): {
							remaining:   remaining,
							commitments: [][]byte{commitment},
						},
					},
				},
				peer: mockPeer,
			},
			bisector: newColumnBisector(func(peer.ID, string, error) {}),
		}

		// Call validate (which wraps countedValidation)
		err := vcr.validate(roCols[0])

		// Should succeed - column not in remaining set, so it returns early
		require.NoError(t, err)
	})

	t.Run("validate returns error from countedValidation", func(t *testing.T) {
		// Create a data column with mismatched commitments
		blockRoot := [32]byte{0x01}
		params := []util.DataColumnParam{
			{
				Index:          0,
				Slot:           100,
				ProposerIndex:  1,
				ParentRoot:     blockRoot[:],
				KzgCommitments: [][]byte{{0xaa}, {0xbb}},
			},
		}
		roCols, _ := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

		vcr := &validatingColumnRequest{
			columnSync: &columnSync{
				columnBatch: &columnBatch{
					toDownload: map[[32]byte]*toDownload{
						roCols[0].BlockRoot(): {
							remaining:   peerdas.NewColumnIndicesFromSlice([]uint64{0}),
							commitments: [][]byte{{0xaa}}, // Length mismatch
						},
					},
				},
				peer: mockPeer,
			},
			bisector: newColumnBisector(func(peer.ID, string, error) {}),
		}

		// Call validate
		err := vcr.validate(roCols[0])

		// Should return the error from countedValidation
		require.ErrorIs(t, err, errCommitmentLengthMismatch)
	})
}

// Helper to create a test column verifier
func testNewDataColumnsVerifier() verification.NewDataColumnsVerifier {
	return func([]blocks.RODataColumn, []verification.Requirement) verification.DataColumnsVerifier {
		return &verification.MockDataColumnsVerifier{}
	}
}