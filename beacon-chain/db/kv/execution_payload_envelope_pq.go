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

func (s *Store) SaveExecutionPayloadEnvelopePQ(ctx context.Context, env *ethpb.SignedExecutionPayloadEnvelopePQ) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveExecutionPayloadEnvelopePQ")
	defer span.End()

	if env == nil || env.Message == nil || env.Message.Payload == nil {
		return errors.New("cannot save nil execution payload envelope")
	}
	blinded := &ethpb.SignedBlindedExecutionPayloadEnvelopePQ{
		Message: &ethpb.BlindedExecutionPayloadEnvelopePQ{
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
	sszBytes, err := blinded.MarshalSSZ()
	if err != nil {
		return errors.Wrap(err, "could not marshal blinded envelope")
	}
	enc := snappy.Encode(nil, append(pqKey, sszBytes...))
	blockRoot := bytesutil.ToBytes32(env.Message.BeaconBlockRoot)
	blockHash := bytesutil.ToBytes32(env.Message.Payload.BlockHash)
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(executionPayloadEnvelopesBucket).Put(blockRoot[:], enc); err != nil {
			return err
		}
		return tx.Bucket(executionPayloadEnvelopeBlockHashBucket).Put(blockHash[:], blockRoot[:])
	})
}

func (s *Store) ExecutionPayloadEnvelopePQ(ctx context.Context, blockRoot [32]byte) (*ethpb.SignedBlindedExecutionPayloadEnvelopePQ, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.ExecutionPayloadEnvelopePQ")
	defer span.End()

	var enc []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		enc = tx.Bucket(executionPayloadEnvelopesBucket).Get(blockRoot[:])
		return nil
	}); err != nil {
		return nil, err
	}
	if enc == nil {
		return nil, errors.Wrap(ErrNotFound, "execution payload envelope not found")
	}
	dec, err := snappy.Decode(nil, enc)
	if err != nil {
		return nil, errors.Wrap(err, "could not snappy decode envelope")
	}
	if !hasPQKey(dec) {
		return nil, errors.New("stored envelope is not a PQ envelope")
	}
	return decodeBlindedEnvelopePQ(dec)
}

func decodeBlindedEnvelopePQ(dec []byte) (*ethpb.SignedBlindedExecutionPayloadEnvelopePQ, error) {
	blinded := &ethpb.SignedBlindedExecutionPayloadEnvelopePQ{}
	if err := blinded.UnmarshalSSZ(dec[len(pqKey):]); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal blinded envelope")
	}
	return blinded, nil
}

// Callers of the shared reader only use the header fields, the PQ requests and XMSS signature do not fit the Gloas type.
func blindedEnvelopeFromPQ(pq *ethpb.SignedBlindedExecutionPayloadEnvelopePQ) *ethpb.SignedBlindedExecutionPayloadEnvelope {
	return &ethpb.SignedBlindedExecutionPayloadEnvelope{
		Message: &ethpb.BlindedExecutionPayloadEnvelope{
			BlockHash:             pq.Message.BlockHash,
			BuilderIndex:          pq.Message.BuilderIndex,
			BeaconBlockRoot:       pq.Message.BeaconBlockRoot,
			Slot:                  pq.Message.Slot,
			ParentBlockHash:       pq.Message.ParentBlockHash,
			ParentBeaconBlockRoot: pq.Message.ParentBeaconBlockRoot,
		},
	}
}
