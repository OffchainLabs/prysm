package sync

import (
	"sync"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	lru "github.com/hashicorp/golang-lru"
)

// slotAwareCache is a cache that tracks which keys belong to which slot
// to enable slot-based pruning when blocks are finalized.
type slotAwareCache struct {
	cache      *lru.Cache
	slotToKeys map[primitives.Slot][]string
	mu         sync.RWMutex
}

// newSlotAwareCache creates a new slot-aware cache with the given size.
func newSlotAwareCache(size int) *slotAwareCache {
	cache, _ := lru.New(size)
	return &slotAwareCache{
		cache:      cache,
		slotToKeys: make(map[primitives.Slot][]string),
	}
}

// Get retrieves a value from the cache.
func (c *slotAwareCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache.Get(key)
}

// Add adds a value to the cache associated with a specific slot.
func (c *slotAwareCache) Add(slot primitives.Slot, key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Add to cache
	c.cache.Add(key, value)

	// Track slot association
	c.slotToKeys[slot] = append(c.slotToKeys[slot], key)
}

// pruneSlotsBefore removes all entries with slots less than the given slot.
// This should be called when a new finalized checkpoint is reached.
func (c *slotAwareCache) pruneSlotsBefore(finalizedSlot primitives.Slot) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	pruned := 0
	for slot, keys := range c.slotToKeys {
		if slot < finalizedSlot {
			for _, key := range keys {
				c.cache.Remove(key)
				pruned++
			}
			delete(c.slotToKeys, slot)
		}
	}
	return pruned
}

// removeKeyFromSlot removes a key from the slot's key list.
func (c *slotAwareCache) removeKeyFromSlot(slot primitives.Slot, key string) {
	keys := c.slotToKeys[slot]
	for i, k := range keys {
		if k == key {
			// Remove by swapping with last element
			keys[i] = keys[len(keys)-1]
			c.slotToKeys[slot] = keys[:len(keys)-1]
			break
		}
	}
	if len(c.slotToKeys[slot]) == 0 {
		delete(c.slotToKeys, slot)
	}
}
