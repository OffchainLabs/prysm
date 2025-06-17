//go:build !fuzz

package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	fuzz "github.com/google/gofuzz"
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
		require.NoError(t, cache.AddCommitteeShuffledList(t.Context(), c))
		_, err := cache.Committee(t.Context(), 0, c.Seed, 0)
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
		require.NoError(t, cache.AddCommitteeShuffledList(t.Context(), c))

		indices, err := cache.ActiveIndices(t.Context(), c.Seed)
		require.NoError(t, err)
		assert.DeepEqual(t, c.SortedIndices, indices)
	}

	assert.Equal(t, maxCommitteesCacheSize, len(cache.CommitteeCache.Keys()), "Incorrect key size")
}
