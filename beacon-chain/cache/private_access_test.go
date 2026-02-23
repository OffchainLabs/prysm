package cache

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	lru "github.com/hashicorp/golang-lru"
)

func BalanceCacheKey(st state.ReadOnlyBeaconState) (string, error) {
	return balanceCacheKey(st)
}

func MaxCheckpointStateSize() int {
	return defaultCheckpointStateSize
}

func ExpandedCheckpointStateSize() int {
	return expandedCheckpointStateSize
}

func (c *CheckpointStateCache) Cache() *lru.Cache {
	return c.cache
}

func CheckpointStateTTL() time.Duration {
	return checkpointStateTTL
}

// BackdateEntry shifts the addedAt time of a cache entry to simulate aging.
func (c *CheckpointStateCache) BackdateEntry(key any, d time.Duration) {
	item, ok := c.cache.Peek(key)
	if !ok {
		return
	}
	entry := item.(*checkpointStateEntry)
	entry.addedAt = entry.addedAt.Add(-d)
}
