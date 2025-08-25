package kv

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filters"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"
)

type testNewBlockFunc func(primitives.Slot, []byte) (interfaces.ReadOnlySignedBeaconBlock, error)

var blockTests = []struct {
	name     string
	newBlock testNewBlockFunc
}{
	{
		name: "phase0",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBeaconBlock()
			b.Block.Slot = slot
			if root != nil {
				b.Block.ParentRoot = root
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
	{
		name: "altair",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBeaconBlockAltair()
			b.Block.Slot = slot
			if root != nil {
				b.Block.ParentRoot = root
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
	{
		name: "bellatrix",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBeaconBlockBellatrix()
			b.Block.Slot = slot
			if root != nil {
				b.Block.ParentRoot = root
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
	{
		name: "bellatrix blind",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBlindedBeaconBlockBellatrix()
			b.Block.Slot = slot
			if root != nil {
				b.Block.ParentRoot = root
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
	{
		name: "capella",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBeaconBlockCapella()
			b.Block.Slot = slot
			if root != nil {
				b.Block.ParentRoot = root
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
	{
		name: "capella blind",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBlindedBeaconBlockCapella()
			b.Block.Slot = slot
			if root != nil {
				b.Block.ParentRoot = root
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
	{
		name: "deneb",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBeaconBlockDeneb()
			b.Block.Slot = slot
			if root != nil {
				b.Block.ParentRoot = root
				b.Block.Body.BlobKzgCommitments = [][]byte{
					bytesutil.PadTo([]byte{0x01}, 48),
					bytesutil.PadTo([]byte{0x02}, 48),
					bytesutil.PadTo([]byte{0x03}, 48),
					bytesutil.PadTo([]byte{0x04}, 48),
				}
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
	{
		name: "deneb blind",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBlindedBeaconBlockDeneb()
			b.Message.Slot = slot
			if root != nil {
				b.Message.ParentRoot = root
				b.Message.Body.BlobKzgCommitments = [][]byte{
					bytesutil.PadTo([]byte{0x05}, 48),
					bytesutil.PadTo([]byte{0x06}, 48),
					bytesutil.PadTo([]byte{0x07}, 48),
					bytesutil.PadTo([]byte{0x08}, 48),
				}
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
	{
		name: "electra",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBeaconBlockElectra()
			b.Block.Slot = slot
			if root != nil {
				b.Block.ParentRoot = root
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
	{
		name: "electra blind",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBlindedBeaconBlockElectra()
			b.Message.Slot = slot
			if root != nil {
				b.Message.ParentRoot = root
			}
			return blocks.NewSignedBeaconBlock(b)
		}},
}

func TestStore_SaveBlock_NoDuplicates(t *testing.T) {
	BlockCacheSize = 1
	slot := primitives.Slot(20)
	ctx := t.Context()

	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)

			// First we save a previous block to ensure the cache max size is reached.
			prevBlock, err := tt.newBlock(slot-1, bytesutil.PadTo([]byte{1, 2, 3}, 32))
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, prevBlock))

			blk, err := tt.newBlock(slot, bytesutil.PadTo([]byte{1, 2, 3}, 32))
			require.NoError(t, err)

			// Even with a full cache, saving new blocks should not cause
			// duplicated blocks in the DB.
			for i := 0; i < 100; i++ {
				require.NoError(t, db.SaveBlock(ctx, blk))
			}

			f := filters.NewFilter().SetStartSlot(slot).SetEndSlot(slot)
			retrieved, _, err := db.Blocks(ctx, f)
			require.NoError(t, err)
			assert.Equal(t, 1, len(retrieved))
		})
	}
	// We reset the block cache size.
	BlockCacheSize = 256
}

func TestStore_BlocksCRUD(t *testing.T) {
	ctx := t.Context()

	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)

			blk, err := tt.newBlock(primitives.Slot(20), bytesutil.PadTo([]byte{1, 2, 3}, 32))
			require.NoError(t, err)
			blockRoot, err := blk.Block().HashTreeRoot()
			require.NoError(t, err)

			_, err = db.getBlock(ctx, blockRoot, nil)
			require.ErrorIs(t, err, ErrNotFound)
			retrievedBlock, err := db.Block(ctx, blockRoot)
			require.NoError(t, err)
			assert.DeepEqual(t, nil, retrievedBlock, "Expected nil block")
			_, err = db.getBlock(ctx, blockRoot, nil)
			require.ErrorIs(t, err, ErrNotFound)

			require.NoError(t, db.SaveBlock(ctx, blk))
			assert.Equal(t, true, db.HasBlock(ctx, blockRoot), "Expected block to exist in the db")
			retrievedBlock, err = db.Block(ctx, blockRoot)
			require.NoError(t, err)
			wanted := retrievedBlock
			if retrievedBlock.Version() >= version.Bellatrix {
				wanted, err = retrievedBlock.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)
			retrievedPb, err := retrievedBlock.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, retrievedPb), "Wanted: %v, received: %v", wanted, retrievedBlock)
			// Check that the block is in the slot->block index
			found, roots, err := db.BlockRootsBySlot(ctx, blk.Block().Slot())
			require.NoError(t, err)
			require.Equal(t, true, found)
			require.Equal(t, 1, len(roots))
			require.Equal(t, blockRoot, roots[0])
			// Delete the block, then check that it is no longer in the index.

			parent := blk.Block().ParentRoot()
			testCheckParentIndices(t, db.db, parent, true)
			require.NoError(t, db.DeleteBlock(ctx, blockRoot))
			require.NoError(t, err)
			testCheckParentIndices(t, db.db, parent, false)
			found, roots, err = db.BlockRootsBySlot(ctx, blk.Block().Slot())
			require.NoError(t, err)
			require.Equal(t, false, found)
			require.Equal(t, 0, len(roots))
		})
	}
}

func testCheckParentIndices(t *testing.T, db *bolt.DB, parent [32]byte, expected bool) {
	require.NoError(t, db.View(func(tx *bolt.Tx) error {
		require.Equal(t, expected, tx.Bucket(blockParentRootIndicesBucket).Get(parent[:]) != nil)
		return nil
	}))
}

func TestStore_BlocksHandleZeroCase(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := t.Context()
			numBlocks := 10
			totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, numBlocks)
			for i := 0; i < len(totalBlocks); i++ {
				b, err := tt.newBlock(primitives.Slot(i), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				totalBlocks[i] = b
				_, err = totalBlocks[i].Block().HashTreeRoot()
				require.NoError(t, err)
			}
			require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
			zeroFilter := filters.NewFilter().SetStartSlot(0).SetEndSlot(0)
			retrieved, _, err := db.Blocks(ctx, zeroFilter)
			require.NoError(t, err)
			assert.Equal(t, 1, len(retrieved), "Unexpected number of blocks received, expected one")
		})
	}
}

func TestStore_BlocksHandleInvalidEndSlot(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := t.Context()
			numBlocks := 10
			totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, numBlocks)
			// Save blocks from slot 1 onwards.
			for i := 0; i < len(totalBlocks); i++ {
				b, err := tt.newBlock(primitives.Slot(i+1), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				totalBlocks[i] = b
				_, err = totalBlocks[i].Block().HashTreeRoot()
				require.NoError(t, err)
			}
			require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
			badFilter := filters.NewFilter().SetStartSlot(5).SetEndSlot(1)
			_, _, err := db.Blocks(ctx, badFilter)
			require.ErrorContains(t, errInvalidSlotRange.Error(), err)

			goodFilter := filters.NewFilter().SetStartSlot(0).SetEndSlot(1)
			requested, _, err := db.Blocks(ctx, goodFilter)
			require.NoError(t, err)
			assert.Equal(t, 1, len(requested), "Unexpected number of blocks received, only expected two")
		})
	}
}

