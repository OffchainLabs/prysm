package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProposerPreferencesCache_AddGetHas(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(123)
	valIdx := primitives.ValidatorIndex(7)
	feeRecipient := []byte{1, 2, 3, 4}

	require.Equal(t, false, c.Has(slot, valIdx))
	added := c.Add(slot, valIdx, feeRecipient, 42)
	require.Equal(t, true, added)
	require.Equal(t, true, c.Has(slot, valIdx))

	pref, ok := c.Get(slot, valIdx)
	require.Equal(t, true, ok)
	require.DeepEqual(t, feeRecipient, pref.FeeRecipient)
	require.Equal(t, uint64(42), pref.GasLimit)
}

func TestProposerPreferencesCache_AddDuplicateSlotAndValidator(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(456)
	valIdx := primitives.ValidatorIndex(3)

	require.Equal(t, true, c.Add(slot, valIdx, []byte{1}, 10))
	require.Equal(t, false, c.Add(slot, valIdx, []byte{2}, 20))

	pref, ok := c.Get(slot, valIdx)
	require.Equal(t, true, ok)
	require.DeepEqual(t, []byte{1}, pref.FeeRecipient)
	require.Equal(t, uint64(10), pref.GasLimit)
}

func TestProposerPreferencesCache_DifferentValidatorsSameSlot(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(456)

	require.Equal(t, true, c.Add(slot, 3, []byte{1}, 10))
	require.Equal(t, true, c.Add(slot, 5, []byte{2}, 20))

	pref3, ok := c.Get(slot, 3)
	require.Equal(t, true, ok)
	require.DeepEqual(t, []byte{1}, pref3.FeeRecipient)

	pref5, ok := c.Get(slot, 5)
	require.Equal(t, true, ok)
	require.DeepEqual(t, []byte{2}, pref5.FeeRecipient)

	require.Equal(t, false, c.Has(slot, 7))
}

func TestProposerPreferencesCache_Clear(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(789)
	valIdx := primitives.ValidatorIndex(1)

	require.Equal(t, true, c.Add(slot, valIdx, []byte{1}, 10))
	c.Clear()

	require.Equal(t, false, c.Has(slot, valIdx))
	_, ok := c.Get(slot, valIdx)
	require.Equal(t, false, ok)
}

func TestProposerPreferencesCache_PruneBefore(t *testing.T) {
	c := NewProposerPreferencesCache()

	require.Equal(t, true, c.Add(10, 1, []byte{1}, 10))
	require.Equal(t, true, c.Add(11, 2, []byte{2}, 11))
	require.Equal(t, true, c.Add(12, 3, []byte{3}, 12))

	c.PruneBefore(11)

	require.Equal(t, false, c.Has(10, 1))
	require.Equal(t, true, c.Has(11, 2))
	require.Equal(t, true, c.Has(12, 3))
}
