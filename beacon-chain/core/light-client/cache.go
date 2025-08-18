package light_client

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

// cache tracks LC data over the non finalized chain for different branches.
type cache struct {
	items map[[32]byte]*cacheItem
	tail  [32]byte // the latest finalized block root.
}

type cacheItem struct {
	period             uint64          // sync committee period
	slot               primitives.Slot // slot of the signature block
	bestUpdate         *interfaces.LightClientUpdate
	bestFinalityUpdate *interfaces.LightClientFinalityUpdate
	parent             *cacheItem // parent item in the cache, can be nil
}

func newLightClientCache(finalizedBlockRoot [32]byte) *cache {
	return &cache{
		items: make(map[[32]byte]*cacheItem),
		tail:  finalizedBlockRoot,
	}
}
