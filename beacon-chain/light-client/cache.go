package lightClient

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

// lightClientCache tracks LC data over the non finalized chain for different branches.
type lightClientCache struct {
	items map[[32]byte]*lightClientCacheItem
	tail  [32]byte // the latest finalized block root.
}

type lightClientCacheItem struct {
	period             uint64          // sync committee period
	slot               primitives.Slot // slot of the signature block
	bestUpdate         *interfaces.LightClientUpdate
	bestFinalityUpdate *interfaces.LightClientFinalityUpdate
	parent             *lightClientCacheItem // parent item in the cache, can be nil
}

func newLightClientCache(finalizedBlockRoot [32]byte) *lightClientCache {
	return &lightClientCache{
		items: make(map[[32]byte]*lightClientCacheItem),
		tail:  finalizedBlockRoot,
	}
}
