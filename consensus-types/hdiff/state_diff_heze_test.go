package hdiff

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/heze"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestHezeDiff(t *testing.T) {
	gloasSource, err := gloasState(t, 64)
	require.NoError(t, err)
	hezeSource, err := heze.UpgradeToHeze(gloasSource.Copy())
	require.NoError(t, err)
	require.Equal(t, version.Heze, hezeSource.Version())

	t.Run("round trip", func(t *testing.T) {
		target := hezeSource.Copy()
		require.NoError(t, target.SetSlot(hezeSource.Slot()+1))
		require.NoError(t, target.SetNextWithdrawalBuilderIndex(42))

		hdiffBytes, err := Diff(hezeSource, target)
		require.NoError(t, err)
		result, err := ApplyDiff(t.Context(), hezeSource.Copy(), hdiffBytes)
		require.NoError(t, err)
		requireEqualState(t, target, result)
	})

	t.Run("cross fork from Gloas", func(t *testing.T) {
		hdiffBytes, err := Diff(gloasSource, hezeSource)
		require.NoError(t, err)
		result, err := ApplyDiff(t.Context(), gloasSource.Copy(), hdiffBytes)
		require.NoError(t, err)
		require.Equal(t, version.Heze, result.Version())
		requireEqualState(t, hezeSource, result)
	})
}