func TestStore_DeleteBlock(t *testing.T) {
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	db := setupDB(t)
	ctx := t.Context()

	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisBlockRoot))
	blks := makeBlocks(t, 0, slotsPerEpoch*4, genesisBlockRoot)
	require.NoError(t, db.SaveBlocks(ctx, blks))
	ss := make([]*ethpb.StateSummary, len(blks))
	for i, blk := range blks {
		r, err := blk.Block().HashTreeRoot()
		require.NoError(t, err)
		ss[i] = &ethpb.StateSummary{
			Slot: blk.Block().Slot(),
			Root: r[:],
		}
	}
	require.NoError(t, db.SaveStateSummaries(ctx, ss))

	root, err := blks[slotsPerEpoch].Block().HashTreeRoot()
	require.NoError(t, err)
	cp := &ethpb.Checkpoint{
		Epoch: 1,
		Root:  root[:],
	}
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, cp))

	root2, err := blks[4*slotsPerEpoch-2].Block().HashTreeRoot()
	require.NoError(t, err)
	b, err := db.Block(ctx, root2)
	require.NoError(t, err)
	require.NotNil(t, b)
	require.NoError(t, db.DeleteBlock(ctx, root2))
	st, err = db.State(ctx, root2)
	require.NoError(t, err)
	require.Equal(t, st, nil)

	b, err = db.Block(ctx, root2)
	require.NoError(t, err)
	require.Equal(t, b, nil)
	require.Equal(t, false, db.HasStateSummary(ctx, root2))

	require.ErrorIs(t, db.DeleteBlock(ctx, root), ErrDeleteJustifiedAndFinalized)
}

func TestStore_DeleteJustifiedBlock(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	b := util.NewBeaconBlock()
	b.Block.Slot = 1
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	cp := &ethpb.Checkpoint{
		Root: root[:],
	}
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	blk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, blk))
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveJustifiedCheckpoint(ctx, cp))
	require.ErrorIs(t, db.DeleteBlock(ctx, root), ErrDeleteJustifiedAndFinalized)
}

func TestStore_DeleteFinalizedBlock(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	b := util.NewBeaconBlock()
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	cp := &ethpb.Checkpoint{
		Root: root[:],
	}
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	blk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, blk))
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, cp))
	require.ErrorIs(t, db.DeleteBlock(ctx, root), ErrDeleteJustifiedAndFinalized)
}

