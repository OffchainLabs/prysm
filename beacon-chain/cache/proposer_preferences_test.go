package cache

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common"
)

var (
	rootA = [32]byte{0xaa}
	rootB = [32]byte{0xbb}
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
	require.DeepEqual(t, pref.FeeRecipient, got.FeeRecipient)
	require.Equal(t, pref.GasLimit, got.GasLimit)
}

func TestProposerPreferencesCache_AddDuplicate(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(456)

	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 3, FeeRecipient: primitives.ExecutionAddress{1}, GasLimit: 10}, slot))
	require.Equal(t, false, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 3, FeeRecipient: primitives.ExecutionAddress{2}, GasLimit: 20}, slot))

	pref, ok := c.Get(rootA, slot)
	require.Equal(t, true, ok)
	require.DeepEqual(t, primitives.ExecutionAddress{1}, pref.FeeRecipient)
	require.Equal(t, uint64(10), pref.GasLimit)
}

func TestProposerPreferencesCache_DifferentBranchesSameSlot(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(456)

	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 3, FeeRecipient: primitives.ExecutionAddress{1}, GasLimit: 10}, slot))
	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootB, ValidatorIndex: 5, FeeRecipient: primitives.ExecutionAddress{2}, GasLimit: 20}, slot))

	prefA, ok := c.Get(rootA, slot)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ValidatorIndex(3), prefA.ValidatorIndex)
	require.DeepEqual(t, primitives.ExecutionAddress{1}, prefA.FeeRecipient)

	prefB, ok := c.Get(rootB, slot)
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ValidatorIndex(5), prefB.ValidatorIndex)
	require.DeepEqual(t, primitives.ExecutionAddress{2}, prefB.FeeRecipient)
}

func TestProposerPreferencesCache_Clear(t *testing.T) {
	c := NewProposerPreferencesCache()
	slot := primitives.Slot(789)

	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 1, FeeRecipient: primitives.ExecutionAddress{1}, GasLimit: 10}, slot))
	c.Clear()

	require.Equal(t, false, c.Has(rootA, slot))
	_, ok := c.Get(rootA, slot)
	require.Equal(t, false, ok)
}

func TestProposerPreferencesCache_PruneBefore(t *testing.T) {
	c := NewProposerPreferencesCache()

	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 1, FeeRecipient: primitives.ExecutionAddress{1}, GasLimit: 10}, 10))
	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 2, FeeRecipient: primitives.ExecutionAddress{2}, GasLimit: 11}, 11))
	require.Equal(t, true, c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: 3, FeeRecipient: primitives.ExecutionAddress{3}, GasLimit: 12}, 12))

	c.PruneBefore(11)

	require.Equal(t, false, c.Has(rootA, 10))
	require.Equal(t, true, c.Has(rootA, 11))
	require.Equal(t, true, c.Has(rootA, 12))
}

func TestProposerPreferencesCache_SetAndDefault(t *testing.T) {
	c := NewProposerPreferencesCache()
	pref := ProposerPreference{
		ValidatorIndex: 9,
		FeeRecipient:   primitives.ExecutionAddress{0xde, 0xad},
		GasLimit:       30_000_000,
	}

	_, ok := c.Default(9)
	require.Equal(t, false, ok)

	c.Set(pref)
	got, ok := c.Default(9)
	require.Equal(t, true, ok)
	require.Equal(t, pref.ValidatorIndex, got.ValidatorIndex)
	require.Equal(t, pref.GasLimit, got.GasLimit)
	require.DeepEqual(t, pref.FeeRecipient, got.FeeRecipient)
}

func TestProposerPreferencesCache_SetOverwrites(t *testing.T) {
	c := NewProposerPreferencesCache()
	c.Set(ProposerPreference{ValidatorIndex: 4, FeeRecipient: primitives.ExecutionAddress{1}, GasLimit: 10})
	c.Set(ProposerPreference{ValidatorIndex: 4, FeeRecipient: primitives.ExecutionAddress{2}, GasLimit: 20})

	got, ok := c.Default(4)
	require.Equal(t, true, ok)
	require.DeepEqual(t, primitives.ExecutionAddress{2}, got.FeeRecipient)
	require.Equal(t, uint64(20), got.GasLimit)
}

