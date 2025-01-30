package helpers_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
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
}
