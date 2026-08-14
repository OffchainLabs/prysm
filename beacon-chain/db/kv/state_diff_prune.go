package kv

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/math"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	bolt "go.etcd.io/bbolt"
)

// LastStateDiffBoundary returns the highest state-diff tree boundary at or before the given slot.
// The state stored there is the closest one a state after it can be replayed from, so keeping the
// blocks above that boundary keeps every state after it rebuildable.
// The slot is returned unchanged when state diff is disabled, or when the tree does not reach it.
func (s *Store) LastStateDiffBoundary(slot primitives.Slot) (primitives.Slot, error) {
	if !features.Get().EnableStateDiff {
		return slot, nil
	}

	hasOffset, err := s.hasStateDiffOffset()
	if err != nil {
		return 0, fmt.Errorf("has state diff offset: %w", err)
	}
	if !hasOffset {
		return slot, nil
	}

	offset, err := s.loadOffset()
	if err != nil {
		return 0, fmt.Errorf("load offset: %w", err)
	}
	if uint64(slot) <= offset {
		return slot, nil
	}

	// The last kept slot is the one of the finest level, hence the closest boundary to the slot.
	keptSlots := stateDiffSlotsToKeep(offset, uint64(slot))

	return primitives.Slot(keptSlots[len(keptSlots)-1]), nil
}

// DeleteStateDiffBeforeSlot deletes at most maxEntries state-diff entries that are not needed any
// more to rebuild the states at or after the given slot, and returns the number of deleted keys.
// Zero means there was nothing left to do. Call it repeatedly until it returns zero.
func (s *Store) DeleteStateDiffBeforeSlot(ctx context.Context, cutoffSlot primitives.Slot, maxEntries int) (int, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.DeleteStateDiffBeforeSlot")
	defer span.End()

	if !features.Get().EnableStateDiff {
		return 0, nil
	}

	if maxEntries <= 0 {
		return 0, fmt.Errorf("maximum number of entries to delete must be positive, got %d", maxEntries)
	}

	hasOffset, err := s.hasStateDiffOffset()
	if err != nil {
		return 0, fmt.Errorf("has state diff offset: %w", err)
	}
	if !hasOffset {
		return 0, nil
	}

	offset, err := s.loadOffset()
	if err != nil {
		return 0, fmt.Errorf("load offset: %w", err)
	}

	cutoff := uint64(cutoffSlot)
	if cutoff <= offset {
		return 0, nil
	}

	keptSlots := stateDiffSlotsToKeep(offset, cutoff)

	// The tree is anchored on the kept level 0 entry from now on.
	// Refuse to prune a tree that cannot be re-anchored, rather than making it unreadable.
	newOffset := keptSlots[0]
	key := makeKeyForStateDiffTree(0, newOffset)
	hasNewAnchor, err := s.hasStateDiffKey(key)
	if err != nil {
		return 0, fmt.Errorf("has state diff key: %w", err)
	}
	if !hasNewAnchor {
		return 0, fmt.Errorf("%w: no level 0 snapshot at slot %d to re-anchor the tree on", ErrStateDiffCorrupted, newOffset)
	}

	kept := make(map[uint64]bool, len(keptSlots))
	for _, slot := range keptSlots {
		kept[slot] = true
	}

	// The current anchor snapshot is deleted last: the tree is read with the current offset until
	// the very last call, and needs it.
	currentAnchorKey := makeKeyForStateDiffTree(0, offset)

	keys, err := s.stateDiffKeysBefore(ctx, cutoff, kept, currentAnchorKey, maxEntries)
	if err != nil {
		return 0, fmt.Errorf("state diff keys before: %w", err)
	}

	if len(keys) > 0 {
		if err := s.deleteStateDiffKeys(keys); err != nil {
			return 0, fmt.Errorf("delete state diff keys: %w", err)
		}

		return len(keys), nil
	}

	// Everything that is not needed any more is gone: re-anchor the tree.
	if newOffset == offset {
		return 0, nil
	}

	count, err := s.reanchorStateDiff(currentAnchorKey, newOffset)
	if err != nil {
		return 0, fmt.Errorf("re-anchor state diff: %w", err)
	}

	return count, nil
}

