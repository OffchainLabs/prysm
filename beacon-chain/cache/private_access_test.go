package cache

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	lru "github.com/hashicorp/golang-lru"
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