func TestStore_HistoricalDataBeforeSlot(t *testing.T) {
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	ctx := t.Context()

	tests := []struct {
		name             string
		batchSize        int
		numOfEpochs      uint64
		deleteBeforeSlot uint64
	}{
		{
			name:             "batchSize less than delete range",
			batchSize:        10,
			numOfEpochs:      4,
			deleteBeforeSlot: 25,
		},
		{
			name:             "batchSize greater than delete range",
			batchSize:        30,
			numOfEpochs:      4,
			deleteBeforeSlot: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			// Save genesis block root
			require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisBlockRoot))

			// Create and save blocks for given epochs
			blks := makeBlocks(t, 0, slotsPerEpoch*tt.numOfEpochs, genesisBlockRoot)
			require.NoError(t, db.SaveBlocks(ctx, blks))

			// Mark state validator migration as complete
			err := db.db.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(migrationsBucket).Put(migrationStateValidatorsKey, migrationCompleted)
			})
			require.NoError(t, err)

			migrated, err := db.isStateValidatorMigrationOver()
			require.NoError(t, err)
			require.Equal(t, true, migrated)

			// Create state summaries and states for each block
			ss := make([]*ethpb.StateSummary, len(blks))
			states := make([]state.BeaconState, len(blks))

			for i, blk := range blks {
				slot := blk.Block().Slot()
				r, err := blk.Block().HashTreeRoot()
				require.NoError(t, err)

				// Create and save state summary
				ss[i] = &ethpb.StateSummary{
					Slot: slot,
					Root: r[:],
				}

				// Create and save state with validator entries
				vals := make([]*ethpb.Validator, 2)
				for j := range vals {
					vals[j] = &ethpb.Validator{
						PublicKey:             bytesutil.PadTo([]byte{byte(i*j + 1)}, 48),
						WithdrawalCredentials: bytesutil.PadTo([]byte{byte(i*j + 2)}, 32),
					}
				}

				st, err := util.NewBeaconState(func(state *ethpb.BeaconState) error {
					state.Validators = vals
					state.Slot = slot
					return nil
				})
				require.NoError(t, err)
				require.NoError(t, db.SaveState(ctx, st, r))
				states[i] = st

				// Verify validator entries are saved to db
				valsActual, err := db.validatorEntries(ctx, r)
				require.NoError(t, err)
				for j, val := range valsActual {
					require.DeepEqual(t, vals[j], val)
				}
			}
			require.NoError(t, db.SaveStateSummaries(ctx, ss))

			// Verify slot indices exist before deletion
			err = db.db.View(func(tx *bolt.Tx) error {
				blockSlotBkt := tx.Bucket(blockSlotIndicesBucket)
				stateSlotBkt := tx.Bucket(stateSlotIndicesBucket)

				for i := uint64(0); i < uint64(tt.deleteBeforeSlot); i++ {
					slot := bytesutil.SlotToBytesBigEndian(primitives.Slot(i + 1))
					assert.NotNil(t, blockSlotBkt.Get(slot), "Expected block slot index to exist")
					assert.NotNil(t, stateSlotBkt.Get(slot), "Expected state slot index to exist", i)
				}
				return nil
			})
			require.NoError(t, err)

			// Delete data before slot
			slotsDeleted, err := db.DeleteHistoricalDataBeforeSlot(ctx, primitives.Slot(tt.deleteBeforeSlot), tt.batchSize)
			require.NoError(t, err)

			var startSlotDeleted, endSlotDeleted uint64
			if tt.batchSize >= int(tt.deleteBeforeSlot) {
				startSlotDeleted = 1
				endSlotDeleted = tt.deleteBeforeSlot
			} else {
				startSlotDeleted = tt.deleteBeforeSlot - uint64(tt.batchSize) + 1
				endSlotDeleted = tt.deleteBeforeSlot
			}

			require.Equal(t, endSlotDeleted-startSlotDeleted+1, uint64(slotsDeleted))

			// Verify blocks before given slot/batch are deleted
			for i := startSlotDeleted; i < endSlotDeleted; i++ {
				root, err := blks[i].Block().HashTreeRoot()
				require.NoError(t, err)

				// Check block is deleted
				retrievedBlocks, err := db.BlocksBySlot(ctx, primitives.Slot(i))
				require.NoError(t, err)
				assert.Equal(t, 0, len(retrievedBlocks), fmt.Sprintf("Expected %d blocks, got %d for slot %d", 0, len(retrievedBlocks), i))

				// Verify block does not exist
				assert.Equal(t, false, db.HasBlock(ctx, root), fmt.Sprintf("Expected block index to not exist for slot %d", i))

				// Verify block parent root does not exist
				err = db.db.View(func(tx *bolt.Tx) error {
					require.Equal(t, 0, len(tx.Bucket(blockParentRootIndicesBucket).Get(root[:])))
					return nil
				})
				require.NoError(t, err)

				// Verify state is deleted
				hasState := db.HasState(ctx, root)
				assert.Equal(t, false, hasState)

				// Verify state summary is deleted
				hasSummary := db.HasStateSummary(ctx, root)
				assert.Equal(t, false, hasSummary)

				// Verify validator hashes for block roots are deleted
				err = db.db.View(func(tx *bolt.Tx) error {
					assert.Equal(t, 0, len(tx.Bucket(blockRootValidatorHashesBucket).Get(root[:])))
					return nil
				})
				require.NoError(t, err)
			}

			// Verify slot indices are deleted
			err = db.db.View(func(tx *bolt.Tx) error {
				blockSlotBkt := tx.Bucket(blockSlotIndicesBucket)
				stateSlotBkt := tx.Bucket(stateSlotIndicesBucket)

				for i := startSlotDeleted; i < endSlotDeleted; i++ {
					slot := bytesutil.SlotToBytesBigEndian(primitives.Slot(i + 1))
					assert.Equal(t, 0, len(blockSlotBkt.Get(slot)), fmt.Sprintf("Expected block slot index to be deleted, slot: %d", slot))
					assert.Equal(t, 0, len(stateSlotBkt.Get(slot)), fmt.Sprintf("Expected state slot index to be deleted, slot: %d", slot))
				}
				return nil
			})
			require.NoError(t, err)

			// Verify blocks from expectedLastDeletedSlot till numEpochs still exist
			for i := endSlotDeleted; i < slotsPerEpoch*tt.numOfEpochs; i++ {
				root, err := blks[i].Block().HashTreeRoot()
				require.NoError(t, err)

				// Verify block exists
				assert.Equal(t, true, db.HasBlock(ctx, root))

				// Verify remaining block parent root exists, except last slot since we store parent roots of each block.
				if i < slotsPerEpoch*tt.numOfEpochs-1 {
					err = db.db.View(func(tx *bolt.Tx) error {
						require.NotNil(t, tx.Bucket(blockParentRootIndicesBucket).Get(root[:]), fmt.Sprintf("Expected block parent index to be deleted, slot: %d", i))
						return nil
					})
					require.NoError(t, err)
				}

				// Verify state exists
				hasState := db.HasState(ctx, root)
				assert.Equal(t, true, hasState)

				// Verify state summary exists
				hasSummary := db.HasStateSummary(ctx, root)
				assert.Equal(t, true, hasSummary)

				// Verify slot indices still exist
				err = db.db.View(func(tx *bolt.Tx) error {
					blockSlotBkt := tx.Bucket(blockSlotIndicesBucket)
					stateSlotBkt := tx.Bucket(stateSlotIndicesBucket)

					slot := bytesutil.SlotToBytesBigEndian(primitives.Slot(i + 1))
					assert.NotNil(t, blockSlotBkt.Get(slot), "Expected block slot index to exist")
					assert.NotNil(t, stateSlotBkt.Get(slot), "Expected state slot index to exist")
					return nil
				})
				require.NoError(t, err)

				// Verify validator entries still exist
				valsActual, err := db.validatorEntries(ctx, root)
				require.NoError(t, err)
				assert.NotNil(t, valsActual)

				// Verify remaining validator hashes for block roots exists
				err = db.db.View(func(tx *bolt.Tx) error {
					assert.NotNil(t, tx.Bucket(blockRootValidatorHashesBucket).Get(root[:]))
					return nil
				})
				require.NoError(t, err)
			}
		})
	}

}

