package backfill

import (
	"context"
	"slices"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/kzg"
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

// TestColumnBatchNeeded tests the needed() method of columnBatch
func TestColumnBatchNeeded(t *testing.T) {
	t.Run("empty batch conditions", func(t *testing.T) {
		t.Run("empty batch returns empty indices", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
			toDownload := make(map[[32]byte]*toDownload)

			cb := testColumnBatch(custodyGroups, toDownload)
			result := cb.needed()

			require.Equal(t, 0, result.Count(), "needed() should return empty indices for empty batch")
		})

		t.Run("no custody groups returns empty", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndices()
			toDownload := map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
			}

			cb := testColumnBatch(custodyGroups, toDownload)
			result := cb.needed()

			require.Equal(t, 0, result.Count(), "needed() should return empty indices when there are no custody groups")
		})

		t.Run("no commitments returns empty", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
			toDownload := make(map[[32]byte]*toDownload)

			cb := testColumnBatch(custodyGroups, toDownload)
			result := cb.needed()

			require.Equal(t, 0, result.Count(), "needed() should return empty indices when no blocks have commitments")
		})
	})

	t.Run("single block scenarios", func(t *testing.T) {
		t.Run("all columns stored", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
			toDownload := map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndices(), nil),
			}

			cb := testColumnBatch(custodyGroups, toDownload)
			result := cb.needed()

			require.Equal(t, 0, result.Count(), "needed() should return empty indices when all custody columns are stored")
		})

		t.Run("no columns stored", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
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
		})

		t.Run("partial download", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3})
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
		})

		t.Run("table driven cases", func(t *testing.T) {
			cases := []struct {
				name          string
				custodyGroups []uint64
				remaining     []uint64
				expectedCount int
				expectedCols  []uint64
			}{
				{
					name:          "all columns needed",
					custodyGroups: []uint64{0, 1, 2},
					remaining:     []uint64{0, 1, 2},
					expectedCount: 3,
					expectedCols:  []uint64{0, 1, 2},
				},
				{
					name:          "partial columns needed",
					custodyGroups: []uint64{0, 1, 2, 3},
					remaining:     []uint64{1, 3},
					expectedCount: 2,
					expectedCols:  []uint64{1, 3},
				},
				{
					name:          "no columns needed",
					custodyGroups: []uint64{0, 1, 2},
					remaining:     []uint64{},
					expectedCount: 0,
					expectedCols:  []uint64{},
				},
				{
					name:          "remaining has non-custody columns",
					custodyGroups: []uint64{0, 1},
					remaining:     []uint64{0, 5, 10},
					expectedCount: 1,
					expectedCols:  []uint64{0},
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
		})
	})

	t.Run("multiple block scenarios", func(t *testing.T) {
		t.Run("same needs across blocks", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
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
		})

		t.Run("different needs across blocks", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3, 4})
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
		})

		t.Run("mixed block states", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3})
			toDownload := map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndices(), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{1, 3}), nil),
				[32]byte{0x03}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3}), nil),
			}

			cb := testColumnBatch(custodyGroups, toDownload)
			result := cb.needed()

			require.Equal(t, 4, result.Count(), "needed() should return all columns that are needed by at least one block")
			require.Equal(t, true, result.Has(0), "result should contain column 0")
			require.Equal(t, true, result.Has(1), "result should contain column 1")
			require.Equal(t, true, result.Has(2), "result should contain column 2")
			require.Equal(t, true, result.Has(3), "result should contain column 3")
		})
	})

	t.Run("state transitions", func(t *testing.T) {
		t.Run("after unset single block", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
			remaining := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
			toDownload := map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(remaining, nil),
			}

			cb := testColumnBatch(custodyGroups, toDownload)

			result := cb.needed()
			require.Equal(t, 3, result.Count(), "initially, all custody columns should be needed")

			remaining.Unset(1)

			result = cb.needed()
			require.Equal(t, 2, result.Count(), "after Unset(1), only 2 columns should be needed")
			require.Equal(t, true, result.Has(0), "result should still contain column 0")
			require.Equal(t, false, result.Has(1), "result should not contain column 1 after Unset")
			require.Equal(t, true, result.Has(2), "result should still contain column 2")

			remaining.Unset(0)
			remaining.Unset(2)

			result = cb.needed()
			require.Equal(t, 0, result.Count(), "after all columns downloaded, none should be needed")
		})

		t.Run("after partial unset multiple blocks", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
			remaining1 := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
			remaining2 := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2})
			toDownload := map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(remaining1, nil),
				[32]byte{0x02}: testToDownload(remaining2, nil),
			}

			cb := testColumnBatch(custodyGroups, toDownload)

			result := cb.needed()
			require.Equal(t, 3, result.Count(), "initially, all custody columns should be needed")

			remaining1.Unset(0)

			result = cb.needed()
			require.Equal(t, 3, result.Count(), "column 0 still needed by block 2")
			require.Equal(t, true, result.Has(0), "column 0 still in needed set")

			remaining2.Unset(0)

			result = cb.needed()
			require.Equal(t, 2, result.Count(), "column 0 no longer needed by any block")
			require.Equal(t, false, result.Has(0), "column 0 should not be in needed set")
			require.Equal(t, true, result.Has(1), "column 1 still needed")
			require.Equal(t, true, result.Has(2), "column 2 still needed")
		})
	})

	t.Run("edge cases", func(t *testing.T) {
		t.Run("early exit optimization", func(t *testing.T) {
			custodyGroups := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1})
			toDownload := map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
			}

			cb := testColumnBatch(custodyGroups, toDownload)
			result := cb.needed()

			require.Equal(t, 2, result.Count(), "needed() should find all custody columns")
			require.Equal(t, true, result.Has(0), "result should contain column 0")
			require.Equal(t, true, result.Has(1), "result should contain column 1")
		})

		t.Run("large column indices", func(t *testing.T) {
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
		})
	})
}

