package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

var (
	rootA = [32]byte{0xaa}
	rootB = [32]byte{0xbb}

	feeA = primitives.ExecutionAddress{0x01}
	feeB = primitives.ExecutionAddress{0x02}
	feeC = primitives.ExecutionAddress{0x03}
)

func TestProposerPreferencesCache_AddGetHas(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(123)
	pref := ProposerPreference{
		DependentRoot:  rootA,
		ValidatorIndex: 7,
		FeeRecipient:   primitives.ExecutionAddress{1, 2, 3, 4},
		GasLimit:       42,
	}

	require.Equal(t, false, c.Has(rootA, slot))
	require.Equal(t, true, c.Add(pref, slot))
	require.Equal(t, true, c.Has(rootA, slot))

	got, ok := c.Get(rootA, slot)
	require.Equal(t, true, ok)
	require.Equal(t, pref.ValidatorIndex, got.ValidatorIndex)
	require.Equal(t, pref.FeeRecipient, got.FeeRecipient)
	require.Equal(t, pref.GasLimit, got.GasLimit)
}

func TestProposerPreferencesCache_AddDuplicate(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(456)

	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 3, FeeRecipient: feeA, GasLimit: 10}, slot))
	require.Equal(t, false, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 3, FeeRecipient: feeB, GasLimit: 20}, slot))

	pref, ok := c.Get(rootA, slot)
	require.Equal(t, true, ok)
	require.Equal(t, feeA, pref.FeeRecipient)
	require.Equal(t, uint64(10), pref.GasLimit)
}

func TestProposerPreferencesCache_DifferentBranchesSameSlot(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(456)

	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 3, FeeRecipient: feeA, GasLimit: 10}, slot))
	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootB, ValidatorIndex: 5, FeeRecipient: feeB, GasLimit: 20}, slot))

	prefA, ok := c.Get(rootA, slot)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ValidatorIndex(3), prefA.ValidatorIndex)
	require.Equal(t, feeA, prefA.FeeRecipient)

	prefB, ok := c.Get(rootB, slot)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ValidatorIndex(5), prefB.ValidatorIndex)
	require.Equal(t, feeB, prefB.FeeRecipient)
}

func TestProposerPreferencesCache_Clear(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(789)

	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 1, FeeRecipient: feeA, GasLimit: 10}, slot))
	c.Clear()

	require.Equal(t, false, c.Has(rootA, slot))
	_, ok := c.Get(rootA, slot)
	require.Equal(t, false, ok)
}

func TestProposerPreferencesCache_PruneBefore(t *testing.T) {
	c := NewProposerPreferencesCache()

	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 1, FeeRecipient: feeA, GasLimit: 10}, 10))
	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 2, FeeRecipient: feeB, GasLimit: 11}, 11))
	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 3, FeeRecipient: feeC, GasLimit: 12}, 12))

	c.PruneBefore(11)

	require.Equal(t, false, c.Has(rootA, 10))
	require.Equal(t, true, c.Has(rootA, 11))
	require.Equal(t, true, c.Has(rootA, 12))
}
