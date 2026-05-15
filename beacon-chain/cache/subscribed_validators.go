package cache

import (
	"strconv"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	gocache "github.com/patrickmn/go-cache"
)

// SubscribedValidatorsCache tracks validator indices the validator client has
// posted committee subscriptions for.
type SubscribedValidatorsCache struct {
	mu      sync.RWMutex
	entries *gocache.Cache
}

// NewSubscribedValidatorsCache returns a cache with the given entry TTL.
// The VC re-posts subscriptions each epoch, so a TTL of a few epochs is
// enough to ride out a missed post without dropping the validator from
// custody calculations.
func NewSubscribedValidatorsCache(ttl, cleanupInterval time.Duration) *SubscribedValidatorsCache {
	return &SubscribedValidatorsCache{
		entries: gocache.New(ttl, cleanupInterval),
	}
}

// Add records the validator as attached. Re-adding extends the TTL.
func (c *SubscribedValidatorsCache) Add(index primitives.ValidatorIndex) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries.Set(subscribedKey(index), struct{}{}, gocache.DefaultExpiration)
}

// Has returns true if the validator is currently attached.
func (c *SubscribedValidatorsCache) Has(index primitives.ValidatorIndex) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.entries.Get(subscribedKey(index))
	return ok
}

// Validating returns true if at least one validator is attached.
func (c *SubscribedValidatorsCache) Validating() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries.ItemCount() > 0
}

// Indices returns the set of currently-attached validator indices.
func (c *SubscribedValidatorsCache) Indices() map[primitives.ValidatorIndex]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	items := c.entries.Items()
	out := make(map[primitives.ValidatorIndex]bool, len(items))
	for key := range items {
		idx, err := strconv.ParseUint(key, 10, 64)
		if err != nil {
			log.WithError(err).Errorf("Failed to parse subscribed validator key: %s", key)
			continue
		}
		out[primitives.ValidatorIndex(idx)] = true
	}
	return out
}

func subscribedKey(index primitives.ValidatorIndex) string {
	return strconv.FormatUint(uint64(index), 10)
}