func TestStore_GenesisBlock(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	genesisBlock := util.NewBeaconBlock()
	genesisBlock.Block.ParentRoot = bytesutil.PadTo([]byte{1, 2, 3}, 32)
	blockRoot, err := genesisBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, blockRoot))
	wsb, err := blocks.NewSignedBeaconBlock(genesisBlock)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))
	retrievedBlock, err := db.GenesisBlock(ctx)
	require.NoError(t, err)
	retrievedBlockPb, err := retrievedBlock.Proto()
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(genesisBlock, retrievedBlockPb), "Wanted: %v, received: %v", genesisBlock, retrievedBlock)
}

func TestStore_BlocksCRUD_NoCache(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := t.Context()
			blk, err := tt.newBlock(primitives.Slot(20), bytesutil.PadTo([]byte{1, 2, 3}, 32))
			require.NoError(t, err)
			blockRoot, err := blk.Block().HashTreeRoot()
			require.NoError(t, err)
			retrievedBlock, err := db.Block(ctx, blockRoot)
			require.NoError(t, err)
			require.DeepEqual(t, nil, retrievedBlock, "Expected nil block")
			require.NoError(t, db.SaveBlock(ctx, blk))
			db.blockCache.Del(string(blockRoot[:]))
			assert.Equal(t, true, db.HasBlock(ctx, blockRoot), "Expected block to exist in the db")
			retrievedBlock, err = db.Block(ctx, blockRoot)
			require.NoError(t, err)

			wanted := blk
			if blk.Version() >= version.Bellatrix {
				wanted, err = blk.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)
			retrievedPb, err := retrievedBlock.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, retrievedPb), "Wanted: %v, received: %v", wanted, retrievedBlock)
		})
	}
}

func TestStore_Blocks_FiltersCorrectly(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			b4, err := tt.newBlock(primitives.Slot(4), bytesutil.PadTo([]byte("parent"), 32))
			require.NoError(t, err)
			b5, err := tt.newBlock(primitives.Slot(5), bytesutil.PadTo([]byte("parent2"), 32))
			require.NoError(t, err)
			b6, err := tt.newBlock(primitives.Slot(6), bytesutil.PadTo([]byte("parent2"), 32))
			require.NoError(t, err)
			b7, err := tt.newBlock(primitives.Slot(7), bytesutil.PadTo([]byte("parent3"), 32))
			require.NoError(t, err)
			b8, err := tt.newBlock(primitives.Slot(8), bytesutil.PadTo([]byte("parent4"), 32))
			require.NoError(t, err)
			blocks := []interfaces.ReadOnlySignedBeaconBlock{
				b4,
				b5,
				b6,
				b7,
				b8,
			}
			ctx := t.Context()
			require.NoError(t, db.SaveBlocks(ctx, blocks))

			tests := []struct {
				filter            *filters.QueryFilter
				expectedNumBlocks int
			}{
				{
					filter:            filters.NewFilter().SetParentRoot(bytesutil.PadTo([]byte("parent2"), 32)),
					expectedNumBlocks: 2,
				},
				{
					// No block meets the criteria below.
					filter:            filters.NewFilter().SetParentRoot(bytesutil.PadTo([]byte{3, 4, 5}, 32)),
					expectedNumBlocks: 0,
				},
				{
					// Block slot range filter criteria.
					filter:            filters.NewFilter().SetStartSlot(5).SetEndSlot(7),
					expectedNumBlocks: 3,
				},
				{
					filter:            filters.NewFilter().SetStartSlot(7).SetEndSlot(7),
					expectedNumBlocks: 1,
				},
				{
					filter:            filters.NewFilter().SetStartSlot(4).SetEndSlot(8),
					expectedNumBlocks: 5,
				},
				{
					filter:            filters.NewFilter().SetStartSlot(4).SetEndSlot(5),
					expectedNumBlocks: 2,
				},
				{
					filter:            filters.NewFilter().SetStartSlot(5).SetEndSlot(9),
					expectedNumBlocks: 4,
				},
				{
					filter:            filters.NewFilter().SetEndSlot(7),
					expectedNumBlocks: 4,
				},
				{
					filter:            filters.NewFilter().SetEndSlot(8),
					expectedNumBlocks: 5,
				},
				{
					filter:            filters.NewFilter().SetStartSlot(5).SetEndSlot(10),
					expectedNumBlocks: 4,
				},
				{
					// Composite filter criteria.
					filter: filters.NewFilter().
						SetParentRoot(bytesutil.PadTo([]byte("parent2"), 32)).
						SetStartSlot(6).
						SetEndSlot(8),
					expectedNumBlocks: 1,
				},
			}
			for _, tt2 := range tests {
				retrievedBlocks, _, err := db.Blocks(ctx, tt2.filter)
				require.NoError(t, err)
				assert.Equal(t, tt2.expectedNumBlocks, len(retrievedBlocks), "Unexpected number of blocks")
			}
		})
	}
}

func testBlockChain(t *testing.T, nb testNewBlockFunc, slots []primitives.Slot, parent []byte) []interfaces.ReadOnlySignedBeaconBlock {
	if len(parent) < 32 {
		var zero [32]byte
		copy(parent, zero[:])
	}
	chain := make([]interfaces.ReadOnlySignedBeaconBlock, 0, len(slots))
	for _, slot := range slots {
		pr := make([]byte, 32)
		copy(pr, parent)
		b, err := nb(slot, pr)
		require.NoError(t, err)
		chain = append(chain, b)
		npr, err := b.Block().HashTreeRoot()
		parent = npr[:]
		require.NoError(t, err)
	}
	return chain
}

func testSlotSlice(start, end primitives.Slot) []primitives.Slot {
	end += 1 // add 1 to make the range inclusive
	slots := make([]primitives.Slot, 0, end-start)
	for ; start < end; start++ {
		slots = append(slots, start)
	}
	return slots
}

