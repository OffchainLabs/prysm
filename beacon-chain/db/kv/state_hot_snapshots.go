package kv

import (
	"context"
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

// SaveHotStateSnapshot saves a hot (unfinalized) state to the hot state snapshots bucket.
// This should be only used in state diff mode in long non finalization.
func (s *Store) SaveHotStateSnapshot(ctx context.Context, st state.ReadOnlyBeaconState, blockRoot [32]byte) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveHotStateSnapshot")
	defer span.End()

	if st == nil || st.IsNil() {
		return errors.New("nil state")
	}

	stateBytes, err := st.MarshalSSZ()
	if err != nil {
		return err
	}
	enc, err := addKey(st.Version(), stateBytes)
	if err != nil {
		return err
	}
	compressedEnc := snappy.Encode(nil, enc)

	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(hotStateSnapshotsBucket)
		if bkt == nil {
			return bolt.ErrBucketNotFound
		}
		return bkt.Put(blockRoot[:], compressedEnc)
	})
}

// HotStateSnapshot returns a full state from the hot state snapshots bucket.
func (s *Store) HotStateSnapshot(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.HotStateSnapshot")
	defer span.End()

	var compressedEnc []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(hotStateSnapshotsBucket)
		if bkt == nil {
			return bolt.ErrBucketNotFound
		}
		raw := bkt.Get(blockRoot[:])
		if raw == nil {
			return ErrNotFoundState
		}
		compressedEnc = slices.Clone(raw)
		return nil
	})
	if err != nil {
		return nil, err
	}
	enc, err := snappy.Decode(nil, compressedEnc)
	if err != nil {
		return nil, err
	}
	return decodeStateSnapshot(enc)
}

// HasHotStateSnapshot checks if a state exists in the hot state snapshots bucket.
func (s *Store) HasHotStateSnapshot(ctx context.Context, blockRoot [32]byte) bool {
	_, span := trace.StartSpan(ctx, "BeaconDB.HasHotStateSnapshot")
	defer span.End()

	has := false
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(hotStateSnapshotsBucket)
		if bkt == nil {
			return bolt.ErrBucketNotFound
		}
		has = bkt.Get(blockRoot[:]) != nil
		return nil
	})
	if err != nil {
		log.WithError(err).Warn("HasHotStateSnapshot: could not check db for hot state snapshots")
		return false
	}
	return has
}

func (s *Store) ClearHotStateSnapshots(ctx context.Context) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.ClearHotStateSnapshots")
	defer span.End()

	return s.db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket(hotStateSnapshotsBucket) == nil {
			return bolt.ErrBucketNotFound
		}

		if err := tx.DeleteBucket(hotStateSnapshotsBucket); err != nil {
			return err
		}

		_, err := tx.CreateBucket(hotStateSnapshotsBucket)
		return err
	})
}
