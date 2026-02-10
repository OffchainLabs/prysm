package cache_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/stretchr/testify/require"
)

func TestPayloadAttestationCache_SeenAndAdd(t *testing.T) {
	var c cache.PayloadAttestationCache
	slot1 := primitives.Slot(1)
	slot2 := primitives.Slot(2)
	idx1 := primitives.ValidatorIndex(3)
	idx2 := primitives.ValidatorIndex(4)

	require.False(t, c.Seen(slot1, idx1))

	require.NoError(t, c.Add(slot1, idx1))
	require.True(t, c.Seen(slot1, idx1))
	require.False(t, c.Seen(slot1, idx2))
	require.False(t, c.Seen(slot2, idx1))

	require.NoError(t, c.Add(slot1, idx2))
	require.True(t, c.Seen(slot1, idx1))
	require.True(t, c.Seen(slot1, idx2))

	require.NoError(t, c.Add(slot2, idx1))
	require.True(t, c.Seen(slot2, idx1))
	require.False(t, c.Seen(slot1, idx1))
	require.False(t, c.Seen(slot1, idx2))
}

func TestPayloadAttestationCache_Clear(t *testing.T) {
	var c cache.PayloadAttestationCache
	slot := primitives.Slot(10)
	idx := primitives.ValidatorIndex(42)

	require.NoError(t, c.Add(slot, idx))
	require.True(t, c.Seen(slot, idx))

	c.Clear()
	require.False(t, c.Seen(slot, idx))

	require.NoError(t, c.Add(slot, idx))
	require.True(t, c.Seen(slot, idx))
}
