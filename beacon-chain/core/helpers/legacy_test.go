package helpers_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/stretchr/testify/require"
)

// TestDepositRequestHaveStarted contains several test cases for depositRequestHaveStarted.
func TestDepositRequestHaveStarted(t *testing.T) {
	t.Run("Version below Electra returns false", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateBellatrix(t, 1)
		result := helpers.DepositRequestsStarted(st)
		require.False(t, result)
	})

	t.Run("Version is Electra or higher, no error, but Eth1DepositIndex != requestsStartIndex returns false", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateElectra(t, 1)
		require.NoError(t, st.SetEth1DepositIndex(1))
		result := helpers.DepositRequestsStarted(st)
		require.False(t, result)
	})

	t.Run("Version is Electra or higher, no error, and Eth1DepositIndex == requestsStartIndex returns true", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateElectra(t, 1)
		require.NoError(t, st.SetEth1DepositIndex(33))
		require.NoError(t, st.SetDepositRequestsStartIndex(33))
		result := helpers.DepositRequestsStarted(st)
		require.True(t, result)
	})

	t.Run("Version is Fulu returns true even with an unset requestsStartIndex", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateFulu(t, 1)
		require.NoError(t, st.SetEth1DepositIndex(1))
		require.NoError(t, st.SetDepositRequestsStartIndex(params.BeaconConfig().UnsetDepositRequestsStartIndex))
		result := helpers.DepositRequestsStarted(st)
		require.True(t, result)
	})

	t.Run("Version is above Fulu returns true even with an unset requestsStartIndex", func(t *testing.T) {
		st, _ := util.DeterministicGenesisStateGloas(t, 64)
		require.NoError(t, st.SetEth1DepositIndex(1))
		require.NoError(t, st.SetDepositRequestsStartIndex(params.BeaconConfig().UnsetDepositRequestsStartIndex))
		result := helpers.DepositRequestsStarted(st)
		require.True(t, result)
	})
}
