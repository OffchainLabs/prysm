package kv

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/hdiff"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

/*
	We use a level-based approach to save state diffs. The levels are 0-6, where each level corresponds to an exponent of 2 (exponents[lvl]).
	The data at level 0 is saved every 2**exponent[0] slots and always contains a full state snapshot that is used as a base for the delta saved at other levels.
*/

// saveStateByDiff takes a state and decides between saving a full state snapshot or a diff.
func (s *Store) saveStateByDiff(ctx context.Context, st state.ReadOnlyBeaconState) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.saveStateByDiff")
	defer span.End()

	if st == nil {
		return errors.New("state is nil")
	}

	slot := st.Slot()
	offset, err := s.getOffset()
	if err != nil {
		return err
	}
	if uint64(slot) < offset {
		return ErrSlotBeforeOffset
	}

	// Find the level to save the state.
	lvl := computeLevel(offset, slot)
	if lvl == -1 {
		return nil
	}

	// Save full state if level is 0.
	if lvl == 0 {
		return s.saveFullSnapshot(lvl, st)
	}

	// Get anchor state to compute the diff from.
	anchorState, err := s.getAnchorState(offset, lvl, slot)
	if err != nil {
		return err
	}

	err = s.saveHdiff(lvl, anchorState, st)
	if err != nil {
		return err
	}

	return nil
}

// stateByDiff retrieves the full state for a given slot.
func (s *Store) stateByDiff(ctx context.Context, slot primitives.Slot) (state.BeaconState, error) {
	offset, err := s.getOffset()
	if err != nil {
		return nil, err
	}
	if uint64(slot) < offset {
		return nil, ErrSlotBeforeOffset
	}

	snapshot, diffChain, err := s.getBaseAndDiffChain(offset, slot)
	if err != nil {
		return nil, err
	}

	for _, diff := range diffChain {
		snapshot, err = hdiff.ApplyDiff(ctx, snapshot, diff)
		if err != nil {
			return nil, err
		}
	}

	return snapshot, nil
}

// SaveHdiff computes the diff between the anchor state and the current state and saves it to the database.
func (s *Store) saveHdiff(lvl int, anchor, st state.ReadOnlyBeaconState) error {
	slot := uint64(st.Slot())
	key := makeKey(lvl, slot)

	diff, err := hdiff.Diff(anchor, st)
	if err != nil {
		return err
	}

	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		buf := append(key, "_s"...)
		if err := bucket.Put(buf, diff.StateDiff); err != nil {
			return err
		}
		buf = append(key, "_v"...)
		if err := bucket.Put(buf, diff.ValidatorDiffs); err != nil {
			return err
		}
		buf = append(key, "_b"...)
		if err := bucket.Put(buf, diff.BalancesDiff); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Save the full state to the cache (if not the last level).
	if lvl != len(params.StateHierarchyExponents())-1 {
		s.stateDiffCache.setAnchor(lvl, st)
	}

	return nil
}

// SaveFullSnapshot saves the full level 0 state snapshot to the database.
func (s *Store) saveFullSnapshot(lvl int, st state.ReadOnlyBeaconState) error {
	slot := uint64(st.Slot())
	key := makeKey(lvl, slot)
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
	s.stateDiffCache.setAnchor(lvl, st)

	return nil
}

func (s *Store) getDiff(lvl int, slot uint64) (hdiff.HdiffBytes, error) {
	key := makeKey(lvl, slot)
	var stateDiff []byte
	var validatorDiff []byte
	var balancesDiff []byte

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		buf := append(key, "_s"...)
		stateDiff = bucket.Get(buf)
		if stateDiff == nil {
			return errors.New("state diff not found")
		}
		buf = append(key, "_v"...)
		validatorDiff = bucket.Get(buf)
		if validatorDiff == nil {
			return errors.New("validator diff not found")
		}
		buf = append(key, "_b"...)
		balancesDiff = bucket.Get(buf)
		if balancesDiff == nil {
			return errors.New("balances diff not found")
		}
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

func (s *Store) getFullSnapshot(lvl int, slot uint64) (state.BeaconState, error) {
	key := makeKey(lvl, slot)
	var enc []byte

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		enc = bucket.Get(key)
		if enc == nil {
			return errors.New("state not found")
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.decodeStateSnapshot(enc)
}