func TestCleanupMissingBlockIndices(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			db := setupDB(t)
			chain := testBlockChain(t, tt.newBlock, testSlotSlice(1, 10), nil)
			require.NoError(t, db.SaveBlocks(ctx, chain))
			corrupt, err := blocks.NewROBlock(chain[5])
			require.NoError(t, err)
			cr := corrupt.Root()
			require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(blocksBucket).Delete(cr[:])
			}))
			// Need to also delete it from the cache!!
			db.blockCache.Del(string(cr[:]))
			res, roots, err := db.Blocks(ctx, filters.NewFilter().SetEndSlot(10).SetStartSlot(1))
			require.NoError(t, err)
			require.Equal(t, 9, len(roots))
			require.Equal(t, len(res), len(roots))
			require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
				encSlot := bytesutil.SlotToBytesBigEndian(corrupt.Block().Slot())
				// make sure slot->root index is cleaned up
				require.Equal(t, 0, len(tx.Bucket(blockSlotIndicesBucket).Get(encSlot)))
				require.Equal(t, 0, len(tx.Bucket(blockParentRootIndicesBucket).Get(cr[:])))
				return nil
			}))
		})
	}
}

func TestCleanupMissingForkedBlockIndices(t *testing.T) {
	for _, tt := range blockTests[0:1] {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			db := setupDB(t)

			chain := testBlockChain(t, tt.newBlock, testSlotSlice(1, 10), nil)
			require.NoError(t, db.SaveBlocks(ctx, chain))

			// forkChain should skip the slot at skipBlock, and have the same parent
			skipBlockParent := chain[4].Block().ParentRoot()
			// It should start at the same slot as missingBlock, which comes one slot after the skip slot,
			// so there are 2 blocks in that slot
			missingBlock, err := blocks.NewROBlock(chain[5])
			require.NoError(t, err)
			// missingBlock will be deleted in the main chain, but there will be a block at that slot in the fork chain
			forkChain := testBlockChain(t, tt.newBlock, testSlotSlice(missingBlock.Block().Slot(), 10), skipBlockParent[:])
			require.NoError(t, db.SaveBlocks(ctx, forkChain))
			forkChainStart, err := blocks.NewROBlock(forkChain[0])
			require.NoError(t, err)

			encMissingSlot := bytesutil.SlotToBytesBigEndian(missingBlock.Block().Slot())
			require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
				require.Equal(t, 32, len(tx.Bucket(blockParentRootIndicesBucket).Get(missingBlock.RootSlice())))
				// There are 2 block roots packed in this slot, so it is 64 bytes long
				require.Equal(t, 64, len(tx.Bucket(blockSlotIndicesBucket).Get(encMissingSlot)))
				// skipBlockParent should also have 2 entries and be 64 bytes, since the forkChain is based on the same parent as the skip block
				childRoots := tx.Bucket(blockParentRootIndicesBucket).Get(skipBlockParent[:])
				require.Equal(t, 64, len(childRoots))
				return nil
			}))

			require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(blocksBucket).Delete(missingBlock.RootSlice())
			}))
			// Need to also delete it from the cache!!
			db.blockCache.Del(string(missingBlock.RootSlice()))

			// Blocks should give us blocks from all chains.
			res, roots, err := db.Blocks(ctx, filters.NewFilter().SetEndSlot(10).SetStartSlot(1))
			require.NoError(t, err)
			require.Equal(t, (len(chain)-1)+len(forkChain), len(roots))
			require.Equal(t, len(res), len(roots))
			require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
				// There should now be 32 bytes in this index - one root from the forked chain
				slotIdxVal := tx.Bucket(blockSlotIndicesBucket).Get(encMissingSlot)
				require.Equal(t, forkChainStart.Root(), [32]byte(slotIdxVal))
				require.Equal(t, 0, len(tx.Bucket(blockParentRootIndicesBucket).Get(missingBlock.RootSlice())))
				forkChildRoot := tx.Bucket(blockParentRootIndicesBucket).Get(skipBlockParent[:])
				require.Equal(t, 64, len(forkChildRoot))
				return nil
			}))
		})
	}
}

func TestStore_Blocks_VerifyBlockRoots(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			db := setupDB(t)
			b1, err := tt.newBlock(primitives.Slot(1), nil)
			require.NoError(t, err)
			r1, err := b1.Block().HashTreeRoot()
			require.NoError(t, err)
			b2, err := tt.newBlock(primitives.Slot(2), nil)
			require.NoError(t, err)
			r2, err := b2.Block().HashTreeRoot()
			require.NoError(t, err)

			require.NoError(t, db.SaveBlock(ctx, b1))
			require.NoError(t, db.SaveBlock(ctx, b2))

			filter := filters.NewFilter().SetStartSlot(b1.Block().Slot()).SetEndSlot(b2.Block().Slot())
			roots, err := db.BlockRoots(ctx, filter)
			require.NoError(t, err)

			assert.DeepEqual(t, [][32]byte{r1, r2}, roots)
		})
	}
}

func TestStore_Blocks_Retrieve_SlotRange(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, 500)
			for i := 0; i < 500; i++ {
				b, err := tt.newBlock(primitives.Slot(i), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				totalBlocks[i] = b
			}
			ctx := t.Context()
			require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
			retrieved, _, err := db.Blocks(ctx, filters.NewFilter().SetStartSlot(100).SetEndSlot(399))
			require.NoError(t, err)
			assert.Equal(t, 300, len(retrieved))
		})
	}
}

func TestStore_Blocks_Retrieve_Epoch(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			slots := params.BeaconConfig().SlotsPerEpoch.Mul(7)
			totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, slots)
			for i := primitives.Slot(0); i < slots; i++ {
				b, err := tt.newBlock(i, bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				totalBlocks[i] = b
			}
			ctx := t.Context()
			require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
			retrieved, _, err := db.Blocks(ctx, filters.NewFilter().SetStartEpoch(5).SetEndEpoch(6))
			require.NoError(t, err)
			want := params.BeaconConfig().SlotsPerEpoch.Mul(2)
			assert.Equal(t, uint64(want), uint64(len(retrieved)))
			retrieved, _, err = db.Blocks(ctx, filters.NewFilter().SetStartEpoch(0).SetEndEpoch(0))
			require.NoError(t, err)
			want = params.BeaconConfig().SlotsPerEpoch
			assert.Equal(t, uint64(want), uint64(len(retrieved)))
		})
	}
}

