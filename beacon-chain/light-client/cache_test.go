package lightClient

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestLCCache(t *testing.T) {
	cpRoot := [32]byte{1, 2, 3}
	lcCache := newLightClientCache(cpRoot)
	require.NotNil(t, lcCache)

	require.Equal(t, true, lcCache.tail == cpRoot)

	item := &lightClientCacheItem{
		period:             5,
		bestUpdate:         nil,
		bestFinalityUpdate: nil,
	}

	blkRoot := [32]byte{4, 5, 6}

	lcCache.items[blkRoot] = item

	require.Equal(t, item, lcCache.items[blkRoot], "Expected to find the item in the cache")
}
