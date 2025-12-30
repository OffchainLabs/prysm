//go:build testing

package kv

import (
	"encoding/binary"

	"go.etcd.io/bbolt"
)

// InitStateDiffCacheForTesting initializes the state diff cache with the given offset.
// This is intended for testing purposes when setting up state diff after database creation.
// This file is only compiled when the "testing" build tag is set.
func (s *Store) InitStateDiffCacheForTesting(offset uint64) error {
	// First, set the offset in the database.
	err := s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}

		offsetBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBytes, offset)
		return bucket.Put([]byte("offset"), offsetBytes)
	})
	if err != nil {
		return err
	}

	// Then create the state diff cache.
	sdCache, err := newStateDiffCache(s)
	if err != nil {
		return err
	}
	s.stateDiffCache = sdCache
	return nil
}
