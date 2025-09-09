package sync

import (
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestSlotAwareCache(t *testing.T) {
	cache := newSlotAwareCache(100)

	t.Run("basic operations", func(t *testing.T) {
		// Add entries for different slots
		cache.Add(primitives.Slot(10), "key1", "value1")
		cache.Add(primitives.Slot(20), "key2", "value2")
		cache.Add(primitives.Slot(30), "key3", "value3")

		// Check they exist
		val, exists := cache.Get("key1")
		require.Equal(t, true, exists)
		require.Equal(t, "value1", val)

		val, exists = cache.Get("key2")
		require.Equal(t, true, exists)
		require.Equal(t, "value2", val)

		val, exists = cache.Get("key3")
		require.Equal(t, true, exists)
		require.Equal(t, "value3", val)

		// Check cache stats
		totalEntries, slotsTracked := cache.cache.Len(), len(cache.slotToKeys)
		require.Equal(t, 3, totalEntries)
		require.Equal(t, 3, slotsTracked)
	})

	// Test slot-based pruning
	t.Run("slot-based pruning", func(t *testing.T) {
		cache := newSlotAwareCache(100)

		// Add entries for different slots
		cache.Add(primitives.Slot(10), "key10", "value10")
		cache.Add(primitives.Slot(20), "key20", "value20")
		cache.Add(primitives.Slot(30), "key30", "value30")
		cache.Add(primitives.Slot(40), "key40", "value40")

		pruned := cache.pruneSlotsBefore(primitives.Slot(25))
		require.Equal(t, 2, pruned) // Should prune entries from slots 10 and 20

		// Check that entries from slots 10 and 20 are gone
		_, exists := cache.Get("key10")
		require.Equal(t, false, exists)

		_, exists = cache.Get("key20")
		require.Equal(t, false, exists)

		// Check that entries from slots 30 and 40 still exist
		val, exists := cache.Get("key30")
		require.Equal(t, true, exists)
		require.Equal(t, "value30", val)

		val, exists = cache.Get("key40")
		require.Equal(t, true, exists)
		require.Equal(t, "value40", val)

		// Check cache stats
		totalEntries, slotsTracked := cache.cache.Len(), len(cache.slotToKeys)
		require.Equal(t, 2, totalEntries)
		require.Equal(t, 2, slotsTracked)
	})

	t.Run("multiple keys per slot", func(t *testing.T) {
		cache := newSlotAwareCache(100)

		// Add multiple entries for the same slot
		cache.Add(primitives.Slot(10), "key1", "value1")
		cache.Add(primitives.Slot(10), "key2", "value2")
		cache.Add(primitives.Slot(20), "key3", "value3")

		// Check they exist
		val, exists := cache.Get("key1")
		require.Equal(t, true, exists)
		require.Equal(t, "value1", val)

		val, exists = cache.Get("key2")
		require.Equal(t, true, exists)
		require.Equal(t, "value2", val)

		// Prune slot 10
		pruned := cache.pruneSlotsBefore(primitives.Slot(15))
		require.Equal(t, 2, pruned) // Should prune both keys from slot 10

		// Check that both keys from slot 10 are gone
		_, exists = cache.Get("key1")
		require.Equal(t, false, exists)

		_, exists = cache.Get("key2")
		require.Equal(t, false, exists)

		// Check that key from slot 20 still exists
		val, exists = cache.Get("key3")
		require.Equal(t, true, exists)
		require.Equal(t, "value3", val)
	})

	t.Run("bounded slot tracking", func(t *testing.T) {
		cache := newSlotAwareCache(200000) // Large cache to avoid LRU eviction

		// Add entries for 1005 slots, each with one key
		for i := 0; i < 1005; i++ {
			slot := primitives.Slot(i)
			key := fmt.Sprintf("key%d", i)
			cache.Add(slot, key, fmt.Sprintf("value%d", i))
		}

		// Should only track 1000 slots (oldest 5 slots pruned)
		require.Equal(t, 1000, len(cache.slotToKeys))
	})
}
