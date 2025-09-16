package light_client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestLCCache(t *testing.T) {
	lcCache := newLightClientCache()
	require.NotNil(t, lcCache)

	item := &cacheItem{
		period:             5,
		bestUpdate:         nil,
		bestFinalityUpdate: nil,
	}

	blkRoot := [32]byte{4, 5, 6}

	lcCache.items[blkRoot] = item

	require.Equal(t, item, lcCache.items[blkRoot], "Expected to find the item in the cache")
}
