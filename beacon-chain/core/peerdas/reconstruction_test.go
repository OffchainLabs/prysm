package peerdas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestCanSelfReconstruct(t *testing.T) {
	testCases := []struct {
		name                       string
		totalNumberOfCustodyGroups uint64
		custodyNumberOfGroups      uint64
		expected                   bool
	}{
		{
			name:                       "totalNumberOfCustodyGroups=64, custodyNumberOfGroups=31",
			totalNumberOfCustodyGroups: 64,
			custodyNumberOfGroups:      31,
			expected:                   false,
		},
		{
			name:                       "totalNumberOfCustodyGroups=64, custodyNumberOfGroups=32",
			totalNumberOfCustodyGroups: 64,
			custodyNumberOfGroups:      32,
			expected:                   true,
		},
		{
			name:                       "totalNumberOfCustodyGroups=65, custodyNumberOfGroups=32",
			totalNumberOfCustodyGroups: 65,
			custodyNumberOfGroups:      32,
			expected:                   false,
		},
		{
			name:                       "totalNumberOfCustodyGroups=63, custodyNumberOfGroups=33",
			totalNumberOfCustodyGroups: 65,
			custodyNumberOfGroups:      33,
			expected:                   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set the total number of columns.
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.NumberOfCustodyGroups = tc.totalNumberOfCustodyGroups
			params.OverrideBeaconConfig(cfg)

			// Check if reconstuction is possible.
			actual := peerdas.CanSelfReconstruct(tc.custodyNumberOfGroups)
			require.Equal(t, tc.expected, actual)
		})
	}
}