// stateDiffSlotsToKeep returns, for every level of the tree, the slot of the last entry at or
// before the given slot. Those are the only entries below it that a state at or after it needs.
// The exponents are validated at node startup, and the callers check the slot against the offset.
func stateDiffSlotsToKeep(offset, slot uint64) []uint64 {
	exponents := flags.Get().StateDiffExponents
	relativeSlot := slot - offset
	keptSlots := make([]uint64, 0, len(exponents))

	for _, exponent := range exponents {
		span := math.PowerOf2(uint64(exponent))
		keptSlots = append(keptSlots, offset+relativeSlot/span*span)
	}

	return keptSlots
}

// hasStateDiffKey reports whether the given key is present in the state-diff bucket.
func (s *Store) hasStateDiffKey(key []byte) (bool, error) {
	var has bool
	if err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}

		has = bucket.Get(key) != nil

		return nil
	}); err != nil {
		return false, err
	}

	return has, nil
}

// stateDiffKeysBefore collects the keys of at most maxEntries tree entries stored before the given
// slot, skipping the entries stored at the kept slots and the given key.
func (s *Store) stateDiffKeysBefore(ctx context.Context, slot uint64, kept map[uint64]bool, skipKey []byte, maxEntries int) ([][]byte, error) {
	var (
		keys        [][]byte
		entries     int
		entryPrefix []byte
	)

	if err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}

		cursor := bucket.Cursor()
		for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}

			// The bucket also holds metadata keys, which are never pruned.
			if !isStateDiffTreeKey(key) {
				continue
			}

			entrySlot := stateDiffTreeKeySlot(key)
			if entrySlot >= slot || kept[entrySlot] {
				continue
			}

			if bytes.Equal(key, skipKey) {
				continue
			}

			// A diff entry is split into a state, a validator and a balances key, which share the
			// same prefix and hence follow each other. Counting prefixes rather than keys keeps the
			// budget in entries, and ends the batch between two of them, never in the middle of one.
			if !bytes.Equal(entryPrefix, key[:stateDiffTreeKeyLength]) {
				if entries == maxEntries {
					return nil
				}

				entryPrefix = bytes.Clone(key[:stateDiffTreeKeyLength])
				entries++
			}

			keys = append(keys, bytes.Clone(key))
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return keys, nil
}

// deleteStateDiffKeys deletes the given keys from the state-diff bucket, in a single transaction.
func (s *Store) deleteStateDiffKeys(keys [][]byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}

		for _, key := range keys {
			if err := bucket.Delete(key); err != nil {
				return fmt.Errorf("delete state diff entry: %w", err)
			}
		}

		return nil
	})
}

// reanchorStateDiff deletes the previous anchor snapshot and moves the offset to the new anchor, in
// a single transaction, then points the in-memory cache at it.
// It returns the number of deleted keys.
func (s *Store) reanchorStateDiff(previousAnchorKey []byte, offset uint64) (int, error) {
	deleted := 0
	if err := s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}

		if bucket.Get(previousAnchorKey) != nil {
			if err := bucket.Delete(previousAnchorKey); err != nil {
				return fmt.Errorf("delete previous anchor snapshot: %w", err)
			}

			deleted = 1
		}

		offsetBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBytes, offset)

		return bucket.Put(offsetKey, offsetBytes)
	}); err != nil {
		return 0, err
	}

	if err := s.reanchorStateDiffCache(offset); err != nil {
		return deleted, fmt.Errorf("reanchor state diff cache: %w", err)
	}

	log.WithField("offset", offset).Debug("Re-anchored the pruned state-diff tree")

	return deleted, nil
}

// reanchorStateDiffCache points the in-memory cache at the new offset. The cached anchors are
// dropped, since they may belong to entries that have just been deleted.
func (s *Store) reanchorStateDiffCache(offset uint64) error {
	if s.stateDiffCache == nil {
		return nil
	}

	levelsWithData := make([]bool, len(flags.Get().StateDiffExponents))
	if err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}

		cursor := bucket.Cursor()
		for level := range levelsWithData {
			key, _ := cursor.Seek([]byte{byte(level)})
			levelsWithData[level] = key != nil && isStateDiffTreeKey(key) && key[0] == byte(level)
		}

		return nil
	}); err != nil {
		return err
	}

	s.stateDiffCache.reanchor(offset, levelsWithData)

	return nil
}
