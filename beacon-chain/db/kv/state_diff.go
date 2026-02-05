package kv

import (
	"context"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/consensus-types/hdiff"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

const (
	stateSuffix     = "_s"
	validatorSuffix = "_v"
	balancesSuffix  = "_b"
)

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
func (s *Store) saveStateByDiff(ctx context.Context, st state.ReadOnlyBeaconState) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.saveStateByDiff")
	defer span.End()

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

	// Save full state if level is 0.
	if lvl == 0 {
		return s.saveFullSnapshot(st)
	}

	// Get anchor state to compute the diff from.
	anchorState, err := s.getAnchorState(offset, lvl, slot)
	if err != nil {
		return err
	}

	return s.saveHdiff(lvl, anchorState, st)
}

// stateByDiff retrieves the full state for a given slot.
func (s *Store) stateByDiff(ctx context.Context, slot primitives.Slot) (state.BeaconState, error) {
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

		snapshot, err = hdiff.ApplyDiff(ctx, snapshot, diff)
		if err != nil {
			return nil, err
		}
	}

	return snapshot, nil
}

// saveHdiff computes the diff between the anchor state and the current state and saves it to the database.
// This function needs to be called only with the latest finalized state, and in a strictly increasing slot order.
func (s *Store) saveHdiff(lvl int, anchor, st state.ReadOnlyBeaconState) error {
	slot := uint64(st.Slot())
	key := makeKeyForStateDiffTree(lvl, slot)

	diff, err := hdiff.Diff(anchor, st)
	if err != nil {
		return err
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		buf := append(key, stateSuffix...)
		if err := bucket.Put(buf, diff.StateDiff); err != nil {
			return err
		}
		buf = append(key, validatorSuffix...)
		if err := bucket.Put(buf, diff.ValidatorDiffs); err != nil {
			return err
		}
		buf = append(key, balancesSuffix...)
		if err := bucket.Put(buf, diff.BalancesDiff); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Save the full state to the cache (if not the last level).
	if lvl != len(flags.Get().StateDiffExponents)-1 {
		err = s.stateDiffCache.setAnchor(lvl, st)
		if err != nil {
			return err
		}
	}

	return nil
}

// SaveFullSnapshot saves the full level 0 state snapshot to the database.
func (s *Store) saveFullSnapshot(st state.ReadOnlyBeaconState) error {
	slot := uint64(st.Slot())
	key := makeKeyForStateDiffTree(0, slot)
	stateBytes, err := st.MarshalSSZ()
	if err != nil {
		return err
	}
	// add version key to value
	enc, err := addKey(st.Version(), stateBytes)
	if err != nil {
		return err
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}

		if err := bucket.Put(key, enc); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	// Save the full state to the cache, and invalidate other levels.
	s.stateDiffCache.clearAnchors()
	err = s.stateDiffCache.setAnchor(0, st)
	if err != nil {
		return err
	}

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
		if len(rawStateDiff) == 0 {
			return errors.New("state diff not found")
		}
		stateDiff = slices.Clone(rawStateDiff)
		buf = append(key, validatorSuffix...)
		rawValidatorDiff := bucket.Get(buf)
		if len(rawValidatorDiff) == 0 {
			return errors.New("validator diff not found")
		}
		validatorDiff = slices.Clone(rawValidatorDiff)
		buf = append(key, balancesSuffix...)
		rawBalancesDiff := bucket.Get(buf)
		if len(rawBalancesDiff) == 0 {
			return errors.New("balances diff not found")
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
	key := makeKeyForStateDiffTree(0, slot)
	var enc []byte

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		rawEnc := bucket.Get(key)
		if rawEnc == nil {
			return errors.New("state not found")
		}
		enc = slices.Clone(rawEnc)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return decodeStateSnapshot(enc)
}
