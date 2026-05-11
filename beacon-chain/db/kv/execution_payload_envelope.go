package kv

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

// SaveExecutionPayloadEnvelope blinds and saves a signed execution payload envelope keyed by
// beacon block root. The envelope is stored in blinded form: the full execution payload is replaced
// with its block hash. The full payload can later be retrieved from the EL via
// engine_getPayloadBodiesByHash.
// A secondary index from BlockHash → BeaconBlockRoot is maintained so that
// envelopes can be looked up by execution block hash.
func (s *Store) SaveExecutionPayloadEnvelope(ctx context.Context, env *ethpb.SignedExecutionPayloadEnvelope) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveExecutionPayloadEnvelope")
	defer span.End()

	if env == nil || env.Message == nil || env.Message.Payload == nil {
		return errors.New("cannot save nil execution payload envelope")
	}

	blockRoot := bytesutil.ToBytes32(env.Message.BeaconBlockRoot)
	blockHash := bytesutil.ToBytes32(env.Message.Payload.BlockHash)
	blinded := blindEnvelope(env)

	enc, err := encodeBlindedEnvelope(blinded)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(executionPayloadEnvelopesBucket).Put(blockRoot[:], enc); err != nil {
			return err
		}
		return tx.Bucket(executionPayloadEnvelopeBlockHashBucket).Put(blockHash[:], blockRoot[:])
	})
}

// ExecutionPayloadEnvelope retrieves the blinded signed execution payload envelope by beacon block root.
func (s *Store) ExecutionPayloadEnvelope(ctx context.Context, blockRoot [32]byte) (*ethpb.SignedBlindedExecutionPayloadEnvelope, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.ExecutionPayloadEnvelope")
	defer span.End()

	var enc []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(executionPayloadEnvelopesBucket)
		enc = bkt.Get(blockRoot[:])
		return nil
	}); err != nil {
		return nil, err
	}
	if enc == nil {
		return nil, errors.Wrap(ErrNotFound, "execution payload envelope not found")
	}
	return decodeBlindedEnvelope(enc)
}

// ExecutionPayloadEnvelopeByBlockHash retrieves the blinded signed execution payload envelope
// by execution block hash. It uses the secondary BlockHash → BeaconBlockRoot index and then
// fetches the envelope from the primary bucket.
func (s *Store) ExecutionPayloadEnvelopeByBlockHash(ctx context.Context, blockHash [32]byte) (*ethpb.SignedBlindedExecutionPayloadEnvelope, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.ExecutionPayloadEnvelopeByBlockHash")
	defer span.End()

	var enc []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		blockRoot := tx.Bucket(executionPayloadEnvelopeBlockHashBucket).Get(blockHash[:])
		if blockRoot == nil {
			return nil
		}
		enc = tx.Bucket(executionPayloadEnvelopesBucket).Get(blockRoot)
		return nil
	}); err != nil {
		return nil, err
	}
	if enc == nil {
		return nil, errors.Wrap(ErrNotFound, "execution payload envelope not found for block hash")
	}
	return decodeBlindedEnvelope(enc)
}

// HasExecutionPayloadEnvelope checks whether an execution payload envelope exists for the given beacon block root.
func (s *Store) HasExecutionPayloadEnvelope(ctx context.Context, blockRoot [32]byte) bool {
	_, span := trace.StartSpan(ctx, "BeaconDB.HasExecutionPayloadEnvelope")
	defer span.End()

	var exists bool
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(executionPayloadEnvelopesBucket)
		exists = bkt.Get(blockRoot[:]) != nil
		return nil
	}); err != nil {
		return false
	}
	return exists
}

// DeleteExecutionPayloadEnvelope removes a signed execution payload envelope by beacon block root
// and cleans up the BlockHash index entry.
func (s *Store) DeleteExecutionPayloadEnvelope(ctx context.Context, blockRoot [32]byte) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.DeleteExecutionPayloadEnvelope")
	defer span.End()

	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(executionPayloadEnvelopesBucket)
		// Read the existing entry to find the BlockHash for index cleanup.
		enc := bkt.Get(blockRoot[:])
		if enc != nil {
			blinded, err := decodeBlindedEnvelope(enc)
			if err == nil && blinded.Message != nil {
				blockHash := bytesutil.ToBytes32(blinded.Message.BlockHash)
				if err := tx.Bucket(executionPayloadEnvelopeBlockHashBucket).Delete(blockHash[:]); err != nil {
					return err
				}
			}
		}
		return bkt.Delete(blockRoot[:])
	})
}

// blindEnvelope converts a full signed envelope to its blinded form by replacing
// the execution payload with its block hash. This avoids computing the expensive
// payload hash tree root on the critical path.
func blindEnvelope(env *ethpb.SignedExecutionPayloadEnvelope) *ethpb.SignedBlindedExecutionPayloadEnvelope {
	return &ethpb.SignedBlindedExecutionPayloadEnvelope{
		Message: &ethpb.BlindedExecutionPayloadEnvelope{
			BlockHash:             env.Message.Payload.BlockHash,
			ExecutionRequests:     env.Message.ExecutionRequests,
			BuilderIndex:          env.Message.BuilderIndex,
			BeaconBlockRoot:       env.Message.BeaconBlockRoot,
			Slot:                  primitives.Slot(env.Message.Payload.SlotNumber),
			ParentBlockHash:       env.Message.Payload.ParentHash,
			ParentBeaconBlockRoot: env.Message.ParentBeaconBlockRoot,
		},
		Signature: env.Signature,
	}
}

// encodeBlindedEnvelope SSZ-encodes and snappy-compresses a blinded envelope for storage.
func encodeBlindedEnvelope(env *ethpb.SignedBlindedExecutionPayloadEnvelope) ([]byte, error) {
	sszBytes, err := env.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal blinded envelope")
	}
	return snappy.Encode(nil, sszBytes), nil
}

// decodeBlindedEnvelope snappy-decompresses and SSZ-decodes a blinded envelope from storage.
func decodeBlindedEnvelope(enc []byte) (*ethpb.SignedBlindedExecutionPayloadEnvelope, error) {
	dec, err := snappy.Decode(nil, enc)
	if err != nil {
		return nil, errors.Wrap(err, "could not snappy decode envelope")
	}
	blinded := &ethpb.SignedBlindedExecutionPayloadEnvelope{}
	if err := blinded.UnmarshalSSZ(dec); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal blinded envelope")
	}
	return blinded, nil
}
