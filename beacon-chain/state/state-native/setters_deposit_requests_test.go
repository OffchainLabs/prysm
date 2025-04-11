package state_native_test

import (
	"testing"

	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestSetDepositRequestsStartIndex(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		dState, _ := util.DeterministicGenesisState(t, 1)
		require.ErrorContains(t, "is not supported", dState.SetDepositRequestsStartIndex(1))
	})
	t.Run("electra sets expected value", func(t *testing.T) {
		old := uint64(2)
		dState, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{DepositRequestsStartIndex: old})
		require.NoError(t, err)
		want := uint64(3)
		require.NoError(t, dState.SetDepositRequestsStartIndex(want))
		got, err := dState.DepositRequestsStartIndex()
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}
