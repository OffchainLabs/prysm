package state_native_test

import (
	"testing"

	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
)

func TestDepositRequestsStartIndex(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		dState, _ := util.DeterministicGenesisState(t, 1)
		_, err := dState.DepositRequestsStartIndex()
		require.ErrorContains(t, "is not supported", err)
	})
	t.Run("electra returns expected value", func(t *testing.T) {
		want := uint64(2)
		dState, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{DepositRequestsStartIndex: want})
		require.NoError(t, err)
		got, err := dState.DepositRequestsStartIndex()
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}
