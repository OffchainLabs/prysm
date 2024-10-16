package kv

import (
	"context"

	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/trace"
)

// SaveBlindPayloadEnvelope saves a signed execution payload envelope blind in the database.
func (s *Store) SaveBlindPayloadEnvelope(ctx context.Context, env *ethpb.SignedBlindPayloadEnvelope) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveBlindPayloadEnvelope")
	defer span.End()

	enc, err := encode(ctx, env)
	if err != nil {
		return err
	}

	r := env.Message.BeaconBlockRoot
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(executionPayloadEnvelopeBucket)
		return bucket.Put(r, enc)
	})

	return err
}

// SignedBlindPayloadEnvelope retrieves a signed execution payload envelope blind from the database.
func (s *Store) SignedBlindPayloadEnvelope(ctx context.Context, blockRoot []byte) (*ethpb.SignedBlindPayloadEnvelope, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SignedBlindPayloadEnvelope")
	defer span.End()

	env := &ethpb.SignedBlindPayloadEnvelope{}
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(executionPayloadEnvelopeBucket)
		enc := bkt.Get(blockRoot)
		if enc == nil {
			return ErrNotFound
		}
		return decode(ctx, enc, env)
	})
	return env, err
}
