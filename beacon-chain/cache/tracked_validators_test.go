package cache

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestTrackedValidatorsCache(t *testing.T) {
	vc := NewTrackedValidatorsCache()

	// No validators in cache.
	require.Equal(t, false, vc.Validating())
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

	// Check if the cache is validating.
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

	require.Equal(t, false, vc.Validating())
}
