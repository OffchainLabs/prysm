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
	sync.RWMutex
	anchors []state.ReadOnlyBeaconState
	offset  uint64
}

func newStateDiffCache() *stateDiffCache {
	return &stateDiffCache{
		anchors: make([]state.ReadOnlyBeaconState, len(params.StateHierarchyExponents())),
		offset:  unset,
	}
}

func (c *stateDiffCache) getAnchor(level int) state.ReadOnlyBeaconState {
	c.RLock()
	defer c.RUnlock()
	return c.anchors[level]
}

func (c *stateDiffCache) setAnchor(level int, anchor state.ReadOnlyBeaconState) {
	c.Lock()
	defer c.Unlock()
	c.anchors[level] = anchor
}

func (c *stateDiffCache) getOffset() (uint64, error) {
	c.RLock()
	defer c.RUnlock()
	if c.offset == unset {
		return 0, errors.New("offset is not set")
	}
	return c.offset, nil
}

func (c *stateDiffCache) setOffset(offset uint64) {
	c.Lock()
	defer c.Unlock()
	c.offset = offset
}

func (c *stateDiffCache) clearAnchors() {
	c.Lock()
	defer c.Unlock()
	c.anchors = make([]state.ReadOnlyBeaconState, len(params.StateHierarchyExponents()))
}