func TestStore_Blocks_Retrieve_SlotRangeWithStep(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, 500)
			for i := 0; i < 500; i++ {
				b, err := tt.newBlock(primitives.Slot(i), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				totalBlocks[i] = b
			}
			const step = 2
			ctx := t.Context()
			require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
			retrieved, _, err := db.Blocks(ctx, filters.NewFilter().SetStartSlot(100).SetEndSlot(399).SetSlotStep(step))
			require.NoError(t, err)
			assert.Equal(t, 150, len(retrieved))
			for _, b := range retrieved {
				assert.Equal(t, primitives.Slot(0), (b.Block().Slot()-100)%step, "Unexpected block slot %d", b.Block().Slot())
			}
		})
	}
}

func TestStore_SaveBlock_CanGetHighestAt(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := t.Context()

			block1, err := tt.newBlock(primitives.Slot(1), nil)
			require.NoError(t, err)
			block2, err := tt.newBlock(primitives.Slot(10), nil)
			require.NoError(t, err)
			block3, err := tt.newBlock(primitives.Slot(100), nil)
			require.NoError(t, err)

			require.NoError(t, db.SaveBlock(ctx, block1))
			require.NoError(t, db.SaveBlock(ctx, block2))
			require.NoError(t, db.SaveBlock(ctx, block3))

			_, roots, err := db.HighestRootsBelowSlot(ctx, 2)
			require.NoError(t, err)
			assert.Equal(t, false, len(roots) <= 0, "Got empty highest at slice")
			require.Equal(t, 1, len(roots))
			root := roots[0]
			b, err := db.Block(ctx, root)
			require.NoError(t, err)
			wanted := block1
			if block1.Version() >= version.Bellatrix {
				wanted, err = wanted.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)
			bPb, err := b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, bPb), "Wanted: %v, received: %v", wanted, b)

			_, roots, err = db.HighestRootsBelowSlot(ctx, 11)
			require.NoError(t, err)
			assert.Equal(t, false, len(roots) <= 0, "Got empty highest at slice")
			require.Equal(t, 1, len(roots))
			root = roots[0]
			b, err = db.Block(ctx, root)
			require.NoError(t, err)
			wanted2 := block2
			if block2.Version() >= version.Bellatrix {
				wanted2, err = block2.ToBlinded()
				require.NoError(t, err)
			}
			wanted2Pb, err := wanted2.Proto()
			require.NoError(t, err)
			bPb, err = b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wanted2Pb, bPb), "Wanted: %v, received: %v", wanted2, b)

			_, roots, err = db.HighestRootsBelowSlot(ctx, 101)
			require.NoError(t, err)
			assert.Equal(t, false, len(roots) <= 0, "Got empty highest at slice")
			require.Equal(t, 1, len(roots))
			root = roots[0]
			b, err = db.Block(ctx, root)
			require.NoError(t, err)
			wanted = block3
			if block3.Version() >= version.Bellatrix {
				wanted, err = wanted.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err = wanted.Proto()
			require.NoError(t, err)
			bPb, err = b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, bPb), "Wanted: %v, received: %v", wanted, b)
		})
	}
}

func TestStore_GenesisBlock_CanGetHighestAt(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := t.Context()

			genesisBlock, err := tt.newBlock(primitives.Slot(0), nil)
			require.NoError(t, err)
			genesisRoot, err := genesisBlock.Block().HashTreeRoot()
			require.NoError(t, err)
			require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisRoot))
			require.NoError(t, db.SaveBlock(ctx, genesisBlock))
			block1, err := tt.newBlock(primitives.Slot(1), nil)
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, block1))

			_, roots, err := db.HighestRootsBelowSlot(ctx, 2)
			require.NoError(t, err)
			require.Equal(t, 1, len(roots))
			root := roots[0]
			b, err := db.Block(ctx, root)
			require.NoError(t, err)
			wanted := block1
			if block1.Version() >= version.Bellatrix {
				wanted, err = block1.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)
			bPb, err := b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, bPb), "Wanted: %v, received: %v", wanted, b)

			_, roots, err = db.HighestRootsBelowSlot(ctx, 1)
			require.NoError(t, err)
			require.Equal(t, 1, len(roots))
			root = roots[0]
			b, err = db.Block(ctx, root)
			require.NoError(t, err)
			wanted = genesisBlock
			if genesisBlock.Version() >= version.Bellatrix {
				wanted, err = genesisBlock.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err = wanted.Proto()
			require.NoError(t, err)
			bPb, err = b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, bPb), "Wanted: %v, received: %v", wanted, b)

			_, roots, err = db.HighestRootsBelowSlot(ctx, 0)
			require.NoError(t, err)
			require.Equal(t, 1, len(roots))
			root = roots[0]
			b, err = db.Block(ctx, root)
			require.NoError(t, err)
			wanted = genesisBlock
			if genesisBlock.Version() >= version.Bellatrix {
				wanted, err = genesisBlock.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err = wanted.Proto()
			require.NoError(t, err)
			bPb, err = b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, bPb), "Wanted: %v, received: %v", wanted, b)
		})
	}
}

func TestStore_SaveBlocks_HasCachedBlocks(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := t.Context()

			b := make([]interfaces.ReadOnlySignedBeaconBlock, 500)
			for i := 0; i < 500; i++ {
				blk, err := tt.newBlock(primitives.Slot(i), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				b[i] = blk
			}

			require.NoError(t, db.SaveBlock(ctx, b[0]))
			require.NoError(t, db.SaveBlocks(ctx, b))
			f := filters.NewFilter().SetStartSlot(0).SetEndSlot(500)

			blks, _, err := db.Blocks(ctx, f)
			require.NoError(t, err)
			assert.Equal(t, 500, len(blks), "Did not get wanted blocks")
		})
	}
}

