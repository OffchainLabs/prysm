package peerdas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestCustodyGroups(t *testing.T) {
	// --------------------------------------------
	// The happy path is unit tested in spec tests.
	// --------------------------------------------
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups
	_, err := peerdas.CustodyGroups(enode.ID{}, numberOfCustodyGroups+1)
	require.ErrorIs(t, err, peerdas.ErrCustodyGroupCountTooLarge)
}

func TestComputeColumnsForCustodyGroup(t *testing.T) {
	// --------------------------------------------
	// The happy path is unit tested in spec tests.
	// --------------------------------------------
	numberOfCustodyGroups := params.BeaconConfig().NumberOfCustodyGroups
	_, err := peerdas.ComputeColumnsForCustodyGroup(numberOfCustodyGroups)
	require.ErrorIs(t, err, peerdas.ErrCustodyGroupTooLarge)
}

func TestCustodyColumns(t *testing.T) {
	t.Run("group too large", func(t *testing.T) {
		_, err := peerdas.CustodyColumns([]uint64{1_000_000})
		require.ErrorIs(t, err, peerdas.ErrCustodyGroupTooLarge)
	})

	t.Run("nominal", func(t *testing.T) {
		input := []uint64{1, 2}
		expected := map[uint64]bool{1: true, 2: true}

		actual, err := peerdas.CustodyColumns(input)
		require.NoError(t, err)
		require.Equal(t, len(expected), len(actual))
		for i := range actual {
			require.Equal(t, expected[i], actual[i])
		}
	})
}
