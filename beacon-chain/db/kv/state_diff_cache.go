package kv

import (
	"math"
	"sync"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/pkg/errors"
)

const unset = math.MaxUint64

type stateDiffCache struct {
	mu      sync.RWMutex
	anchors map[int]state.ReadOnlyBeaconState
	offset  uint64
}

func newStateDiffCache() *stateDiffCache {
	return &stateDiffCache{
		anchors: make(map[int]state.ReadOnlyBeaconState, len(params.StateHierarchyExponents())),
		offset:  unset,
	}
}

func (c *stateDiffCache) getAnchor(level int) state.ReadOnlyBeaconState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.anchors[level]
}

func (c *stateDiffCache) setAnchor(level int, anchor state.ReadOnlyBeaconState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.anchors[level] = anchor
}

func (c *stateDiffCache) getOffset() (uint64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.offset == unset {
		return 0, errors.New("offset is not set")
	}
	return c.offset, nil
}

func (c *stateDiffCache) setOffset(offset uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset = offset
}

func (c *stateDiffCache) clearAnchors() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.anchors = make(map[int]state.ReadOnlyBeaconState, len(params.StateHierarchyExponents()))
}
