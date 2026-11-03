package kv

import (
	"context"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/benchmark"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

const maxBenchmarkIterations = 10000

func createBlock(t testing.TB, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	blk := util.NewBeaconBlockDeneb()
	blk.Block.Slot = slot
	blk.Block.Body.Graffiti = []byte("benchmark-block-data-for-htr")
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	return wsb
}

func createROBlock(t testing.TB, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
	wsb := createBlock(t, slot)
	root, err := wsb.Block().HashTreeRoot()
	require.NoError(t, err)
	roblock, err := blocks.NewROBlockWithRoot(wsb, root)
	require.NoError(t, err)
	return roblock
}

type blockCreator func(t testing.TB, slot primitives.Slot) interfaces.ReadOnlySignedBeaconBlock

func BenchmarkBlocks_SaveBlock(b *testing.B) {
	cases := []struct {
		name    string
		creator blockCreator
	}{
		{
			// When non ROBlock is passed, hashtree root is recalculated.
			"RawBlock", createBlock,
		},
		{
			// When ROBlock is passed, hashtree is re-used
			"ROBlock", createROBlock,
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			db := setupDB(b)
			ctx := context.Background()

			// Pre-create blocks to avoid allocation overhead in the hot loop.
			blks := make([]interfaces.ReadOnlySignedBeaconBlock, maxBenchmarkIterations)
			for i := range blks {
				blks[i] = tc.creator(b, primitives.Slot(i+1))
			}

			for i := 0; b.Loop(); i++ {
				err := db.SaveBlock(ctx, blks[i%maxBenchmarkIterations])
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkBlocks_SaveBlocks(b *testing.B) {
	batchSizes := []int{10, 64}
	blockTypes := []struct {
		name    string
		creator blockCreator
	}{
		{"RawBlocks", createBlock},
		{"ROBlocks", createROBlock},
	}

	for _, size := range batchSizes {
		for _, bt := range blockTypes {
			name := fmt.Sprintf("%d/%s", size, bt.name)
			b.Run(name, func(b *testing.B) {
				db := setupDB(b)
				ctx := context.Background()

				// Scale numBatches inversely with batch size to keep memory reasonable.
				numBatches := max(1000/size, 10)

				// Pre-create batches of blocks.
				batches := make([][]interfaces.ReadOnlySignedBeaconBlock, numBatches)
				for batch := range batches {
					blks := make([]interfaces.ReadOnlySignedBeaconBlock, size)
					for i := range blks {
						blks[i] = bt.creator(b, primitives.Slot(batch*size+i+1))
					}
					batches[batch] = blks
				}

				for i := 0; b.Loop(); i++ {
					err := db.SaveBlocks(ctx, batches[i%numBatches])
					require.NoError(b, err)
				}
			})
		}
	}
}

func BenchmarkBlocks_BlockHashTreeRoot(b *testing.B) {
	blk := createBlock(b, 1)

	for b.Loop() {
		// This is essentially a baseline cost we are trying to eliminate.
		_, err := blk.Block().HashTreeRoot()
		require.NoError(b, err)
	}
}

func BenchmarkBlocks_ROBlockTypeAssertion(b *testing.B) {
	roblock := createROBlock(b, 1)
	var signed interfaces.ReadOnlySignedBeaconBlock = roblock

	for b.Loop() {
		// This is the overhead our optimization adds, it should be negligible.
		if rob, ok := signed.(blocks.ROBlock); ok {
			_ = rob.Root()
		}
	}
}

func BenchmarkBlocks_FullBlock_SaveBlock(b *testing.B) {
	undo, err := benchmark.SetBenchmarkConfig()
	require.NoError(b, err)
	defer undo()

	// Load pre-generated full block with attestations.
	// This represents realistic mainnet block sizes where HTR cost is significant.
	fullBlock, err := benchmark.PreGenFullBlock()
	require.NoError(b, err)

	cases := []struct {
		name      string
		createBlk func() (interfaces.ReadOnlySignedBeaconBlock, error)
	}{
		{
			name: "RawBlock",
			createBlk: func() (interfaces.ReadOnlySignedBeaconBlock, error) {
				return blocks.NewSignedBeaconBlock(fullBlock)
			},
		},
		{
			name: "ROBlock",
			createBlk: func() (interfaces.ReadOnlySignedBeaconBlock, error) {
				wsb, err := blocks.NewSignedBeaconBlock(fullBlock)
				if err != nil {
					return nil, err
				}
				root, err := wsb.Block().HashTreeRoot()
				if err != nil {
					return nil, err
				}
				return blocks.NewROBlockWithRoot(wsb, root)
			},
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			db := setupDB(b)
			ctx := context.Background()

			// Pre-create blocks to avoid setup overhead in hot loop.
			blks := make([]interfaces.ReadOnlySignedBeaconBlock, 100)
			for i := range blks {
				blks[i], err = tc.createBlk()
				require.NoError(b, err)
			}

			for i := 0; b.Loop(); i++ {
				err := db.SaveBlock(ctx, blks[i%len(blks)])
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkBlocks_FullBlock_HashTreeRoot(b *testing.B) {
	undo, err := benchmark.SetBenchmarkConfig()
	require.NoError(b, err)
	defer undo()

	fullBlock, err := benchmark.PreGenFullBlock()
	require.NoError(b, err)
	wsb, err := blocks.NewSignedBeaconBlock(fullBlock)
	require.NoError(b, err)

	for b.Loop() {
		// More realistic (non-empty block) baseline cost that optimization should eliminate.
		_, err := wsb.Block().HashTreeRoot()
		require.NoError(b, err)
	}
}
