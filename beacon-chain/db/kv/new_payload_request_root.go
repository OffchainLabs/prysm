package kv

import (
	"context"
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	bolt "go.etcd.io/bbolt"
)

// NewPayloadRequestRoot returns the stored NewPayloadRequest hash tree root
// associated with the given block root.
func (s *Store) NewPayloadRequestRoot(ctx context.Context, blockRoot [fieldparams.RootLength]byte) ([fieldparams.RootLength]byte, bool, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.NewPayloadRequestRoot")
	defer span.End()

	var (
		newPayloadRequestRoot [fieldparams.RootLength]byte
		found                 bool
	)

	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(newPayloadRequestRootsBucket)
		value := bucket.Get(blockRoot[:])
		if value == nil {
			return nil
		}

		if len(value) != fieldparams.RootLength {
			return fmt.Errorf("malformed new payload request root for block root %#x: expected %d bytes, got %d", blockRoot, fieldparams.RootLength, len(value))
		}

		copy(newPayloadRequestRoot[:], value)
		found = true
		return nil
	})

	return newPayloadRequestRoot, found, err
}

// SaveNewPayloadRequestRoot stores the NewPayloadRequest hash tree root
// for the given block root.
func (s *Store) SaveNewPayloadRequestRoot(ctx context.Context, blockRoot, newPayloadRequestRoot [fieldparams.RootLength]byte) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveNewPayloadRequestRoot")
	defer span.End()

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(newPayloadRequestRootsBucket)
		return bucket.Put(blockRoot[:], newPayloadRequestRoot[:])
	})
}