func nilIshColumnBatch(t *testing.T, cb *columnBatch) {
	if cb == nil {
		return
	}
	require.Equal(t, 0, len(cb.toDownload), "expected toDownload to be empty for nil-ish columnBatch")
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
	specNeeds := mockCurrentSpecNeeds()
	t.Run("empty blocks returns nil", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		cb, err := buildColumnBatch(ctx, batch{}, verifiedROBlocks{}, p, store, specNeeds)
		require.NoError(t, err)
		nilIshColumnBatch(t, cb)
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

		cb, err := buildColumnBatch(ctx, b, blks, p, store, specNeeds)
		require.NoError(t, err)
		nilIshColumnBatch(t, cb)
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

		cb, err := buildColumnBatch(ctx, b, blks, p, store, specNeeds)
		require.NoError(t, err)
		nilIshColumnBatch(t, cb)
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

		cb, err := buildColumnBatch(ctx, b, blks, p, store, specNeeds)
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
			begin: fuluSlot - 31,
			end:   fuluSlot + 1,
		}

		cb, err := buildColumnBatch(ctx, b, blks, p, store, specNeeds)
		require.NoError(t, err)
		require.NotNil(t, cb, "last block at Fulu boundary should not be nil")
	})
	t.Run("boundary: batch ends at fulu, block is pre-fulu", func(t *testing.T) {
		ctx := context.Background()
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)

		// Create blocks at Fulu start
		blks, _ := testBlobGen(t, fuluSlot-1, 1)
		b := batch{
			begin: fuluSlot - 31,
			end:   fuluSlot,
		}

		cb, err := buildColumnBatch(ctx, b, blks, p, store, specNeeds)
		require.NoError(t, err)
		nilIshColumnBatch(t, cb)
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

		cb, err := buildColumnBatch(ctx, b, allBlocks, p, store, specNeeds)
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

		cb, err := buildColumnBatch(ctx, b, blks, p, store, specNeeds)
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

		cb, err := buildColumnBatch(ctx, b, blks, p, store, specNeeds)
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

		cb, err := buildColumnBatch(ctx, b, blks, p, store, specNeeds)
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

		cb, err := buildColumnBatch(ctx, b, allBlocks, p, store, specNeeds)
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

		sr := func(slot primitives.Slot) bool {
			return true
		}
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
				store:   das.NewLazilyPersistentStoreColumn(colStore, testNewDataColumnsVerifier(), p2p.NodeID(), 1, bisector, sr),
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
			colStore:     colStore,
			downscore:    func(peer.ID, string, error) {},
			currentNeeds: mockCurrentNeedsFunc(0, 0),
		}

		// Empty blocks should result in nil columnBatch
		cs, err := newColumnSync(ctx, batch{}, verifiedROBlocks{}, current, p2p, cfg)
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
			colStore:     colStore,
			downscore:    func(peer.ID, string, error) {},
			currentNeeds: func() das.CurrentNeeds { return mockCurrentSpecNeeds() },
		}

		cs, err := newColumnSync(ctx, b, blks, current, p2p, cfg)
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
	cases := []struct {
		name          string
		custodyAmount uint64
		expectedCount int
		err           error
	}{
		{
			name:          "no custody columns",
			custodyAmount: 0,
			expectedCount: 0,
		},
		{
			name:          "some custody columns",
			custodyAmount: 3,
			expectedCount: 3, // shouldn't be allowed, this is a bug in UpdateCustodyInfo/CustodyGroupCount
		},
		{
			name:          "maximum custody columns",
			custodyAmount: params.BeaconConfig().NumberOfCustodyGroups,
			expectedCount: int(params.BeaconConfig().NumberOfCustodyGroups),
		},
		{
			name:          "maximum custody columns",
			custodyAmount: params.BeaconConfig().NumberOfCustodyGroups + 1,
			expectedCount: int(params.BeaconConfig().NumberOfCustodyGroups),
			err:           peerdas.ErrCustodyGroupCountTooLarge,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			p2p := p2ptest.NewTestP2P(t)
			_, _, err := p2p.UpdateCustodyInfo(0, tc.custodyAmount)
			require.NoError(t, err)

			indices, err := currentCustodiedColumns(ctx, p2p)
			if err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, indices)
			require.Equal(t, tc.expectedCount, indices.Count())
		})
	}
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

