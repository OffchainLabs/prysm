package kv

import (
	"encoding/binary"
	"errors"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"go.etcd.io/bbolt"
)

type stateDiffCache struct {
	sync.RWMutex
	anchors []state.ReadOnlyBeaconState
	offset  uint64
}

func newStateDiffCache(s *Store) (*stateDiffCache, error) {
	var offset uint64

	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}

		offsetBytes := bucket.Get([]byte("offset"))
		if offsetBytes == nil {
			return errors.New("state diff cache: offset not found")
		}
		offset = binary.LittleEndian.Uint64(offsetBytes)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &stateDiffCache{
		anchors: make([]state.ReadOnlyBeaconState, len(flags.Get().StateDiffExponents)-1), // -1 because last level doesn't need to be cached
		offset:  offset,
	}, nil
}

func (c *stateDiffCache) getAnchor(level int) state.ReadOnlyBeaconState {
	c.RLock()
	defer c.RUnlock()
	return c.anchors[level]
}

func (c *stateDiffCache) setAnchor(level int, anchor state.ReadOnlyBeaconState) error {
	c.Lock()
	defer c.Unlock()
	if level >= len(c.anchors) || level < 0 {
		return errors.New("state diff cache: anchor level out of range")
	}
	c.anchors[level] = anchor
	return nil
}

func (c *stateDiffCache) getOffset() uint64 {
	c.RLock()
	defer c.RUnlock()
	return c.offset
}

func (c *stateDiffCache) setOffset(offset uint64) {
	c.Lock()
	defer c.Unlock()
	c.offset = offset
}

func (c *stateDiffCache) clearAnchors() {
	c.Lock()
	defer c.Unlock()
	c.anchors = make([]state.ReadOnlyBeaconState, len(flags.Get().StateDiffExponents)-1) // -1 because last level doesn't need to be cached
}
