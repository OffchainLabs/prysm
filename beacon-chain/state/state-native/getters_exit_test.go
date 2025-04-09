package state_native_test

import (
	"testing"

	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestExitBalanceToConsume(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		dState, _ := util.DeterministicGenesisState(t, 1)
		_, err := dState.ExitBalanceToConsume()
		require.ErrorContains(t, "is not supported", err)
	})
	t.Run("electra returns expected value", func(t *testing.T) {
		want := primitives.Gwei(2)
		dState, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{ExitBalanceToConsume: want})
		require.NoError(t, err)
		got, err := dState.ExitBalanceToConsume()
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}

func TestEarliestExitEpoch(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		dState, _ := util.DeterministicGenesisState(t, 1)
		_, err := dState.EarliestExitEpoch()
		require.ErrorContains(t, "is not supported", err)
	})
	t.Run("electra returns expected value", func(t *testing.T) {
		want := primitives.Epoch(2)
		dState, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{EarliestExitEpoch: want})
		require.NoError(t, err)
		got, err := dState.EarliestExitEpoch()
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}
