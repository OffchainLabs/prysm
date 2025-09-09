package sync

import (
	"slices"
	"sync"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	lru "github.com/hashicorp/golang-lru"
)

const maxSlots = 1000 // Maximum number of slots to track before pruning oldest

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

	c.pruneOldestSlots()
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

// pruneOldestSlots removes the oldest slots when we exceed maxSlots.
// This ensures bounded memory usage even during long finalization delays.
// Note: This function assumes the mutex is already held.
func (c *slotAwareCache) pruneOldestSlots() {
	if len(c.slotToKeys) <= maxSlots {
		return
	}

	// Get all slots and sort them to find the oldest
	slots := make([]primitives.Slot, 0, len(c.slotToKeys))
	for slot := range c.slotToKeys {
		slots = append(slots, slot)
	}
	slices.Sort(slots)

	// Remove oldest slots until we're back under the limit
	slotsToRemove := len(c.slotToKeys) - maxSlots
	for i := range slotsToRemove {
		slot := slots[i]
		delete(c.slotToKeys, slot)
	}
}
