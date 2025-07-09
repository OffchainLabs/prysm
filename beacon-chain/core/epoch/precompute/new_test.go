package precompute_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/epoch/precompute"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestNew(t *testing.T) {
	ffe := params.BeaconConfig().FarFutureEpoch
	s, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot: params.BeaconConfig().SlotsPerEpoch,
		// Validator 0 is slashed
		// Validator 1 is withdrawable
		// Validator 2 is active prev epoch and current epoch
		// Validator 3 is active prev epoch
		Validators: []*ethpb.Validator{
			{Slashed: true, WithdrawableEpoch: ffe, EffectiveBalance: 100},
			{EffectiveBalance: 100},
			{WithdrawableEpoch: ffe, ExitEpoch: ffe, EffectiveBalance: 100},
			{WithdrawableEpoch: ffe, ExitEpoch: 1, EffectiveBalance: 100},
		},
	})
	require.NoError(t, err)
	e := params.BeaconConfig().FarFutureSlot
	v, b, err := precompute.New(t.Context(), s)
	require.NoError(t, err)
	assert.DeepEqual(t, &precompute.Validator{
		IsSlashed:                    true,
		CurrentEpochEffectiveBalance: 100,
		InclusionDistance:            e,
		InclusionSlot:                e,
	}, v[0], "Incorrect validator 0 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: 100,
		InclusionDistance:            e,
		InclusionSlot:                e,
	}, v[1], "Incorrect validator 1 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		CurrentEpochEffectiveBalance: 100,
		InclusionDistance:            e,
		InclusionSlot:                e,
	}, v[2], "Incorrect validator 2 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsActivePrevEpoch:            true,
		CurrentEpochEffectiveBalance: 100,
		InclusionDistance:            e,
		InclusionSlot:                e,
	}, v[3], "Incorrect validator 3 status")

	wantedBalances := &precompute.Balance{
		ActiveCurrentEpoch: 100,
		ActivePrevEpoch:    200,
	}
	assert.DeepEqual(t, wantedBalances, b, "Incorrect wanted balance")
}