// TestNeededSidecarsByColumn is a table-driven test that verifies neededSidecarsByColumn
// correctly counts sidecars needed for each column, considering only columns the peer has.
func TestNeededSidecarsByColumn(t *testing.T) {
	cases := []struct {
		name           string
		toDownload     map[[32]byte]*toDownload
		peerHas        peerdas.ColumnIndices
		expectedCounts map[uint64]int
	}{
		{
			name:           "EmptyBatch",
			toDownload:     make(map[[32]byte]*toDownload),
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}),
			expectedCounts: map[uint64]int{},
		},
		{
			name: "EmptyPeer",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
			},
			peerHas:        peerdas.NewColumnIndices(),
			expectedCounts: map[uint64]int{},
		},
		{
			name: "SingleBlockSingleColumn",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0}), nil),
			},
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{0}),
			expectedCounts: map[uint64]int{0: 1},
		},
		{
			name: "SingleBlockMultipleColumns",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
			},
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}),
			expectedCounts: map[uint64]int{0: 1, 1: 1, 2: 1},
		},
		{
			name: "SingleBlockPartialPeer",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
			},
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{0, 2}),
			expectedCounts: map[uint64]int{0: 1, 2: 1},
		},
		{
			name: "MultipleBlocksSameColumns",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
			},
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}),
			expectedCounts: map[uint64]int{0: 2, 1: 2},
		},
		{
			name: "MultipleBlocksDifferentColumns",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{2, 3}), nil),
			},
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3}),
			expectedCounts: map[uint64]int{0: 1, 1: 1, 2: 1, 3: 1},
		},
		{
			name: "PartialBlocksPartialPeer",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{2, 3, 4}), nil),
			},
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{1, 2, 3}),
			expectedCounts: map[uint64]int{1: 1, 2: 2, 3: 1},
		},
		{
			name: "AllColumnsDownloaded",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndices(), nil),
			},
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}),
			expectedCounts: map[uint64]int{},
		},
		{
			name: "PeerHasExtraColumns",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
			},
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3, 4}),
			expectedCounts: map[uint64]int{0: 1, 1: 1},
		},
		{
			name: "LargeBlockCount",
			toDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				for i := range 100 {
					root := [32]byte{byte(i % 256)}
					remaining := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1})
					result[root] = testToDownload(remaining, nil)
				}
				return result
			}(),
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}),
			expectedCounts: map[uint64]int{0: 100, 1: 100},
		},
		{
			name: "MixedWithEmptyRemaining",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndices(), nil),
				[32]byte{0x03}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{1, 2}), nil),
			},
			peerHas:        peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}),
			expectedCounts: map[uint64]int{0: 1, 1: 2, 2: 2},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cb := testColumnBatch(peerdas.NewColumnIndices(), tt.toDownload)
			result := cb.neededSidecarsByColumn(tt.peerHas)

			// Verify result has same number of entries as expected
			require.Equal(t, len(tt.expectedCounts), len(result),
				"result map should have %d entries, got %d", len(tt.expectedCounts), len(result))

			// Verify each expected entry
			for col, expectedCount := range tt.expectedCounts {
				actualCount, exists := result[col]
				require.Equal(t, true, exists,
					"column %d should be in result map", col)
				require.Equal(t, expectedCount, actualCount,
					"column %d should have count %d, got %d", col, expectedCount, actualCount)
			}

			// Verify no unexpected columns in result
			for col := range result {
				_, exists := tt.expectedCounts[col]
				require.Equal(t, true, exists,
					"column %d should not be in result but was found", col)
			}
		})
	}
}

