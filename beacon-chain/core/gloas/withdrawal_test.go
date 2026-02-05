package gloas

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProcessWithdrawals_ParentBlockNotFull(t *testing.T) {
	state, err := state_native.InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{})
	require.NoError(t, err)

	st := &withdrawalsState{BeaconState: state}
	require.NoError(t, ProcessWithdrawals(st))
	require.Equal(t, false, st.expectedCalled)
}

type withdrawalsState struct {
	state.BeaconState
	expectedCalled bool
	decreaseCalled bool
}

func (w *withdrawalsState) IsParentBlockFull() (bool, error) {
	return false, nil
}

func (w *withdrawalsState) ExpectedWithdrawalsGloas() (state.ExpectedWithdrawalsGloasResult, error) {
	w.expectedCalled = true
	return state.ExpectedWithdrawalsGloasResult{}, nil
}
