package kv

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	engine "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/trace"
)

// SaveBlindPayloadEnvelope saves a signed execution payload envelope blind in the database.
func (s *Store) SaveBlindPayloadEnvelope(ctx context.Context, signed interfaces.ROSignedExecutionPayloadEnvelope) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveBlindPayloadEnvelope")
	defer span.End()

	pb := signed.Proto()
	if pb == nil {
		return errors.New("nil payload envelope")
	}
	env, ok := pb.(*engine.SignedExecutionPayloadEnvelope)
	if !ok {
		return errors.New("invalid payload envelope")
	}

	r := env.Message.Payload.BlockHash
	blind := env.Blind()
	if blind == nil {
		return errors.New("nil blind payload envelope")
	}
	enc, err := encode(ctx, blind)
	if err != nil {
		return err
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(executionPayloadEnvelopeBucket)
		return bucket.Put(r, enc)
	})

	return err
}

// SignedBlindPayloadEnvelope retrieves a signed execution payload envelope blind from the database.
func (s *Store) SignedBlindPayloadEnvelope(ctx context.Context, blockHash []byte) (*engine.SignedBlindPayloadEnvelope, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SignedBlindPayloadEnvelope")
	defer span.End()

	env := &engine.SignedBlindPayloadEnvelope{}
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(executionPayloadEnvelopeBucket)
		enc := bkt.Get(blockHash)
		if enc == nil {
			return ErrNotFound
		}
		return decode(ctx, enc, env)
	})
	return env, err
}

func (s *Store) HasBlindPayloadEnvelope(ctx context.Context, hash [32]byte) bool {
	_, span := trace.StartSpan(ctx, "BeaconDB.HasBlock")
	defer span.End()
	if v, ok := s.blockCache.Get(string(hash[:])); v != nil && ok {
		return true
	}
	exists := false
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(executionPayloadEnvelopeBucket)
		exists = bkt.Get(hash[:]) != nil
		return nil
	}); err != nil { // This view never returns an error, but we'll handle anyway for sanity.
		panic(err)
	}
	return exists
}