// TestNeededSidecarCount is a table-driven test that verifies neededSidecarCount
// correctly sums the total number of sidecars needed across all blocks in the batch.
func TestNeededSidecarCount(t *testing.T) {
	cases := []struct {
		name       string
		toDownload map[[32]byte]*toDownload
		expected   int
	}{
		{
			name:       "EmptyBatch",
			toDownload: make(map[[32]byte]*toDownload),
			expected:   0,
		},
		{
			name: "SingleBlockEmpty",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndices(), nil),
			},
			expected: 0,
		},
		{
			name: "SingleBlockOneColumn",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0}), nil),
			},
			expected: 1,
		},
		{
			name: "SingleBlockThreeColumns",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
			},
			expected: 3,
		},
		{
			name: "TwoBlocksEmptyEach",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndices(), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndices(), nil),
			},
			expected: 0,
		},
		{
			name: "TwoBlocksOneEach",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0}), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{1}), nil),
			},
			expected: 2,
		},
		{
			name: "TwoBlocksSameColumns",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
			},
			expected: 4,
		},
		{
			name: "TwoBlocksDifferentCounts",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{3, 4}), nil),
			},
			expected: 5,
		},
		{
			name: "ThreeBlocksMixed",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndices(), nil),
				[32]byte{0x03}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{3, 4, 5, 6}), nil),
			},
			expected: 7,
		},
		{
			name: "AllBlocksEmpty",
			toDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				for i := range 5 {
					result[[32]byte{byte(i)}] = testToDownload(peerdas.NewColumnIndices(), nil)
				}
				return result
			}(),
			expected: 0,
		},
		{
			name: "AllBlocksFull",
			toDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 5 blocks, each with 10 columns = 50 total
				for i := range 5 {
					cols := make([]uint64, 10)
					for j := range 10 {
						cols[j] = uint64(j)
					}
					result[[32]byte{byte(i)}] = testToDownload(peerdas.NewColumnIndicesFromSlice(cols), nil)
				}
				return result
			}(),
			expected: 50,
		},
		{
			name: "ProgressiveIncrease",
			toDownload: map[[32]byte]*toDownload{
				[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0}), nil),
				[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
				[32]byte{0x03}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
			},
			expected: 6, // 1 + 2 + 3 = 6
		},
		{
			name: "LargeBlockCount",
			toDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 100 blocks, each with 2 columns = 200 total
				for i := range 100 {
					root := [32]byte{byte(i % 256)}
					remaining := peerdas.NewColumnIndicesFromSlice([]uint64{0, 1})
					result[root] = testToDownload(remaining, nil)
				}
				return result
			}(),
			expected: 200,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cb := testColumnBatch(peerdas.NewColumnIndices(), tt.toDownload)
			result := cb.neededSidecarCount()

			require.Equal(t, tt.expected, result,
				"neededSidecarCount should return %d, got %d", tt.expected, result)
		})
	}
}

