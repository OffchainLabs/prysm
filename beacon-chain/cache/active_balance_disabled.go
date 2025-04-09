//go:build fuzz

package cache

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
)

// FakeBalanceCache is a fake struct with 1 LRU cache for looking up balance by epoch.
type FakeBalanceCache struct {
}

// NewEffectiveBalanceCache creates a new effective balance cache for storing/accessing total balance by epoch.
func NewEffectiveBalanceCache() *FakeBalanceCache {
	return &FakeBalanceCache{}
}

// AddTotalEffectiveBalance adds a new total effective balance entry for current balance for state `st` into the cache.
func (c *FakeBalanceCache) AddTotalEffectiveBalance(st state.ReadOnlyBeaconState, balance uint64) error {
	return nil
}

// Get returns the current epoch's effective balance for state `st` in cache.
func (c *FakeBalanceCache) Get(st state.ReadOnlyBeaconState) (uint64, error) {
	return 0, nil
}

// Clear is a stub.
func (c *FakeBalanceCache) Clear() {
	return
}
