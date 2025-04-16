package kv

import (
	"bytes"
	"context"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	bolt "go.etcd.io/bbolt"
)

// LastValidatedCheckpoint returns the latest fully validated checkpoint in beacon chain.
func (s *Store) LastValidatedCheckpoint(ctx context.Context) (*ethpb.Checkpoint, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.LastValidatedCheckpoint")
	defer span.End()
	var checkpoint *ethpb.Checkpoint
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(checkpointBucket)
		enc := bkt.Get(lastValidatedCheckpointKey)
		if enc == nil {
			var finErr error
			checkpoint, finErr = s.FinalizedCheckpoint(ctx)
			if finErr != nil {
				return finErr
			}
			if bytes.Equal(checkpoint.Root, params.BeaconConfig().ZeroHash[:]) {
				bkt = tx.Bucket(blocksBucket)
				r := bkt.Get(genesisBlockRootKey)
				if r != nil {
					checkpoint.Root = r
				}
			}
			return nil
		}
		checkpoint = &ethpb.Checkpoint{}
		return decode(ctx, enc, checkpoint)
	})
	return checkpoint, err
}

// SaveLastValidatedCheckpoint saves the last validated checkpoint in beacon chain.
func (s *Store) SaveLastValidatedCheckpoint(ctx context.Context, checkpoint *ethpb.Checkpoint) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveLastValidatedCheckpoint")
	defer span.End()

	return s.saveCheckpoint(ctx, lastValidatedCheckpointKey, checkpoint)
}