// TestColumnSyncRequest is a table-driven test that verifies the request method
// correctly applies fast path (no truncation) and slow path (with truncation) logic
// based on whether the total sidecar count exceeds the limit.
// Test cases use block counts of 4, 16, 32, and 64 with limits of 64 or 512
// to efficiently cover various truncation conditions and block/column shapes.
func TestColumnSyncRequest(t *testing.T) {
	cases := []struct {
		name             string
		buildToDownload  func() map[[32]byte]*toDownload
		reqCols          []uint64
		limit            int
		expectNil        bool
		expectFastPath   bool
		expectedColCount int // Number of columns in result
		expectedMaxCount int // Max sidecar count that should be in result
	}{
		{
			name: "EmptyReqCols",
			buildToDownload: func() map[[32]byte]*toDownload {
				return map[[32]byte]*toDownload{
					[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil),
				}
			},
			reqCols:          []uint64{},
			limit:            64,
			expectNil:        true,
			expectFastPath:   false,
			expectedColCount: -1, // Not checked when nil
		},
		{
			name: "EmptyBatch",
			buildToDownload: func() map[[32]byte]*toDownload {
				return make(map[[32]byte]*toDownload)
			},
			reqCols:          []uint64{0, 1, 2},
			limit:            64,
			expectNil:        false,
			expectFastPath:   true,
			expectedColCount: 3,
		},
		// 4-block tests with limit 64
		{
			name: "FastPath_4blocks_1col",
			buildToDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 4 blocks × 1 column = 4 sidecars < 64 (fast path)
				for i := range 4 {
					result[[32]byte{byte(i)}] = testToDownload(
						peerdas.NewColumnIndicesFromSlice([]uint64{0}), nil)
				}
				return result
			},
			reqCols:          []uint64{0},
			limit:            64,
			expectNil:        false,
			expectFastPath:   true,
			expectedColCount: 1,
		},
		{
			name: "FastPath_4blocks_16cols",
			buildToDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 4 blocks × 16 columns = 64 sidecars = limit (fast path, exactly at)
				for i := range 4 {
					result[[32]byte{byte(i)}] = testToDownload(
						peerdas.NewColumnIndicesFromSlice(makeRange(0, 16)), nil)
				}
				return result
			},
			reqCols:          makeRange(0, 16),
			limit:            64,
			expectNil:        false,
			expectFastPath:   true,
			expectedColCount: 16,
		},
		{
			name: "SlowPath_4blocks_17cols",
			buildToDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 4 blocks × 17 columns = 68 sidecars > 64 (triggers slow path check but no truncation at 512)
				for i := range 4 {
					result[[32]byte{byte(i)}] = testToDownload(
						peerdas.NewColumnIndicesFromSlice(makeRange(0, 17)), nil)
				}
				return result
			},
			reqCols:          makeRange(0, 17),
			limit:            64,
			expectNil:        false,
			expectFastPath:   false,
			expectedColCount: 17, // 17 cols × 4 blocks = 68 < 512, no actual truncation
		},
		// 16-block tests with limit 64
		{
			name: "FastPath_16blocks_1col",
			buildToDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 16 blocks × 1 column = 16 sidecars < 64 (fast path)
				for i := range 16 {
					result[[32]byte{byte(i)}] = testToDownload(
						peerdas.NewColumnIndicesFromSlice([]uint64{0}), nil)
				}
				return result
			},
			reqCols:          []uint64{0},
			limit:            64,
			expectNil:        false,
			expectFastPath:   true,
			expectedColCount: 1,
		},
		{
			name: "FastPath_16blocks_4cols",
			buildToDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 16 blocks × 4 columns = 64 sidecars = limit (fast path, exactly at)
				for i := range 16 {
					result[[32]byte{byte(i)}] = testToDownload(
						peerdas.NewColumnIndicesFromSlice(makeRange(0, 4)), nil)
				}
				return result
			},
			reqCols:          makeRange(0, 4),
			limit:            64,
			expectNil:        false,
			expectFastPath:   true,
			expectedColCount: 4,
		},
		{
			name: "SlowPath_16blocks_5cols_earlytrunc",
			buildToDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 16 blocks × 5 columns = 80 sidecars > 64 (triggers slow path check but no truncation at 512)
				for i := range 16 {
					result[[32]byte{byte(i)}] = testToDownload(
						peerdas.NewColumnIndicesFromSlice(makeRange(0, 5)), nil)
				}
				return result
			},
			reqCols:          makeRange(0, 5),
			limit:            64,
			expectNil:        false,
			expectFastPath:   false,
			expectedColCount: 5, // 5 cols × 16 blocks = 80 < 512, no actual truncation
		},
		// 32-block tests with limit 64
		{
			name: "FastPath_32blocks_1col",
			buildToDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 32 blocks × 1 column = 32 sidecars < 64 (fast path)
				for i := range 32 {
					result[[32]byte{byte(i)}] = testToDownload(
						peerdas.NewColumnIndicesFromSlice([]uint64{0}), nil)
				}
				return result
			},
			reqCols:          []uint64{0},
			limit:            64,
			expectNil:        false,
			expectFastPath:   true,
			expectedColCount: 1,
		},
		{
			name: "FastPath_32blocks_2cols",
			buildToDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 32 blocks × 2 columns = 64 sidecars = limit (fast path, exactly at)
				for i := range 32 {
					result[[32]byte{byte(i)}] = testToDownload(
						peerdas.NewColumnIndicesFromSlice([]uint64{0, 1}), nil)
				}
				return result
			},
			reqCols:          []uint64{0, 1},
			limit:            64,
			expectNil:        false,
			expectFastPath:   true,
			expectedColCount: 2,
		},
		{
			name: "SlowPath_32blocks_3cols_earlytrunc",
			buildToDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 32 blocks × 3 columns = 96 sidecars > 64 (triggers slow path check but no truncation at 512)
				for i := range 32 {
					result[[32]byte{byte(i)}] = testToDownload(
						peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil)
				}
				return result
			},
			reqCols:          []uint64{0, 1, 2},
			limit:            64,
			expectNil:        false,
			expectFastPath:   false,
			expectedColCount: 3, // 3 cols × 32 blocks = 96 < 512, no actual truncation
		},
		// 64-block tests with limit 64 - comprehensive test
		{
			name: "SlowPath_64blocks_10cols_midtrunc",
			buildToDownload: func() map[[32]byte]*toDownload {
				result := make(map[[32]byte]*toDownload)
				// 64 blocks × 10 columns = 640 sidecars > 512 (slow path)
				// Each column appears 64 times: 8 cols = 512, 9 cols = 576
				for i := range 64 {
					result[[32]byte{byte(i)}] = testToDownload(
						peerdas.NewColumnIndicesFromSlice(makeRange(0, 10)), nil)
				}
				return result
			},
			reqCols:          makeRange(0, 10),
			limit:            512, // Use 512 for this one as it tests against columnRequestLimit
			expectNil:        false,
			expectFastPath:   false,
			expectedColCount: 8, // 8 cols × 64 blocks = 512 < 512 (exactly at)
		},
		// Non-uniform block/column distributions
		{
			name: "OverlappingColumns_4blocks",
			buildToDownload: func() map[[32]byte]*toDownload {
				return map[[32]byte]*toDownload{
					[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2}), nil),
					[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{1, 2, 3}), nil),
					[32]byte{0x03}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{2, 3, 4}), nil),
					[32]byte{0x04}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{3, 4, 5}), nil),
				}
			},
			reqCols:          makeRange(0, 6),
			limit:            64,
			expectNil:        false,
			expectFastPath:   true,
			expectedColCount: 6, // 1+2+3+3+2+1 = 12 sidecars
		},
		{
			name: "NonOverlappingColumns_4blocks",
			buildToDownload: func() map[[32]byte]*toDownload {
				return map[[32]byte]*toDownload{
					[32]byte{0x01}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{0, 1, 2, 3}), nil),
					[32]byte{0x02}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{4, 5, 6, 7}), nil),
					[32]byte{0x03}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{8, 9, 10, 11}), nil),
					[32]byte{0x04}: testToDownload(peerdas.NewColumnIndicesFromSlice([]uint64{12, 13, 14, 15}), nil),
				}
			},
			reqCols:          makeRange(0, 16),
			limit:            64,
			expectNil:        false,
			expectFastPath:   true,
			expectedColCount: 16, // 16 sidecars total (one per column)
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cb := testColumnBatch(peerdas.NewColumnIndices(), tt.buildToDownload())
			cs := &columnSync{
				columnBatch: cb,
				current:     primitives.Slot(1),
			}

			result, err := cs.request(tt.reqCols, tt.limit)
			require.NoError(t, err)

			if tt.expectNil {
				require.Equal(t, true, result == nil, "expected nil result")
				return
			}

			require.NotNil(t, result, "expected non-nil result")

			// Verify the returned columns are a prefix of reqCols
			require.Equal(t, true, len(result.Columns) <= len(tt.reqCols),
				"result columns should be a prefix of reqCols")

			// Verify returned columns are in same order and from reqCols
			for i, col := range result.Columns {
				require.Equal(t, tt.reqCols[i], col,
					"result column %d should match reqCols[%d]", i, i)
			}

			// Verify column count matches expectation
			require.Equal(t, tt.expectedColCount, len(result.Columns),
				"expected %d columns in result, got %d", tt.expectedColCount, len(result.Columns))

			// For slow path, verify the result doesn't exceed limit
			if !tt.expectFastPath {
				peerHas := peerdas.NewColumnIndicesFromSlice(result.Columns)
				needed := cs.neededSidecarsByColumn(peerHas)
				totalCount := 0
				for _, count := range needed {
					totalCount += count
				}
				require.Equal(t, true, totalCount <= columnRequestLimit,
					"slow path result should not exceed columnRequestLimit: %d <= 512", totalCount)

				// Verify that if we added the next column from reqCols, we'd exceed the limit
				// We need to recompute with the next column included
				if len(result.Columns) < len(tt.reqCols) {
					nextCol := tt.reqCols[len(result.Columns)]
					// Create a new column set with the next column added
					nextCols := slices.Clone(result.Columns)
					nextCols = append(nextCols, nextCol)
					nextPeerHas := peerdas.NewColumnIndicesFromSlice(nextCols)
					nextNeeded := cs.neededSidecarsByColumn(nextPeerHas)
					nextTotalCount := 0
					for _, count := range nextNeeded {
						nextTotalCount += count
					}
					require.Equal(t, true, nextTotalCount > columnRequestLimit,
						"adding next column should exceed columnRequestLimit: %d > 512", nextTotalCount)
				}
			}
		})
	}
}

