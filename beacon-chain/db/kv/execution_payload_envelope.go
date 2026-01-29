package kv

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

// SaveExecutionPayloadEnvelope blinds and saves a signed execution payload envelope.
// The envelope is always stored in blinded form (payload replaced with its hash tree root).
// The key is the execution payload's BlockHash extracted from the envelope.
func (s *Store) SaveExecutionPayloadEnvelope(ctx context.Context, env *ethpb.SignedExecutionPayloadEnvelope) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveExecutionPayloadEnvelope")
	defer span.End()

	if env == nil || env.Message == nil || env.Message.Payload == nil {
		return errors.New("cannot save nil execution payload envelope")
	}

	blockHash := env.Message.Payload.BlockHash
	blinded, err := blindEnvelope(env)
	if err != nil {
		return err
	}

	enc, err := encodeBlindedEnvelope(blinded)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(executionPayloadEnvelopesBucket)
		return bkt.Put(blockHash, enc)
	})
}

// ExecutionPayloadEnvelope retrieves the blinded signed execution payload envelope by block hash.
func (s *Store) ExecutionPayloadEnvelope(ctx context.Context, blockHash [32]byte) (*ethpb.SignedBlindedExecutionPayloadEnvelope, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.ExecutionPayloadEnvelope")
	defer span.End()

	var enc []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(executionPayloadEnvelopesBucket)
		enc = bkt.Get(blockHash[:])
		return nil
	}); err != nil {
		return nil, err
	}
	if enc == nil {
		return nil, errors.Wrap(ErrNotFound, "execution payload envelope not found")
	}
	return decodeBlindedEnvelope(enc)
}

// HasExecutionPayloadEnvelope checks whether an execution payload envelope exists for the given block hash.
func (s *Store) HasExecutionPayloadEnvelope(ctx context.Context, blockHash [32]byte) bool {
	_, span := trace.StartSpan(ctx, "BeaconDB.HasExecutionPayloadEnvelope")
	defer span.End()

	var exists bool
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(executionPayloadEnvelopesBucket)
		exists = bkt.Get(blockHash[:]) != nil
		return nil
	}); err != nil {
		return false
	}
	return exists
}

// DeleteExecutionPayloadEnvelope removes a signed execution payload envelope by block hash.
func (s *Store) DeleteExecutionPayloadEnvelope(ctx context.Context, blockHash [32]byte) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.DeleteExecutionPayloadEnvelope")
	defer span.End()

	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(executionPayloadEnvelopesBucket)
		return bkt.Delete(blockHash[:])
	})
}

// blindEnvelope converts a full signed envelope to its blinded form by replacing
// the execution payload with its hash tree root.
func blindEnvelope(env *ethpb.SignedExecutionPayloadEnvelope) (*ethpb.SignedBlindedExecutionPayloadEnvelope, error) {
	payloadRoot, err := env.Message.Payload.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not compute payload hash tree root")
	}
	return &ethpb.SignedBlindedExecutionPayloadEnvelope{
		Message: &ethpb.BlindedExecutionPayloadEnvelope{
			PayloadRoot:        payloadRoot[:],
			ExecutionRequests:  env.Message.ExecutionRequests,
			BuilderIndex:       env.Message.BuilderIndex,
			BeaconBlockRoot:    env.Message.BeaconBlockRoot,
			Slot:               env.Message.Slot,
			BlobKzgCommitments: env.Message.BlobKzgCommitments,
			StateRoot:          env.Message.StateRoot,
		},
		Signature: env.Signature,
	}, nil
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
