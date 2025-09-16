package light_client

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

// cache tracks LC data over the non finalized chain for different branches.
type cache struct {
	items map[[32]byte]*cacheItem
}

// cacheItem represents the LC data for a block. It tracks the best update and finality update seen in that branch.
type cacheItem struct {
	parent             *cacheItem      // parent item in the cache, can be nil
	period             uint64          // sync committee period
	slot               primitives.Slot // slot of the signature block
	bestUpdate         interfaces.LightClientUpdate
	bestFinalityUpdate interfaces.LightClientFinalityUpdate
}

func newLightClientCache() *cache {
	return &cache{
		items: make(map[[32]byte]*cacheItem),
	}
}