func TestStore_SaveBlocks_HasRootsMatched(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := t.Context()

			b := make([]interfaces.ReadOnlySignedBeaconBlock, 500)
			for i := 0; i < 500; i++ {
				blk, err := tt.newBlock(primitives.Slot(i), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				b[i] = blk
			}

			require.NoError(t, db.SaveBlocks(ctx, b))
			f := filters.NewFilter().SetStartSlot(0).SetEndSlot(500)

			blks, roots, err := db.Blocks(ctx, f)
			require.NoError(t, err)
			assert.Equal(t, 500, len(blks), "Did not get wanted blocks")

			for i, blk := range blks {
				rt, err := blk.Block().HashTreeRoot()
				require.NoError(t, err)
				assert.Equal(t, roots[i], rt, "mismatch of block roots")
			}
		})
	}
}

func TestStore_BlocksBySlot_BlockRootsBySlot(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := t.Context()

			b1, err := tt.newBlock(primitives.Slot(20), nil)
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, b1))
			b2, err := tt.newBlock(primitives.Slot(100), bytesutil.PadTo([]byte("parent1"), 32))
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, b2))
			b3, err := tt.newBlock(primitives.Slot(100), bytesutil.PadTo([]byte("parent2"), 32))
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, b3))

			r1, err := b1.Block().HashTreeRoot()
			require.NoError(t, err)
			r2, err := b2.Block().HashTreeRoot()
			require.NoError(t, err)
			r3, err := b3.Block().HashTreeRoot()
			require.NoError(t, err)

			retrievedBlocks, err := db.BlocksBySlot(ctx, 1)
			require.NoError(t, err)
			assert.Equal(t, 0, len(retrievedBlocks), "Unexpected number of blocks received, expected none")
			retrievedBlocks, err = db.BlocksBySlot(ctx, 20)
			require.NoError(t, err)

			wanted := b1
			if b1.Version() >= version.Bellatrix {
				wanted, err = b1.ToBlinded()
				require.NoError(t, err)
			}
			retrieved0Pb, err := retrievedBlocks[0].Proto()
			require.NoError(t, err)
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(retrieved0Pb, wantedPb), "Wanted: %v, received: %v", retrievedBlocks[0], wanted)
			assert.Equal(t, true, len(retrievedBlocks) > 0, "Expected to have blocks")
			retrievedBlocks, err = db.BlocksBySlot(ctx, 100)
			require.NoError(t, err)
			if len(retrievedBlocks) != 2 {
				t.Fatalf("Expected 2 blocks, received %d blocks", len(retrievedBlocks))
			}
			wanted = b2
			if b2.Version() >= version.Bellatrix {
				wanted, err = b2.ToBlinded()
				require.NoError(t, err)
			}
			retrieved0Pb, err = retrievedBlocks[0].Proto()
			require.NoError(t, err)
			wantedPb, err = wanted.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, retrieved0Pb), "Wanted: %v, received: %v", retrievedBlocks[0], wanted)
			wanted = b3
			if b3.Version() >= version.Bellatrix {
				wanted, err = b3.ToBlinded()
				require.NoError(t, err)
			}
			retrieved1Pb, err := retrievedBlocks[1].Proto()
			require.NoError(t, err)
			wantedPb, err = wanted.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(retrieved1Pb, wantedPb), "Wanted: %v, received: %v", retrievedBlocks[1], wanted)
			assert.Equal(t, true, len(retrievedBlocks) > 0, "Expected to have blocks")

			hasBlockRoots, retrievedBlockRoots, err := db.BlockRootsBySlot(ctx, 1)
			require.NoError(t, err)
			assert.DeepEqual(t, [][32]byte{}, retrievedBlockRoots)
			assert.Equal(t, false, hasBlockRoots, "Expected no block roots")
			hasBlockRoots, retrievedBlockRoots, err = db.BlockRootsBySlot(ctx, 20)
			require.NoError(t, err)
			assert.DeepEqual(t, [][32]byte{r1}, retrievedBlockRoots)
			assert.Equal(t, true, hasBlockRoots, "Expected no block roots")
			hasBlockRoots, retrievedBlockRoots, err = db.BlockRootsBySlot(ctx, 100)
			require.NoError(t, err)
			assert.DeepEqual(t, [][32]byte{r2, r3}, retrievedBlockRoots)
			assert.Equal(t, true, hasBlockRoots, "Expected no block roots")
		})
	}
}

func TestStore_FeeRecipientByValidatorID(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	ids := []primitives.ValidatorIndex{0, 0, 0}
	feeRecipients := []common.Address{{}, {}, {}, {}}
	require.ErrorContains(t, "validatorIDs and feeRecipients must be the same length", db.SaveFeeRecipientsByValidatorIDs(ctx, ids, feeRecipients))

	ids = []primitives.ValidatorIndex{0, 1, 2}
	feeRecipients = []common.Address{{'a'}, {'b'}, {'c'}}
	require.NoError(t, db.SaveFeeRecipientsByValidatorIDs(ctx, ids, feeRecipients))
	f, err := db.FeeRecipientByValidatorID(ctx, 0)
	require.NoError(t, err)
	require.Equal(t, common.Address{'a'}, f)
	f, err = db.FeeRecipientByValidatorID(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, common.Address{'b'}, f)
	f, err = db.FeeRecipientByValidatorID(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, common.Address{'c'}, f)
	_, err = db.FeeRecipientByValidatorID(ctx, 3)
	want := errors.Wrap(ErrNotFoundFeeRecipient, "validator id 3")
	require.Equal(t, want.Error(), err.Error())

	regs := []*ethpb.ValidatorRegistrationV1{
		{
			FeeRecipient: bytesutil.PadTo([]byte("a"), 20),
			GasLimit:     1,
			Timestamp:    2,
			Pubkey:       bytesutil.PadTo([]byte("b"), 48),
		}}
	require.NoError(t, db.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{3}, regs))
	f, err = db.FeeRecipientByValidatorID(ctx, 3)
	require.NoError(t, err)
	require.Equal(t, common.Address{'a'}, f)

	_, err = db.FeeRecipientByValidatorID(ctx, 4)
	want = errors.Wrap(ErrNotFoundFeeRecipient, "validator id 4")
	require.Equal(t, want.Error(), err.Error())
}

