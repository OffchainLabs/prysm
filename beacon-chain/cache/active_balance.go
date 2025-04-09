//go:build !fuzz

package cache

import (
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	lruwrpr "github.com/prysmaticlabs/prysm/v5/cache/lru"
)

const (
	// maxBalanceCacheSize defines the max number of active balances can cache.
	maxBalanceCacheSize = int(4)
)

var (
	// BalanceCacheMiss tracks the number of balance requests that aren't present in the cache.
	balanceCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "total_effective_balance_cache_miss",
		Help: "The number of get requests that aren't present in the cache.",
	})
	// BalanceCacheHit tracks the number of balance requests that are in the cache.
	balanceCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "total_effective_balance_cache_hit",
		Help: "The number of get requests that are present in the cache.",
	})
)

// BalanceCache is a struct with 1 LRU cache for looking up balance by epoch.
type BalanceCache struct {
	cache *lru.Cache
	lock  sync.RWMutex
}

// NewEffectiveBalanceCache creates a new effective balance cache for storing/accessing total balance by epoch.
func NewEffectiveBalanceCache() *BalanceCache {
	c := &BalanceCache{}
	c.Clear()
	return c
}

// Clear resets the SyncCommitteeCache to its initial state
func (c *BalanceCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.cache = lruwrpr.New(maxBalanceCacheSize)
}

// AddTotalEffectiveBalance adds a new total effective balance entry for current balance for state `st` into the cache.
func (c *BalanceCache) AddTotalEffectiveBalance(st state.ReadOnlyBeaconState, balance uint64) error {
	key, err := balanceCacheKey(st)
	if err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	_ = c.cache.Add(key, balance)
	return nil
}

// Get returns the current epoch's effective balance for state `st` in cache.
func (c *BalanceCache) Get(st state.ReadOnlyBeaconState) (uint64, error) {
	key, err := balanceCacheKey(st)
	if err != nil {
		return 0, err
	}

	c.lock.RLock()
	defer c.lock.RUnlock()

	value, exists := c.cache.Get(key)
	if !exists {
		balanceCacheMiss.Inc()
		return 0, ErrNotFound
	}
	balanceCacheHit.Inc()
	return value.(uint64), nil
}
