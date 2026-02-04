package kv

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"go.etcd.io/bbolt"
)

type stateDiffCache struct {
	sync.RWMutex
	anchors        []state.ReadOnlyBeaconState
	levelsWithData []bool
	offset         uint64
}

func populateStateDiffCacheFromDB(s *Store, offset uint64) (*stateDiffCache, error) {
	cache := &stateDiffCache{
		anchors:        make([]state.ReadOnlyBeaconState, len(flags.Get().StateDiffExponents)-1),
		levelsWithData: make([]bool, len(flags.Get().StateDiffExponents)),
		offset:         offset,
	}

	if err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		for level := range cache.levelsWithData {
			cursor := bucket.Cursor()
			prefix := []byte{byte(level)}
			key, _ := cursor.Seek(prefix)
			if key != nil && key[0] == byte(level) {
				cache.levelsWithData[level] = true
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	anchor0, err := s.getFullSnapshot(offset)
	if err != nil {
		return nil, fmt.Errorf("state diff cache: missing offset snapshot at %d: %w", offset, err)
	}
	cache.anchors[0] = anchor0
	cache.levelsWithData[0] = true

	return cache, nil
}

func newStateDiffCache(s *Store) (*stateDiffCache, error) {
	var offset uint64

	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}

		offsetBytes := bucket.Get(offsetKey)
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
		anchors:        make([]state.ReadOnlyBeaconState, len(flags.Get().StateDiffExponents)-1), // -1 because last level doesn't need to be cached
		levelsWithData: make([]bool, len(flags.Get().StateDiffExponents)),
		offset:         offset,
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

func (c *stateDiffCache) levelHasData(level int) bool {
	c.RLock()
	defer c.RUnlock()
	if level < 0 || level >= len(c.levelsWithData) {
		return false
	}
	return c.levelsWithData[level]
}

func (c *stateDiffCache) setLevelHasData(level int) error {
	c.Lock()
	defer c.Unlock()
	if level < 0 || level >= len(c.levelsWithData) {
		return errors.New("state diff cache: level data index out of range")
	}
	c.levelsWithData[level] = true
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
