package kv

import (
	"fmt"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	bolt "go.etcd.io/bbolt"
)

// Validators stay inside the state because the shared validator table has no VRF root.
func (s *Store) processPQ(pbState *ethpb.BeaconStatePQ, rootHash []byte, bucket, valIdxBkt *bolt.Bucket, validatorKey []byte) error {
	encodedState, err := encodeContainerWithKey(version.PQ, pbState)
	if err != nil {
		return fmt.Errorf("encode PQ state: %w", err)
	}
	if err := bucket.Put(rootHash, encodedState); err != nil {
		return fmt.Errorf("put state: %w", err)
	}
	if err := valIdxBkt.Put(rootHash, validatorKey); err != nil {
		return fmt.Errorf("put validator index key: %w", err)
	}
	return nil
}