func TestStore_RegistrationsByValidatorID(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	ids := []primitives.ValidatorIndex{0, 0, 0}
	regs := []*ethpb.ValidatorRegistrationV1{{}, {}, {}, {}}
	require.ErrorContains(t, "ids and registrations must be the same length", db.SaveRegistrationsByValidatorIDs(ctx, ids, regs))
	timestamp := time.Now().Unix()
	ids = []primitives.ValidatorIndex{0, 1, 2}
	regs = []*ethpb.ValidatorRegistrationV1{
		{
			FeeRecipient: bytesutil.PadTo([]byte("a"), 20),
			GasLimit:     1,
			Timestamp:    uint64(timestamp),
			Pubkey:       bytesutil.PadTo([]byte("b"), 48),
		},
		{
			FeeRecipient: bytesutil.PadTo([]byte("c"), 20),
			GasLimit:     3,
			Timestamp:    uint64(timestamp),
			Pubkey:       bytesutil.PadTo([]byte("d"), 48),
		},
		{
			FeeRecipient: bytesutil.PadTo([]byte("e"), 20),
			GasLimit:     5,
			Timestamp:    uint64(timestamp),
			Pubkey:       bytesutil.PadTo([]byte("f"), 48),
		},
	}
	require.NoError(t, db.SaveRegistrationsByValidatorIDs(ctx, ids, regs))
	f, err := db.RegistrationByValidatorID(ctx, 0)
	require.NoError(t, err)
	require.DeepEqual(t, &ethpb.ValidatorRegistrationV1{
		FeeRecipient: bytesutil.PadTo([]byte("a"), 20),
		GasLimit:     1,
		Timestamp:    uint64(timestamp),
		Pubkey:       bytesutil.PadTo([]byte("b"), 48),
	}, f)
	f, err = db.RegistrationByValidatorID(ctx, 1)
	require.NoError(t, err)
	require.DeepEqual(t, &ethpb.ValidatorRegistrationV1{
		FeeRecipient: bytesutil.PadTo([]byte("c"), 20),
		GasLimit:     3,
		Timestamp:    uint64(timestamp),
		Pubkey:       bytesutil.PadTo([]byte("d"), 48),
	}, f)
	f, err = db.RegistrationByValidatorID(ctx, 2)
	require.NoError(t, err)
	require.DeepEqual(t, &ethpb.ValidatorRegistrationV1{
		FeeRecipient: bytesutil.PadTo([]byte("e"), 20),
		GasLimit:     5,
		Timestamp:    uint64(timestamp),
		Pubkey:       bytesutil.PadTo([]byte("f"), 48),
	}, f)
	_, err = db.RegistrationByValidatorID(ctx, 3)
	want := errors.Wrap(ErrNotFoundFeeRecipient, "validator id 3")
	require.Equal(t, want.Error(), err.Error())
}

// Block creates a phase0 beacon block at the specified slot and saves it to the database.
func createAndSaveBlock(t *testing.T, ctx context.Context, db *Store, slot primitives.Slot) {
	block := util.NewBeaconBlock()
	block.Block.Slot = slot

	wrappedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wrappedBlock))
}

func TestStore_EarliestSlot(t *testing.T) {
	ctx := t.Context()

	t.Run("empty database returns ErrNotFound", func(t *testing.T) {
		db := setupDB(t)

		slot, err := db.EarliestSlot(ctx)
		require.ErrorIs(t, err, ErrNotFound)
		assert.Equal(t, primitives.Slot(0), slot)
	})

	t.Run("database with only genesis block", func(t *testing.T) {
		db := setupDB(t)

		// Create and save genesis block (slot 0)
		createAndSaveBlock(t, ctx, db, 0)

		slot, err := db.EarliestSlot(ctx)
		require.NoError(t, err)
		assert.Equal(t, primitives.Slot(0), slot)
	})

	t.Run("database with genesis and blocks in genesis epoch", func(t *testing.T) {
		db := setupDB(t)
		slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

		// Create and save genesis block (slot 0)
		createAndSaveBlock(t, ctx, db, 0)

		// Create and save a block in the genesis epoch
		createAndSaveBlock(t, ctx, db, primitives.Slot(slotsPerEpoch-1))

		slot, err := db.EarliestSlot(ctx)
		require.NoError(t, err)
		assert.Equal(t, primitives.Slot(0), slot)
	})

	t.Run("database with genesis and blocks beyond genesis epoch", func(t *testing.T) {
		db := setupDB(t)
		slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

		// Create and save genesis block (slot 0)
		createAndSaveBlock(t, ctx, db, 0)

		// Create and save a block beyond the genesis epoch
		nextEpochSlot := primitives.Slot(slotsPerEpoch)
		createAndSaveBlock(t, ctx, db, nextEpochSlot)

		slot, err := db.EarliestSlot(ctx)
		require.NoError(t, err)
		assert.Equal(t, nextEpochSlot, slot)
	})

	t.Run("database starting from checkpoint (non-zero earliest slot)", func(t *testing.T) {
		db := setupDB(t)
		slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

		// Simulate starting from a checkpoint by creating blocks starting from a later slot
		checkpointSlot := primitives.Slot(slotsPerEpoch * 10) // 10 epochs later
		nextEpochSlot := checkpointSlot + slotsPerEpoch

		// Create and save first block at checkpoint slot
		createAndSaveBlock(t, ctx, db, checkpointSlot)

		// Create and save another block in the next epoch
		createAndSaveBlock(t, ctx, db, nextEpochSlot)

		slot, err := db.EarliestSlot(ctx)
		require.NoError(t, err)
		assert.Equal(t, nextEpochSlot, slot)
	})
}
