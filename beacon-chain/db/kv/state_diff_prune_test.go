package kv

import (
	"bytes"
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	bolt "go.etcd.io/bbolt"
)

// deleteStateDiffBucket drops the whole state-diff bucket, to exercise the paths that deal with a
// database that does not have one.
func deleteStateDiffBucket(t *testing.T, db *Store) {
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket(stateDiffBucket)
	}))
}

func TestStateDiff_LastBoundary(t *testing.T) {
	t.Run("returns the slot when state diff is disabled", func(t *testing.T) {
		setStateDiffExponents([]int{7, 5})
		db := setupDB(t)

		boundary, err := db.LastStateDiffBoundary(100)
		require.NoError(t, err)
		require.Equal(t, primitives.Slot(100), boundary)
	})

	t.Run("returns the slot when the tree has not been initialized", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
		defer resetCfg()

		setStateDiffExponents([]int{7, 5})
		db := setupDB(t)

		boundary, err := db.LastStateDiffBoundary(100)
		require.NoError(t, err)
		require.Equal(t, primitives.Slot(100), boundary)
	})

	t.Run("returns the slot when the tree does not reach it", func(t *testing.T) {
		db := setupPrunableStateDiffTree(t, 96)

		boundary, err := db.LastStateDiffBoundary(0)
		require.NoError(t, err)
		require.Equal(t, primitives.Slot(0), boundary)
	})

	t.Run("returns the finest boundary at or before the slot", func(t *testing.T) {
		db := setupPrunableStateDiffTree(t, 96)

		// The finest level spans 32 slots here, and is counted from the anchor.
		for slot, want := range map[primitives.Slot]primitives.Slot{32: 32, 33: 32, 63: 32, 64: 64, 100: 96} {
			boundary, err := db.LastStateDiffBoundary(slot)
			require.NoError(t, err)
			require.Equal(t, want, boundary)
		}
	})
}

