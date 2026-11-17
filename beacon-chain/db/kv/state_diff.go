package kv

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/consensus-types/hdiff"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	logrus "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

const (
	stateSuffix     = "_s"
	validatorSuffix = "_v"
	balancesSuffix  = "_b"
)

var errSnapshotNotFound = errors.New("full snapshot not found")

/*
	We use a level-based approach to save state diffs. Each level corresponds to an exponent of 2 (exponents[lvl]).
	The data at level 0 is saved every 2**exponent[0] slots and always contains a full state snapshot that is used as a base for the delta saved at other levels.
*/

// SlotInDiffTree returns whether the given slot is a saving point in the diff tree.
// If it is, it also returns the offset and level in the tree.
func (s *Store) SlotInDiffTree(slot primitives.Slot) (uint64, int, error) {
	offset := s.getOffset()
	if uint64(slot) < offset {
		return 0, -1, ErrSlotBeforeOffset
	}
	return offset, computeLevel(offset, slot), nil
}

// saveStateByDiff takes a state and decides between saving a full state snapshot or a diff.
func (s *Store) saveStateByDiff(ctx context.Context, st state.ReadOnlyBeaconState) (err error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.saveStateByDiff")
	defer span.End()
	defer func() { recordStateDiffError("save", err) }()

	if st == nil {
		return errors.New("state is nil")
	}

	slot := st.Slot()
	offset, lvl, err := s.SlotInDiffTree(slot)
	if err != nil {
		return errors.Wrap(err, "could not determine if slot is in diff tree")
	}
	if lvl == -1 {
		return nil
	}

	// While an archive walk is filling the tree, only the walk may write: above its frontier there is no anchor.
	if frontier, pending := s.archivePending(); pending && uint64(slot) > uint64(frontier)+deepestDiffSpan() {
		return errors.Wrapf(ErrAboveArchiveFrontier, "slot %d is above the archive frontier %d", slot, frontier)
	}

	start := time.Now()
	label := levelLabel(lvl)
	// Save full state if level is 0.
	if lvl == 0 {
		err = s.saveFullSnapshot(st)
	} else {
		// Get anchor state to compute the diff from.
		anchorStart := time.Now()
		var anchorState state.ReadOnlyBeaconState
		anchorState, err = s.getAnchorState(ctx, offset, lvl, slot)
		if err == nil {
			stateDiffSavePhaseTime.WithLabelValues(label, "anchor").Observe(msSince(anchorStart))
			err = s.saveHdiff(lvl, anchorState, st)
		}
	}
	if err == nil {
		stateDiffSaveTime.WithLabelValues(label).Observe(msSince(start))
	}
	return err
}

// StateBySlotFromDiffTree returns the state at the given slot from the tree; slot must be a saving point.
func (s *Store) StateBySlotFromDiffTree(ctx context.Context, slot primitives.Slot) (state.BeaconState, error) {
	st, err := s.stateByDiff(ctx, slot)
	if errors.Is(err, errSnapshotNotFound) {
		return nil, fmt.Errorf("%w: %w", ErrNotFoundState, err)
	}
	return st, err
}

