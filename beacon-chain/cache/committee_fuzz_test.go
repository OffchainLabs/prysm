//go:build !fuzz

package cache

import (
	"context"
	"testing"

	fuzz "github.com/google/gofuzz"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestCommitteeKeyFuzz_OK(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	c := &Committees{}

	for i := 0; i < 100000; i++ {
		fuzzer.Fuzz(c)
		k, err := committeeKeyFn(c)
		require.NoError(t, err)
		assert.Equal(t, key(c.Seed), k)
	}
}

func TestCommitteeCache_FuzzCommitteesByEpoch(t *testing.T) {
	cache := NewCommitteesCache()
	fuzzer := fuzz.NewWithSeed(0)
	c := &Committees{}

	for i := 0; i < 100000; i++ {
		fuzzer.Fuzz(c)
		require.NoError(t, cache.AddCommitteeShuffledList(context.Background(), c))
		_, err := cache.Committee(context.Background(), 0, c.Seed, 0)
		require.NoError(t, err)
	}

	assert.Equal(t, maxCommitteesCacheSize, len(cache.CommitteeCache.Keys()), "Incorrect key size")
}

func TestCommitteeCache_FuzzActiveIndices(t *testing.T) {
	cache := NewCommitteesCache()
	fuzzer := fuzz.NewWithSeed(0)
	c := &Committees{}

	for i := 0; i < 100000; i++ {
		fuzzer.Fuzz(c)
		require.NoError(t, cache.AddCommitteeShuffledList(context.Background(), c))

		indices, err := cache.ActiveIndices(context.Background(), c.Seed)
		require.NoError(t, err)
		assert.DeepEqual(t, c.SortedIndices, indices)
	}

	assert.Equal(t, maxCommitteesCacheSize, len(cache.CommitteeCache.Keys()), "Incorrect key size")
}