// Helper function to create a range of column indices
func makeRange(start, end uint64) []uint64 {
	result := make([]uint64, end-start)
	for i := uint64(0); i < end-start; i++ {
		result[i] = start + i
	}
	return result
}

// Helper to create a test column verifier
func testNewDataColumnsVerifier() verification.NewDataColumnsVerifier {
	return func([]blocks.RODataColumn, []verification.Requirement) verification.DataColumnsVerifier {
		return &verification.MockDataColumnsVerifier{}
	}
}

// Helper to create a verifier that marks all columns as verified
func markAllVerified(m *verification.MockDataColumnsVerifier) verification.NewDataColumnsVerifier {
	return func(cols []blocks.RODataColumn, reqs []verification.Requirement) verification.DataColumnsVerifier {
		m.AppendRODataColumns(cols...)
		return m
	}
}

func TestRetentionCheckWithOverride(t *testing.T) {
	require.NoError(t, kzg.Start())
	fuluEpoch := params.BeaconConfig().FuluForkEpoch
	fuluSlot, err := slots.EpochStart(fuluEpoch)
	require.NoError(t, err)
	colRetention := params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest
	savedSlotRange := func(start, end primitives.Slot) map[primitives.Slot]struct{} {
		m := make(map[primitives.Slot]struct{})
		for s := start; s < end; s++ {
			m[s] = struct{}{}
		}
		return m
	}
	cases := []struct {
		name          string
		span          das.NeedSpan
		savedSlots    map[primitives.Slot]struct{}
		currentEpoch  primitives.Epoch
		retentionFlag primitives.Epoch
		cgc           uint64
		oldestSlot    *primitives.Slot
	}{
		{
			name:         "Before retention period, none saved",
			span:         das.NeedSpan{Begin: fuluSlot, End: fuluSlot + 5},
			currentEpoch: fuluEpoch + colRetention + 1,
			cgc:          4,
		},
		{
			name:         "At retention period boundary, all saved",
			span:         das.NeedSpan{Begin: fuluSlot, End: fuluSlot + 5},
			currentEpoch: fuluEpoch + colRetention,
			savedSlots:   savedSlotRange(fuluSlot, fuluSlot+5),
			cgc:          4,
		},
		{
			name:         "Across boundary, saved after",
			span:         das.NeedSpan{Begin: fuluSlot + 30, End: fuluSlot + 35},
			currentEpoch: fuluEpoch + colRetention + 1,
			savedSlots:   savedSlotRange(fuluSlot+32, fuluSlot+35),
			cgc:          4,
		},
		{
			name:          "Before retention period with override, all saved",
			span:          das.NeedSpan{Begin: fuluSlot, End: fuluSlot + 5},
			currentEpoch:  fuluEpoch + colRetention*2, // well past retention without override
			savedSlots:    savedSlotRange(fuluSlot, fuluSlot+5),
			retentionFlag: fuluEpoch + colRetention*2, // flag covers all slots
		},
		{
			name:          "Before retention period and just before override, none saved",
			span:          das.NeedSpan{Begin: fuluSlot, End: fuluSlot + 5},
			currentEpoch:  1 + fuluEpoch + colRetention*2, // current slot is beyond base retention
			retentionFlag: fuluEpoch + colRetention*2,     // stops 1 short flag coverage
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			colStore := filesystem.NewEphemeralDataColumnStorage(t)
			p2p := p2ptest.NewTestP2P(t)
			bisector := newColumnBisector(func(peer.ID, string, error) {})
			current, err := slots.EpochStart(tc.currentEpoch)
			require.NoError(t, err)
			cs := func() primitives.Slot { return current }
			sn, err := das.NewSyncNeeds(cs, tc.oldestSlot, tc.retentionFlag)
			require.NoError(t, err)

			sr := func(slot primitives.Slot) bool {
				current := sn.Currently()
				return current.Col.At(slot)
			}

			nodeId := p2p.NodeID()
			peerInfo, _, err := peerdas.Info(nodeId, tc.cgc)
			require.NoError(t, err)
			indices := peerdas.NewColumnIndicesFromMap(peerInfo.CustodyColumns)
			// note that markAllVerified considers all columns that get through the rest of the da check as verified,
			// not all columns in the test.
			verifier := markAllVerified(&verification.MockDataColumnsVerifier{})
			avs := das.NewLazilyPersistentStoreColumn(colStore, verifier, p2p.NodeID(), tc.cgc, bisector, sr)
			fixtures := testBlockWithColumnSpan(t, tc.span.Begin, tc.span, 3)
			blocks := make([]blocks.ROBlock, 0, len(fixtures))
			for _, fix := range fixtures {
				for _, col := range fix.cols {
					if indices.Has(col.Index) {
						require.NoError(t, avs.Persist(current, col))
					}
				}
				blocks = append(blocks, fix.block)
			}
			require.NoError(t, avs.IsDataAvailable(t.Context(), current, blocks...))
			for _, block := range blocks {
				slot := block.Block().Slot()
				sum := colStore.Summary(block.Root())
				stored := sum.Stored()
				// If we don't expect storage for the slot, verify none are stored
				if _, exists := tc.savedSlots[slot]; !exists {
					require.Equal(t, 0, len(stored), "expected no columns stored for slot %d", slot)
					continue
				}
				// If we expect storage, verify all stored columns are expected, and that all expected columns are stored.
				missing := indices.Copy()
				for idx := range stored {
					require.Equal(t, true, missing.Has(idx), "unexpected stored column %d for slot %d", idx, slot)
					missing.Unset(idx)
				}
				require.Equal(t, 0, missing.Count(), "expected all columns to be stored for slot %d, missing %v", slot, missing.ToSlice())
			}
		})
	}
}

type blockFixture struct {
	block blocks.ROBlock
	cols  []blocks.RODataColumn
	vcols []blocks.VerifiedRODataColumn
}

func testBlockWithColumnSpan(t *testing.T, slot primitives.Slot, colSpan das.NeedSpan, numBlobs int) []blockFixture {
	res := make([]blockFixture, 0, colSpan.End-colSpan.Begin)
	parent := [32]byte{0x00}
	for bs := colSpan.Begin; bs < colSpan.End; bs++ {
		blk, c, vc := util.GenerateTestFuluBlockWithSidecars(t, numBlobs, util.WithSlot(bs), util.WithParentRoot(parent))
		res = append(res, blockFixture{block: blk, cols: c, vcols: vc})
		parent = blk.Root()
	}
	return res
}
