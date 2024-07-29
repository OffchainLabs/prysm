package epbs_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/altair"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/epbs"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestValidatorFlag_Remove(t *testing.T) {
	tests := []struct {
		name          string
		add           []uint8
		remove        []uint8
		expectedTrue  []uint8
		expectedFalse []uint8
	}{
		{
			name:          "none",
			add:           []uint8{},
			remove:        []uint8{},
			expectedTrue:  []uint8{},
			expectedFalse: []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
		},
		{
			name:          "source",
			add:           []uint8{params.BeaconConfig().TimelySourceFlagIndex},
			remove:        []uint8{params.BeaconConfig().TimelySourceFlagIndex},
			expectedTrue:  []uint8{},
			expectedFalse: []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
		},
		{
			name:          "source, target",
			add:           []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex},
			remove:        []uint8{params.BeaconConfig().TimelySourceFlagIndex},
			expectedTrue:  []uint8{params.BeaconConfig().TimelyTargetFlagIndex},
			expectedFalse: []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
		},
		{
			name:          "source, target, head",
			add:           []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
			remove:        []uint8{params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
			expectedTrue:  []uint8{params.BeaconConfig().TimelySourceFlagIndex},
			expectedFalse: []uint8{params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
		},
	}
	var err error
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flag := uint8(0)

			// Add flags.
			for _, flagPosition := range test.add {
				flag, err = altair.AddValidatorFlag(flag, flagPosition)
				require.NoError(t, err)

				has, err := altair.HasValidatorFlag(flag, flagPosition)
				require.NoError(t, err)
				require.Equal(t, true, has)
			}

			// Remove flags.
			for _, flagPosition := range test.remove {
				flag, err = epbs.RemoveValidatorFlag(flag, flagPosition)
				require.NoError(t, err)
			}

			// Check if flags are set correctly.
			for _, flagPosition := range test.expectedTrue {
				has, err := altair.HasValidatorFlag(flag, flagPosition)
				require.NoError(t, err)
				require.Equal(t, true, has)
			}
			for _, flagPosition := range test.expectedFalse {
				has, err := altair.HasValidatorFlag(flag, flagPosition)
				require.NoError(t, err)
				require.Equal(t, false, has)
			}
		})
	}
}

func TestValidatorFlag_Remove_ExceedsLength(t *testing.T) {
	_, err := epbs.RemoveValidatorFlag(0, 8)
	require.ErrorContains(t, "flag position 8 exceeds length", err)
}

func TestValidatorFlag_Remove_NotSet(t *testing.T) {
	_, err := epbs.RemoveValidatorFlag(0, 1)
	require.NoError(t, err)
}
