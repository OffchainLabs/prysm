package peerdas_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/peerdas"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestValidatorsCustodyRequirement(t *testing.T) {
	testCases := []struct {
		name     string
		count    uint64
		expected uint64
	}{
		{name: "0 validators", count: 0, expected: 8},
		{name: "1 validator", count: 1, expected: 8},
		{name: "8 validators", count: 8, expected: 8},
		{name: "9 validators", count: 9, expected: 9},
		{name: "100 validators", count: 100, expected: 100},
		{name: "128 validators", count: 128, expected: 128},
		{name: "129 validators", count: 129, expected: 128},
		{name: "1000 validators", count: 1000, expected: 128},
	}

	const balance = uint64(32_000_000_000)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validators := make([]*ethpb.Validator, 0, tc.count)
			for range tc.count {
				validator := &ethpb.Validator{
					EffectiveBalance: balance,
				}

				validators = append(validators, validator)
			}

			validatorsIndex := make(map[primitives.ValidatorIndex]bool)
			for i := range tc.count {
				validatorsIndex[primitives.ValidatorIndex(i)] = true
			}

			beaconState, err := state_native.InitializeFromProtoFulu(&ethpb.BeaconStateElectra{Validators: validators})
			require.NoError(t, err)

			actual, err := peerdas.ValidatorsCustodyRequirement(beaconState, validatorsIndex)
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}
