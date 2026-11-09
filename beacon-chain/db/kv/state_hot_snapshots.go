package kv

import (
	"context"
	"fmt"
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

	compressedEnc, err := encodeHotStateSnapshot(st)
	if err != nil {
		return err
	}

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

// DeleteHotStateSnapshots removes the given roots from the hot state snapshots bucket.
func (s *Store) DeleteHotStateSnapshots(ctx context.Context, blockRoots [][32]byte) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.DeleteHotStateSnapshots")
	defer span.End()

	if len(blockRoots) == 0 {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(hotStateSnapshotsBucket)
		if bkt == nil {
			return bolt.ErrBucketNotFound
		}
		for _, r := range blockRoots {
			if err := bkt.Delete(r[:]); err != nil {
				return err
			}
		}
		return nil
	})
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

func encodeHotStateSnapshot(st state.ReadOnlyBeaconState) ([]byte, error) {
	if st == nil || st.IsNil() {
		return nil, errors.New("nil state")
	}
	enc, err := encodeStateWithKey(st)
	if err != nil {
		return nil, fmt.Errorf("encode state with key: %w", err)
	}
	return enc, nil
}

func (s *Store) SaveArchiveResumeSnapshot(ctx context.Context, st state.ReadOnlyBeaconState, blockRoot [32]byte) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveArchiveResumeSnapshot")
	defer span.End()

	compressedEnc, err := encodeHotStateSnapshot(st)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		hot := tx.Bucket(hotStateSnapshotsBucket)
		tracked := tx.Bucket(archiveResumeSnapshotsBucket)
		if hot == nil || tracked == nil {
			return bolt.ErrBucketNotFound
		}
		if hot.Get(blockRoot[:]) != nil && tracked.Get(blockRoot[:]) == nil {
			return nil
		}
		if err := hot.Put(blockRoot[:], compressedEnc); err != nil {
			return err
		}
		return tracked.Put(blockRoot[:], []byte{1})
	})
}

func (s *Store) ArchiveResumeSnapshotRoots(ctx context.Context) ([][32]byte, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.ArchiveResumeSnapshotRoots")
	defer span.End()

	var roots [][32]byte
	err := s.db.View(func(tx *bolt.Tx) error {
		tracked := tx.Bucket(archiveResumeSnapshotsBucket)
		if tracked == nil {
			return bolt.ErrBucketNotFound
		}
		return tracked.ForEach(func(k, _ []byte) error {
			var r [32]byte
			copy(r[:], k)
			roots = append(roots, r)
			return nil
		})
	})
	return roots, err
}

func (s *Store) DeleteArchiveResumeSnapshots(ctx context.Context, blockRoots [][32]byte) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.DeleteArchiveResumeSnapshots")
	defer span.End()

	if len(blockRoots) == 0 {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		hot := tx.Bucket(hotStateSnapshotsBucket)
		tracked := tx.Bucket(archiveResumeSnapshotsBucket)
		if hot == nil || tracked == nil {
			return bolt.ErrBucketNotFound
		}
		for _, r := range blockRoots {
			if tracked.Get(r[:]) == nil {
				continue
			}
			if err := hot.Delete(r[:]); err != nil {
				return err
			}
			if err := tracked.Delete(r[:]); err != nil {
				return err
			}
		}
		return nil
	})
}
