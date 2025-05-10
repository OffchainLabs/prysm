package peerdas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestInfo(t *testing.T) {
	nodeID := enode.ID{}
	custodyGroupCount := uint64(7)

	expectedCustodyGroup := map[uint64]bool{1: true, 17: true, 19: true, 42: true, 75: true, 87: true, 102: true}
	expectedCustodyColumns := map[uint64]bool{1: true, 17: true, 19: true, 42: true, 75: true, 87: true, 102: true}
	expectedDataColumnsSubnets := map[uint64]bool{1: true, 17: true, 19: true, 42: true, 75: true, 87: true, 102: true}

	for _, cached := range []bool{false, true} {
		actual, ok, err := peerdas.Info(nodeID, custodyGroupCount)
		require.NoError(t, err)
		require.Equal(t, cached, ok)
		require.DeepEqual(t, expectedCustodyGroup, actual.CustodyGroups)
		require.DeepEqual(t, expectedCustodyColumns, actual.CustodyColumns)
		require.DeepEqual(t, expectedDataColumnsSubnets, actual.DataColumnsSubnets)
	}
}

func TestTargetCustodyGroupCount(t *testing.T) {
	testCases := []struct {
		name                         string
		subscribeToAllSubnets        bool
		validatorsCustodyRequirement uint64
		expected                     uint64
	}{
		{
			name:                         "subscribed to all subnets",
			subscribeToAllSubnets:        true,
			validatorsCustodyRequirement: 100,
			expected:                     128,
		},
		{
			name:                         "no validators attached",
			subscribeToAllSubnets:        false,
			validatorsCustodyRequirement: 0,
			expected:                     4,
		},
		{
			name:                         "some validators attached",
			subscribeToAllSubnets:        false,
			validatorsCustodyRequirement: 100,
			expected:                     100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Subscribe to all subnets if needed.
			if tc.subscribeToAllSubnets {
				resetFlags := flags.Get()
				gFlags := new(flags.GlobalFlags)
				gFlags.SubscribeToAllSubnets = true
				flags.Init(gFlags)
				defer flags.Init(resetFlags)
			}

			var custodyInfo peerdas.CustodyInfo

			// Set the validators custody requirement.
			custodyInfo.TargetGroupCount.SetValidatorsCustodyRequirement(tc.validatorsCustodyRequirement)

			// Get the target custody group count.
			actual := custodyInfo.TargetGroupCount.Get()

			// Compare the expected and actual values.
			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestToAdvertiseCustodyGroupCount(t *testing.T) {
	testCases := []struct {
		name                         string
		subscribeToAllSubnets        bool
		toAdvertiseCustodyGroupCount uint64
		expected                     uint64
	}{
		{
			name:                         "subscribed to all subnets",
			subscribeToAllSubnets:        true,
			toAdvertiseCustodyGroupCount: 100,
			expected:                     128,
		},
		{
			name:                         "higher than custody requirement",
			subscribeToAllSubnets:        false,
			toAdvertiseCustodyGroupCount: 100,
			expected:                     100,
		},
		{
			name:                         "lower than custody requirement",
			subscribeToAllSubnets:        false,
			toAdvertiseCustodyGroupCount: 1,
			expected:                     4,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Subscribe to all subnets if needed.
			if tc.subscribeToAllSubnets {
				resetFlags := flags.Get()
				gFlags := new(flags.GlobalFlags)
				gFlags.SubscribeToAllSubnets = true
				flags.Init(gFlags)
				defer flags.Init(resetFlags)
			}

			// Create a custody info.
			var custodyInfo peerdas.CustodyInfo

			// Set the to advertise custody group count.
			custodyInfo.ToAdvertiseGroupCount.Set(tc.toAdvertiseCustodyGroupCount)

			// Get the to advertise custody group count.
			actual := custodyInfo.ToAdvertiseGroupCount.Get()

			// Compare the expected and actual values.
			require.Equal(t, tc.expected, actual)
		})
	}
}