func TestStateDiff_DeleteBeforeSlot(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	t.Run("does nothing when the state diff feature is disabled", func(t *testing.T) {
		db := setupPrunableStateDiffTree(t, 384)

		disableCfg := features.InitWithReset(&features.Flags{EnableStateDiff: false})
		deleted, err := db.DeleteStateDiffBeforeSlot(t.Context(), 320, 512)
		disableCfg()

		require.NoError(t, err)
		require.Equal(t, 0, deleted)
		require.Equal(t, uint64(0), db.getOffset())
	})

	t.Run("refuses a non-positive maximum number of entries", func(t *testing.T) {
		db := setupPrunableStateDiffTree(t, 96)

		for _, maxEntries := range []int{0, -1} {
			_, err := db.DeleteStateDiffBeforeSlot(t.Context(), 96, maxEntries)
			require.ErrorContains(t, "maximum number of entries to delete must be positive", err)
		}
	})

	t.Run("does nothing when the tree has not been initialized", func(t *testing.T) {
		setStateDiffExponents([]int{7, 5})

		// A database that never went through a checkpoint or a genesis sync has no anchor, hence
		// nothing to prune.
		db := setupDB(t)

		deleted, err := db.DeleteStateDiffBeforeSlot(t.Context(), 320, 512)
		require.NoError(t, err)
		require.Equal(t, 0, deleted)
	})

	t.Run("deletes everything no retained state needs", func(t *testing.T) {
		db := setupPrunableStateDiffTree(t, 384)

		// Rebuilding a state at or after slot 320 needs the level 0 entry at slot 256 and the level
		// 1 entry at slot 320, and nothing else below the cutoff slot. A small batch size makes
		// sure this needs several calls.
		deleted := drainStateDiffPruning(t, db, 320, 2)
		require.Equal(t, true, deleted > 0)

		// The tree is now anchored on the kept level 0 entry.
		storedOffset, err := db.loadOffset()
		require.NoError(t, err)
		require.Equal(t, uint64(256), storedOffset)
		require.Equal(t, uint64(256), db.getOffset())

		// The states at or after the cutoff slot are still readable, and so is the anchor.
		for _, slot := range []primitives.Slot{256, 320, 352, 384} {
			st, err := db.stateByDiff(t.Context(), slot)
			require.NoError(t, err)
			require.Equal(t, slot, st.Slot())
		}

		// Everything else is gone: the entries below the anchor, and the ones between the anchor
		// and the cutoff slot that no retained state needs.
		for _, slot := range []primitives.Slot{0, 128, 224} {
			_, err := db.stateByDiff(t.Context(), slot)
			require.ErrorIs(t, err, ErrSlotBeforeOffset)
		}

		_, err = db.stateByDiff(t.Context(), 288)
		require.NotNil(t, err)

		// And the database still opens on the next restart.
		cache, err := populateStateDiffCacheFromDB(db, storedOffset)
		require.NoError(t, err)
		require.NoError(t, validateStateDiffCache(t.Context(), db, cache))
	})

	t.Run("keeps the metadata keys, whatever their length", func(t *testing.T) {
		db := setupPrunableStateDiffTree(t, 384)

		// Metadata keys are told apart from tree keys by their shape, not by their length, so a
		// metadata key longer than a tree key is not mistaken for an entry to prune.
		longKeys := [][]byte{
			// A word longer than a tree key.
			[]byte("a-metadata-key-longer-than-a-tree-key"),
			// One whose bytes would otherwise decode as a tree entry at slot 0, below the cutoff.
			append([]byte("m"), make([]byte, stateDiffTreeKeyLength)...),
		}

		require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
			for _, key := range longKeys {
				require.Equal(t, true, len(key) > stateDiffTreeKeyLength)
				if err := tx.Bucket(stateDiffBucket).Put(key, []byte("value")); err != nil {
					return err
				}
			}

			return nil
		}))

		require.Equal(t, true, drainStateDiffPruning(t, db, 320, 2) > 0)

		require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
			bucket := tx.Bucket(stateDiffBucket)
			for _, key := range longKeys {
				require.DeepEqual(t, []byte("value"), bucket.Get(key))
			}
			require.Equal(t, true, bucket.Get(offsetKey) != nil)
			require.Equal(t, true, bucket.Get(exponentsKey) != nil)

			return nil
		}))
	})

	t.Run("does not re-anchor the tree on a cancelled context", func(t *testing.T) {
		db := setupPrunableStateDiffTree(t, 384)

		// A scan that stops early has not proven there is nothing left to delete, so it must not
		// pass for a finished one and let the tree be re-anchored on a slot it never cleaned up to.
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := db.DeleteStateDiffBeforeSlot(ctx, 320, 512)
		require.ErrorIs(t, err, context.Canceled)

		// The tree is left untouched, and the entries below the would-be new anchor are still there.
		require.Equal(t, uint64(0), db.getOffset())
		st, err := db.stateByDiff(t.Context(), 128)
		require.NoError(t, err)
		require.Equal(t, primitives.Slot(128), st.Slot())
	})

	t.Run("never ends a batch in the middle of an entry", func(t *testing.T) {
		db := setupPrunableStateDiffTree(t, 384)

		// The budget is spent on entries rather than on keys, so a batch holds every key of the
		// entries it touches, and none of the next one.
		kept := map[uint64]bool{256: true, 320: true}
		for _, maxEntries := range []int{1, 2, 3} {
			keys, err := db.stateDiffKeysBefore(t.Context(), 320, kept, makeKeyForStateDiffTree(0, 0), maxEntries)
			require.NoError(t, err)
			require.Equal(t, true, len(keys) > 0)

			batch := make(map[string]int)
			for _, key := range keys {
				batch[string(key[:stateDiffTreeKeyLength])]++
			}
			require.Equal(t, true, len(batch) <= maxEntries)

			require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
				bucket := tx.Bucket(stateDiffBucket)
				for prefix, batched := range batch {
					stored := 0
					cursor := bucket.Cursor()
					for key, _ := cursor.Seek([]byte(prefix)); bytes.HasPrefix(key, []byte(prefix)); key, _ = cursor.Next() {
						stored++
					}

					require.Equal(t, stored, batched)
				}

				return nil
			}))
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		db := setupPrunableStateDiffTree(t, 384)

		deleted := drainStateDiffPruning(t, db, 320, 512)
		require.Equal(t, true, deleted > 0)

		// Pruning again at the same cutoff slot, or below the new offset, is a no-op.
		for _, cutoff := range []primitives.Slot{320, 256, 100} {
			deleted, err := db.DeleteStateDiffBeforeSlot(t.Context(), cutoff, 512)
			require.NoError(t, err)
			require.Equal(t, 0, deleted)
			require.Equal(t, uint64(256), db.getOffset())
		}

		st, err := db.stateByDiff(t.Context(), 352)
		require.NoError(t, err)
		require.Equal(t, primitives.Slot(352), st.Slot())
	})

	t.Run("refuses to prune a tree it cannot re-anchor", func(t *testing.T) {
		// The tree stops before the second level 0 boundary, so there is no snapshot at slot 128 to
		// re-anchor it on.
		db := setupPrunableStateDiffTree(t, 96)

		_, err := db.DeleteStateDiffBeforeSlot(t.Context(), 200, 512)
		require.ErrorIs(t, err, ErrStateDiffCorrupted)

		// The tree is left untouched.
		require.Equal(t, uint64(0), db.getOffset())
		st, err := db.stateByDiff(t.Context(), 96)
		require.NoError(t, err)
		require.Equal(t, primitives.Slot(96), st.Slot())
	})
}