func TestProposerPreferencesCache_BestFor(t *testing.T) {
	slot := primitives.Slot(123)
	idx := primitives.ValidatorIndex(7)

	t.Run("total miss returns false", func(t *testing.T) {
		c := NewProposerPreferencesCache()
		_, ok := c.BestFor(rootA, slot, idx)
		require.Equal(t, false, ok)
	})

	t.Run("default-only fallback hits", func(t *testing.T) {
		c := NewProposerPreferencesCache()
		c.Set(ProposerPreference{ValidatorIndex: idx, FeeRecipient: primitives.ExecutionAddress{0x01}})
		got, ok := c.BestFor(rootA, slot, idx)
		require.Equal(t, true, ok)
		require.Equal(t, primitives.ExecutionAddress{0x01}, got.FeeRecipient)
	})

	t.Run("branch-specific entry wins over default", func(t *testing.T) {
		c := NewProposerPreferencesCache()
		c.Set(ProposerPreference{ValidatorIndex: idx, FeeRecipient: primitives.ExecutionAddress{0x01}})
		c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: idx, FeeRecipient: primitives.ExecutionAddress{0x02}}, slot)
		got, ok := c.BestFor(rootA, slot, idx)
		require.Equal(t, true, ok)
		require.Equal(t, primitives.ExecutionAddress{0x02}, got.FeeRecipient)
	})

	t.Run("branch-specific entry for wrong validator falls through to default", func(t *testing.T) {
		c := NewProposerPreferencesCache()
		c.Set(ProposerPreference{ValidatorIndex: idx, FeeRecipient: primitives.ExecutionAddress{0x01}})
		c.Add(ProposerPreference{DependentRoot: rootA, ValidatorIndex: idx + 1, FeeRecipient: primitives.ExecutionAddress{0x99}}, slot)
		got, ok := c.BestFor(rootA, slot, idx)
		require.Equal(t, true, ok)
		require.Equal(t, primitives.ExecutionAddress{0x01}, got.FeeRecipient)
	})

	t.Run("different branch falls through to default", func(t *testing.T) {
		c := NewProposerPreferencesCache()
		c.Set(ProposerPreference{ValidatorIndex: idx, FeeRecipient: primitives.ExecutionAddress{0x01}})
		c.Add(ProposerPreference{DependentRoot: rootB, ValidatorIndex: idx, FeeRecipient: primitives.ExecutionAddress{0x02}}, slot)
		got, ok := c.BestFor(rootA, slot, idx)
		require.Equal(t, true, ok)
		require.Equal(t, primitives.ExecutionAddress{0x01}, got.FeeRecipient)
	})
}

func TestProposerPreference_FeeRecipientOrDefault(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.DefaultFeeRecipient = common.Address([20]byte{'a'})
	params.OverrideBeaconConfig(cfg)

	t.Run("empty fee recipient returns configured default", func(t *testing.T) {
		val := &ProposerPreference{ValidatorIndex: 1}
		got := val.FeeRecipientOrDefault()
		require.Equal(t, params.BeaconConfig().DefaultFeeRecipient, common.BytesToAddress(got[:]))
	})

	t.Run("non-empty fee recipient returned as-is", func(t *testing.T) {
		preset := common.Address([20]byte{'b'})
		val := &ProposerPreference{ValidatorIndex: 1, FeeRecipient: primitives.ExecutionAddress(preset)}
		got := val.FeeRecipientOrDefault()
		require.Equal(t, preset, common.BytesToAddress(got[:]))
	})
}

func TestProposerPreference_GasLimitOrDefault(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.DefaultBuilderGasLimit = 36_000_000
	params.OverrideBeaconConfig(cfg)

	t.Run("zero gas limit returns configured default", func(t *testing.T) {
		val := &ProposerPreference{ValidatorIndex: 1}
		require.Equal(t, params.BeaconConfig().DefaultBuilderGasLimit, val.GasLimitOrDefault())
	})

	t.Run("non-zero gas limit returned as-is", func(t *testing.T) {
		val := &ProposerPreference{ValidatorIndex: 1, GasLimit: 42}
		require.Equal(t, uint64(42), val.GasLimitOrDefault())
	})
}
