package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProposerPreferencesCache_AddGetHas(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(123)
	feeRecipient := []byte{1, 2, 3, 4}

	require.Equal(t, false, c.Has(slot))
	added := c.Add(slot, feeRecipient, 42)
	require.Equal(t, true, added)
	require.Equal(t, true, c.Has(slot))

	pref, ok := c.Get(slot)
	require.Equal(t, true, ok)
	require.DeepEqual(t, feeRecipient, pref.FeeRecipient)
	require.Equal(t, uint64(42), pref.GasLimit)
}

func TestProposerPreferencesCache_AddDuplicateSlot(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(456)

	require.Equal(t, true, c.Add(slot, []byte{1}, 10))
	require.Equal(t, false, c.Add(slot, []byte{2}, 20))

	pref, ok := c.Get(slot)
	require.Equal(t, true, ok)
	require.DeepEqual(t, []byte{1}, pref.FeeRecipient)
	require.Equal(t, uint64(10), pref.GasLimit)
}

func TestProposerPreferencesCache_Clear(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(789)

	require.Equal(t, true, c.Add(slot, []byte{1}, 10))
	c.Clear()

	require.Equal(t, false, c.Has(slot))
	_, ok := c.Get(slot)
	require.Equal(t, false, ok)
}

func TestProposerPreferencesCache_PruneBefore(t *testing.T) {
	c := NewProposerPreferencesCache()

	require.Equal(t, true, c.Add(10, []byte{1}, 10))
	require.Equal(t, true, c.Add(11, []byte{2}, 11))
	require.Equal(t, true, c.Add(12, []byte{3}, 12))

	c.PruneBefore(11)

	require.Equal(t, false, c.Has(10))
	require.Equal(t, true, c.Has(11))
	require.Equal(t, true, c.Has(12))
}
