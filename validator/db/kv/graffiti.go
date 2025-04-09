package kv

import (
	"bytes"
	"context"

	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	bolt "go.etcd.io/bbolt"
)

// SaveGraffitiOrderedIndex writes the current graffiti index to the db
func (s *Store) SaveGraffitiOrderedIndex(_ context.Context, index uint64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(graffitiBucket)
		indexBytes := bytesutil.Uint64ToBytesBigEndian(index)
		return bkt.Put(graffitiOrderedIndexKey, indexBytes)
	})
}

// GraffitiOrderedIndex fetches the ordered index, resetting if the file hash changed
func (s *Store) GraffitiOrderedIndex(_ context.Context, fileHash [32]byte) (uint64, error) {
	orderedIndex := uint64(0)
	err := s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(graffitiBucket)
		dbFileHash := bkt.Get(graffitiFileHashKey)
		if bytes.Equal(dbFileHash, fileHash[:]) {
			indexBytes := bkt.Get(graffitiOrderedIndexKey)
			orderedIndex = bytesutil.BytesToUint64BigEndian(indexBytes)
		} else {
			indexBytes := bytesutil.Uint64ToBytesBigEndian(0)
			if err := bkt.Put(graffitiOrderedIndexKey, indexBytes); err != nil {
				return err
			}
			return bkt.Put(graffitiFileHashKey, fileHash[:])
		}
		return nil
	})
	return orderedIndex, err
}

// GraffitiFileHash fetches the graffiti file hash.
func (s *Store) GraffitiFileHash() ([32]byte, bool, error) {
	// Define a default file hash.
	var fileHash [32]byte

	exists := false

	err := s.db.View(func(tx *bolt.Tx) error {
		// Get the graffiti bucket.
		bkt := tx.Bucket(graffitiBucket)

		// Get the file hash.
		dbFileHash := bkt.Get(graffitiFileHashKey)

		if dbFileHash == nil {
			// If the file hash is nil, return early.
			return nil
		}

		// A DB file hash exists.
		exists = true
		copy(fileHash[:], dbFileHash)
		return nil
	})

	// Return the file hash.
	return fileHash, exists, err
}
