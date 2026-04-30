package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

var (
	rootA = [32]byte{0xaa}
	rootB = [32]byte{0xbb}
)

func TestProposerPreferencesCache_AddGetHas(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(123)
	valIdx := primitives.ValidatorIndex(7)
	feeRecipient := []byte{1, 2, 3, 4}

	require.Equal(t, false, c.Has(rootA, slot))
	added := c.Add(rootA, slot, valIdx, feeRecipient, 42)
	require.Equal(t, true, added)
	require.Equal(t, true, c.Has(rootA, slot))

	pref, ok := c.Get(rootA, slot)
	require.Equal(t, true, ok)
	require.Equal(t, valIdx, pref.ValidatorIndex)
	require.DeepEqual(t, feeRecipient, pref.FeeRecipient)
	require.Equal(t, uint64(42), pref.GasLimit)
}

func TestProposerPreferencesCache_AddDuplicate(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(456)

	require.Equal(t, true, c.Add(rootA, slot, 3, []byte{1}, 10))
	require.Equal(t, false, c.Add(rootA, slot, 3, []byte{2}, 20))

	pref, ok := c.Get(rootA, slot)
	require.Equal(t, true, ok)
	require.DeepEqual(t, []byte{1}, pref.FeeRecipient)
	require.Equal(t, uint64(10), pref.GasLimit)
}

func TestProposerPreferencesCache_DifferentBranchesSameSlot(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(456)

	require.Equal(t, true, c.Add(rootA, slot, 3, []byte{1}, 10))
	require.Equal(t, true, c.Add(rootB, slot, 5, []byte{2}, 20))

	prefA, ok := c.Get(rootA, slot)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ValidatorIndex(3), prefA.ValidatorIndex)
	require.DeepEqual(t, []byte{1}, prefA.FeeRecipient)

	prefB, ok := c.Get(rootB, slot)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ValidatorIndex(5), prefB.ValidatorIndex)
	require.DeepEqual(t, []byte{2}, prefB.FeeRecipient)
}

func TestProposerPreferencesCache_Clear(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(789)

	require.Equal(t, true, c.Add(rootA, slot, 1, []byte{1}, 10))
	c.Clear()

	require.Equal(t, false, c.Has(rootA, slot))
	_, ok := c.Get(rootA, slot)
	require.Equal(t, false, ok)
}

func TestProposerPreferencesCache_PruneBefore(t *testing.T) {
	c := NewProposerPreferencesCache()

	require.Equal(t, true, c.Add(rootA, 10, 1, []byte{1}, 10))
	require.Equal(t, true, c.Add(rootA, 11, 2, []byte{2}, 11))
	require.Equal(t, true, c.Add(rootA, 12, 3, []byte{3}, 12))

	c.PruneBefore(11)

	require.Equal(t, false, c.Has(rootA, 10))
	require.Equal(t, true, c.Has(rootA, 11))
	require.Equal(t, true, c.Has(rootA, 12))
}
