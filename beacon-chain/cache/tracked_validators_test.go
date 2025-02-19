package cache

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func mapEqual(a, b map[primitives.ValidatorIndex]bool) bool {
	if len(a) != len(b) {
		return false
	}

	for k, v := range a {
		if b[k] != v {
			return false
		}
	}

	return true
}

func TestTrackedValidatorsCache(t *testing.T) {
	vc := NewTrackedValidatorsCache()

	// No validators in cache.
	require.Equal(t, 0, vc.ItemCount())
	require.Equal(t, false, vc.Validating())
	require.Equal(t, 0, len(vc.Indices()))

	_, ok := vc.Validator(41)
	require.Equal(t, false, ok)

	// Add some validators (one twice).
	v42Expected := TrackedValidator{Active: true, FeeRecipient: [20]byte{1}, Index: 42}
	v43Expected := TrackedValidator{Active: false, FeeRecipient: [20]byte{2}, Index: 43}

	vc.Set(v42Expected)
	vc.Set(v43Expected)
	vc.Set(v42Expected)

	// Check if they are in the cache.
	v42Actual, ok := vc.Validator(42)
	require.Equal(t, true, ok)
	require.Equal(t, v42Expected, v42Actual)

	v43Actual, ok := vc.Validator(43)
	require.Equal(t, true, ok)
	require.Equal(t, v43Expected, v43Actual)

	expected := map[primitives.ValidatorIndex]bool{42: true, 43: true}
	actual := vc.Indices()
	require.Equal(t, true, mapEqual(expected, actual))

	// Check the item count and if the cache is validating.
	require.Equal(t, 2, vc.ItemCount())
	require.Equal(t, true, vc.Validating())

	// Check if a non-existing validator is in the cache.
	_, ok = vc.Validator(41)
	require.Equal(t, false, ok)

	// Prune the cache and test it.
	vc.Prune()

	_, ok = vc.Validator(41)
	require.Equal(t, false, ok)

	_, ok = vc.Validator(42)
	require.Equal(t, false, ok)

	_, ok = vc.Validator(43)
	require.Equal(t, false, ok)

	require.Equal(t, 0, vc.ItemCount())
	require.Equal(t, false, vc.Validating())
	require.Equal(t, 0, len(vc.Indices()))
}
