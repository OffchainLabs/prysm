package kv

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

// SaveGenesisValidatorsRoot saves the genesis validators root to db.
func (s *Store) SaveGenesisValidatorsRoot(_ context.Context, genValRoot []byte) error {
	if s == nil || s.db == nil {
		return errors.New("store is nil")
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(genesisInfoBucket)
		if bkt == nil {
			return errors.New("genesis info bucket not found")
		}
		enc := bkt.Get(genesisValidatorsRootKey)
		if len(enc) != 0 && !bytes.Equal(enc, genValRoot) {
			return fmt.Errorf("cannot overwrite existing genesis validators root: %#x", enc)
		}
		return bkt.Put(genesisValidatorsRootKey, genValRoot)
	})
	return err
}

// GenesisValidatorsRoot retrieves the genesis validators root from db.
func (s *Store) GenesisValidatorsRoot(_ context.Context) ([]byte, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store is nil")
	}
	var genValRoot []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(genesisInfoBucket)
		if bkt == nil {
			return errors.New("genesis info bucket not found")
		}
		enc := bkt.Get(genesisValidatorsRootKey)
		if len(enc) == 0 {
			return nil
		}
		genValRoot = enc
		return nil
	})
	return genValRoot, err
}