// stateByDiff retrieves the full state for a given slot.
func (s *Store) stateByDiff(ctx context.Context, slot primitives.Slot) (_ state.BeaconState, err error) {
	start := time.Now()
	defer func() { recordStateDiffError("read", err) }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	offset := s.getOffset()
	if uint64(slot) < offset {
		return nil, ErrSlotBeforeOffset
	}

	snapshot, diffChain, err := s.getBaseAndDiffChain(offset, slot)
	if err != nil {
		return nil, err
	}

	for _, diff := range diffChain {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		applyStart := time.Now()
		snapshot, err = hdiff.ApplyDiff(ctx, snapshot, diff)
		if err != nil {
			return nil, err
		}
		stateDiffApplyTime.Observe(msSince(applyStart))
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	level := computeLevel(offset, slot)
	label := levelLabel(level)
	duration := msSince(start)
	stateDiffReadTime.WithLabelValues(label).Observe(duration)
	stateDiffReadDiffsApplied.WithLabelValues(label).Add(float64(len(diffChain)))
	log.WithFields(logrus.Fields{
		"slot": slot, "treeLevel": level, "diffsApplied": len(diffChain), "durationMs": duration,
	}).Debug("Read state from state-diff tree")
	return snapshot, nil
}

// saveHdiff computes the diff between the anchor state and the current state and saves it to the database.
// This function needs to be called only with the latest finalized state, and in a strictly increasing slot order.
func (s *Store) saveHdiff(lvl int, anchor, st state.ReadOnlyBeaconState) error {
	slot := uint64(st.Slot())
	key := makeKeyForStateDiffTree(lvl, slot)
	label := levelLabel(lvl)

	computeStart := time.Now()
	diff, err := hdiff.Diff(anchor, st)
	if err != nil {
		return err
	}
	computeMs := msSince(computeStart)
	stateDiffSavePhaseTime.WithLabelValues(label, "diff").Observe(computeMs)

	var overwrite, same bool
	writeStart := time.Now()
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		overwrite, same, err = putTreeEntry(bucket,
			treeEntry{key: append(append([]byte{}, key...), stateSuffix...), value: diff.StateDiff},
			treeEntry{key: append(append([]byte{}, key...), validatorSuffix...), value: diff.ValidatorDiffs},
			treeEntry{key: append(append([]byte{}, key...), balancesSuffix...), value: diff.BalancesDiff})
		return err
	})
	if err != nil {
		return err
	}
	writeMs := msSince(writeStart)
	stateDiffSavePhaseTime.WithLabelValues(label, "write").Observe(writeMs)
	total := recordTreeWrite(lvl, slot, map[string]int{
		"state": len(diff.StateDiff), "validators": len(diff.ValidatorDiffs), "balances": len(diff.BalancesDiff),
	}, overwrite, same)

	// Save the full state to the cache (if not the last level).
	cacheStart := time.Now()
	if lvl != len(flags.Get().StateDiffExponents)-1 {
		err = s.stateDiffCache.setAnchor(lvl, st)
		if err != nil {
			return err
		}
	}
	if err := s.stateDiffCache.setLevelHasData(lvl); err != nil {
		return err
	}
	cacheMs := msSince(cacheStart)
	stateDiffSavePhaseTime.WithLabelValues(label, "cache").Observe(cacheMs)
	log.WithFields(logrus.Fields{
		"slot": slot, "treeLevel": lvl, "kind": "diff", "totalBytes": total,
		"stateBytes": len(diff.StateDiff), "validatorBytes": len(diff.ValidatorDiffs),
		"balancesBytes": len(diff.BalancesDiff), "computeMs": computeMs,
		"writeMs": writeMs, "cacheMs": cacheMs, "overwrite": overwrite, "sameBytes": same,
	}).Debug("Saved state-diff tree entry")

	return nil
}

// SaveFullSnapshot saves the full level 0 state snapshot to the database.
func (s *Store) saveFullSnapshot(st state.ReadOnlyBeaconState) error {
	slot := uint64(st.Slot())
	key := makeKeyForStateDiffTree(0, slot)
	label := levelLabel(0)
	encodeStart := time.Now()
	compressed, err := encodeStateWithKey(st)
	if err != nil {
		return fmt.Errorf("encode state with key: %w", err)
	}
	encodeMs := msSince(encodeStart)
	stateDiffSavePhaseTime.WithLabelValues(label, "encode").Observe(encodeMs)

	var overwrite, same bool
	writeStart := time.Now()
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}

		overwrite, same, err = putTreeEntry(bucket, treeEntry{key: key, value: compressed})
		return err
	})
	if err != nil {
		return err
	}
	writeMs := msSince(writeStart)
	stateDiffSavePhaseTime.WithLabelValues(label, "write").Observe(writeMs)
	total := recordTreeWrite(0, slot, map[string]int{"snapshot": len(compressed)}, overwrite, same)
	// Save the full state to the cache, and invalidate other levels.
	cacheStart := time.Now()
	s.stateDiffCache.clearAnchors()
	if len(flags.Get().StateDiffExponents) > 1 {
		if err = s.stateDiffCache.setAnchor(0, st); err != nil {
			return err
		}
	}
	if err := s.stateDiffCache.setLevelHasData(0); err != nil {
		return err
	}
	cacheMs := msSince(cacheStart)
	stateDiffSavePhaseTime.WithLabelValues(label, "cache").Observe(cacheMs)
	log.WithFields(logrus.Fields{
		"slot": slot, "treeLevel": 0, "kind": "snapshot", "totalBytes": total,
		"snapshotBytes": len(compressed), "computeMs": encodeMs, "writeMs": writeMs,
		"cacheMs": cacheMs, "overwrite": overwrite, "sameBytes": same,
	}).Debug("Saved state-diff tree entry")

	return nil
}