func TestStateDiff_SlotsToKeep(t *testing.T) {
	t.Run("returns the last boundary of every level", func(t *testing.T) {
		setStateDiffExponents([]int{7, 5})

		// The levels span 128 and 32 slots, counted from the offset.
		require.DeepEqual(t, []uint64{256, 320}, stateDiffSlotsToKeep(0, 320))
		require.DeepEqual(t, []uint64{256, 352}, stateDiffSlotsToKeep(0, 383))
		require.DeepEqual(t, []uint64{384, 384}, stateDiffSlotsToKeep(0, 384))
	})

	t.Run("counts the boundaries from the offset", func(t *testing.T) {
		setStateDiffExponents([]int{7, 5})

		require.DeepEqual(t, []uint64{128, 128}, stateDiffSlotsToKeep(128, 128))
		require.DeepEqual(t, []uint64{256, 288}, stateDiffSlotsToKeep(128, 300))
		require.DeepEqual(t, []uint64{256, 256}, stateDiffSlotsToKeep(128, 256))
	})
}

func TestStateDiff_HasKey(t *testing.T) {
	db := setupPrunableStateDiffTree(t, 96)
	deleteStateDiffBucket(t, db)

	_, err := db.hasStateDiffKey(makeKeyForStateDiffTree(0, 0))
	require.ErrorIs(t, err, bolt.ErrBucketNotFound)
}

func TestStateDiff_KeysBefore(t *testing.T) {
	db := setupPrunableStateDiffTree(t, 96)
	deleteStateDiffBucket(t, db)

	_, err := db.stateDiffKeysBefore(t.Context(), 96, nil, nil, 512)
	require.ErrorIs(t, err, bolt.ErrBucketNotFound)
}

func TestStateDiff_DeleteKeys(t *testing.T) {
	db := setupPrunableStateDiffTree(t, 96)
	deleteStateDiffBucket(t, db)

	err := db.deleteStateDiffKeys([][]byte{makeKeyForStateDiffTree(0, 0)})
	require.ErrorIs(t, err, bolt.ErrBucketNotFound)
}

func TestStateDiff_Reanchor(t *testing.T) {
	db := setupPrunableStateDiffTree(t, 96)
	deleteStateDiffBucket(t, db)

	_, err := db.reanchorStateDiff(makeKeyForStateDiffTree(0, 0), 32)
	require.ErrorIs(t, err, bolt.ErrBucketNotFound)
}

func TestStateDiff_ReanchorCache(t *testing.T) {
	t.Run("does nothing without a cache", func(t *testing.T) {
		setStateDiffExponents([]int{7, 5})

		// The cache only exists once the tree has been initialized.
		db := setupDB(t)
		require.IsNil(t, db.stateDiffCache)
		require.NoError(t, db.reanchorStateDiffCache(32))
	})

	t.Run("errors without a state-diff bucket", func(t *testing.T) {
		db := setupPrunableStateDiffTree(t, 96)
		deleteStateDiffBucket(t, db)

		err := db.reanchorStateDiffCache(32)
		require.ErrorIs(t, err, bolt.ErrBucketNotFound)
	})
}

// setupPrunableStateDiffTree anchors a tree at slot 0 with a level 0 span of 128 slots and a level
// 1 span of 32 slots, and fills it with epoch boundary states up to (and including) the given slot.
func setupPrunableStateDiffTree(t *testing.T, upTo primitives.Slot) *Store {
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	t.Cleanup(resetCfg)

	setStateDiffExponents([]int{7, 5})

	db := setupDB(t)

	anchorState, _ := createState(t, 0, version.Fulu)
	require.NoError(t, db.initializeStateDiff(0, anchorState))

	for slot := primitives.Slot(32); slot <= upTo; slot += 32 {
		st, _ := createState(t, slot, version.Fulu)
		require.NoError(t, db.saveStateByDiff(t.Context(), st))
	}

	return db
}

// drainStateDiffPruning prunes until there is nothing left to delete, as the pruner service does,
// and returns the total number of deleted keys.
func drainStateDiffPruning(t *testing.T, db *Store, cutoffSlot primitives.Slot, maxEntries int) int {
	total := 0
	for {
		deleted, err := db.DeleteStateDiffBeforeSlot(t.Context(), cutoffSlot, maxEntries)
		require.NoError(t, err)
		if deleted == 0 {
			return total
		}
		total += deleted
	}
}
