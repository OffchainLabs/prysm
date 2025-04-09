package cache

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
)

func BalanceCacheKey(st state.ReadOnlyBeaconState) (string, error) {
	return balanceCacheKey(st)
}

func MaxCheckpointStateSize() int {
	return maxCheckpointStateSize
}

func (c *CheckpointStateCache) Cache() *lru.Cache {
	return c.cache
}
