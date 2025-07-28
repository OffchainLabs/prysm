package peerdas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/kzg"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
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

func TestDataColumnSidecars(t *testing.T) {
	t.Run("nil signed block", func(t *testing.T) {
		var expected []*ethpb.DataColumnSidecar = nil
		actual, err := peerdas.DataColumnSidecars(nil, []kzg.CellsAndProofs{})
		require.NoError(t, err)

		require.DeepSSZEqual(t, expected, actual)
	})

	t.Run("empty cells and proofs", func(t *testing.T) {
		// Create a protobuf signed beacon block.
		signedBeaconBlockPb := util.NewBeaconBlockDeneb()

		// Create a signed beacon block from the protobuf.
		signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
		require.NoError(t, err)

		actual, err := peerdas.DataColumnSidecars(signedBeaconBlock, []kzg.CellsAndProofs{})
		require.NoError(t, err)
		require.IsNil(t, actual)
	})

	t.Run("sizes mismatch", func(t *testing.T) {
		// Create a protobuf signed beacon block.
		signedBeaconBlockPb := util.NewBeaconBlockDeneb()

		// Create a signed beacon block from the protobuf.
		signedBeaconBlock, err := blocks.NewSignedBeaconBlock(signedBeaconBlockPb)
		require.NoError(t, err)

		// Create cells and proofs.
		cellsAndProofs := make([]kzg.CellsAndProofs, 1)

		_, err = peerdas.DataColumnSidecars(signedBeaconBlock, cellsAndProofs)
		require.ErrorIs(t, err, peerdas.ErrSizeMismatch)
	})
}

func TestComputeCustodyGroupForColumn(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.NumberOfColumns = 128
	config.NumberOfCustodyGroups = 64
	params.OverrideBeaconConfig(config)

	t.Run("index too large", func(t *testing.T) {
		_, err := peerdas.ComputeCustodyGroupForColumn(1_000_000)
		require.ErrorIs(t, err, peerdas.ErrIndexTooLarge)
	})

	t.Run("nominal", func(t *testing.T) {
		expected := uint64(2)
		actual, err := peerdas.ComputeCustodyGroupForColumn(2)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		expected = uint64(3)
		actual, err = peerdas.ComputeCustodyGroupForColumn(3)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		expected = uint64(2)
		actual, err = peerdas.ComputeCustodyGroupForColumn(66)
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		expected = uint64(3)
		actual, err = peerdas.ComputeCustodyGroupForColumn(67)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	})
}

func TestCustodyGroupSamplingSize(t *testing.T) {
	testCases := []struct {
		name                         string
		custodyType                  peerdas.CustodyType
		validatorsCustodyRequirement uint64
		toAdvertiseCustodyGroupCount uint64
		expected                     uint64
	}{
		{
			name:                         "target, lower than samples per slot",
			custodyType:                  peerdas.Target,
			validatorsCustodyRequirement: 2,
			expected:                     8,
		},
		{
			name:                         "target, higher than samples per slot",
			custodyType:                  peerdas.Target,
			validatorsCustodyRequirement: 100,
			expected:                     100,
		},
		{
			name:                         "actual, lower than samples per slot",
			custodyType:                  peerdas.Actual,
			validatorsCustodyRequirement: 3,
			toAdvertiseCustodyGroupCount: 4,
			expected:                     8,
		},
		{
			name:                         "actual, higher than samples per slot",
			custodyType:                  peerdas.Actual,
			validatorsCustodyRequirement: 100,
			toAdvertiseCustodyGroupCount: 101,
			expected:                     100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a custody info.
			custodyInfo := peerdas.CustodyInfo{}

			// Set the validators custody requirement for target custody group count.
			custodyInfo.TargetGroupCount.SetValidatorsCustodyRequirement(tc.validatorsCustodyRequirement)

			// Set the to advertise custody group count.
			custodyInfo.ToAdvertiseGroupCount.Set(tc.toAdvertiseCustodyGroupCount)

			// Compute the custody group sampling size.
			actual := custodyInfo.CustodyGroupSamplingSize(tc.custodyType)

			// Check the result.
			require.Equal(t, tc.expected, actual)
		})
	}
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