func (s *Store) getDiff(lvl int, slot uint64) (hdiff.HdiffBytes, error) {
	key := makeKeyForStateDiffTree(lvl, slot)
	var stateDiff []byte
	var validatorDiff []byte
	var balancesDiff []byte

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		buf := append(key, stateSuffix...)
		rawStateDiff := bucket.Get(buf)
		// Missing history permits replay, but present corrupt records must still fail.
		if rawStateDiff == nil {
			return errors.Wrapf(ErrNotFoundState, "state diff not found at level %d slot %d", lvl, slot)
		}
		if len(rawStateDiff) == 0 {
			return errors.Wrapf(ErrStateDiffCorrupted, "empty state diff at level %d slot %d", lvl, slot)
		}
		stateDiff = slices.Clone(rawStateDiff)
		buf = append(key, validatorSuffix...)
		rawValidatorDiff := bucket.Get(buf)
		if rawValidatorDiff == nil {
			return errors.Wrapf(ErrNotFoundState, "validator diff not found at level %d slot %d", lvl, slot)
		}
		if len(rawValidatorDiff) == 0 {
			return errors.Wrapf(ErrStateDiffCorrupted, "empty validator diff at level %d slot %d", lvl, slot)
		}
		validatorDiff = slices.Clone(rawValidatorDiff)
		buf = append(key, balancesSuffix...)
		rawBalancesDiff := bucket.Get(buf)
		if rawBalancesDiff == nil {
			return errors.Wrapf(ErrNotFoundState, "balances diff not found at level %d slot %d", lvl, slot)
		}
		if len(rawBalancesDiff) == 0 {
			return errors.Wrapf(ErrStateDiffCorrupted, "empty balances diff at level %d slot %d", lvl, slot)
		}
		balancesDiff = slices.Clone(rawBalancesDiff)
		return nil
	})

	if err != nil {
		return hdiff.HdiffBytes{}, err
	}

	return hdiff.HdiffBytes{
		StateDiff:      stateDiff,
		ValidatorDiffs: validatorDiff,
		BalancesDiff:   balancesDiff,
	}, nil
}

func (s *Store) getFullSnapshot(slot uint64) (state.BeaconState, error) {
	if s.stateDiffCache != nil {
		if anchor := s.stateDiffCache.getAnchor(0, withExactSlot(primitives.Slot(slot))); anchor != nil {
			stateDiffReadBase.WithLabelValues("snapshot_cache").Inc()
			return anchor, nil
		}
	}

	start := time.Now()
	key := makeKeyForStateDiffTree(0, slot)
	var compressed []byte

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		rawEnc := bucket.Get(key)
		if rawEnc == nil {
			return errors.Wrapf(errSnapshotNotFound, "level 0 slot %d", slot)
		}
		compressed = slices.Clone(rawEnc)
		return nil
	})
	if err != nil {
		return nil, err
	}

	enc, err := snappy.Decode(nil, compressed)
	if err != nil {
		return nil, err
	}

	snapshot, err := decodeStateSnapshot(enc)
	if err != nil {
		return nil, err
	}
	stateDiffSnapshotLoadTime.Observe(msSince(start))
	stateDiffReadBase.WithLabelValues("snapshot_db").Inc()
	return snapshot, nil
}
