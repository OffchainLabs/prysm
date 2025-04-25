package electra_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/config/params"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestProcessEffectiveBalanceUpdates_SafeCopy(t *testing.T) {
	pb := &eth.BeaconStateElectra{
		Validators: []*eth.Validator{
			{
				EffectiveBalance:      params.BeaconConfig().MinActivationBalance,
				WithdrawalCredentials: []byte{params.BeaconConfig().CompoundingWithdrawalPrefixByte, 0x11},
			},
		},
		Balances: []uint64{
			params.BeaconConfig().MaxEffectiveBalanceElectra * 2,
		},
	}
	st, err := state_native.InitializeFromProtoElectra(pb)
	require.NoError(t, err)
	copiedState := st.Copy()

	err = electra.ProcessEffectiveBalanceUpdates(copiedState)
	require.NoError(t, err)

	require.Equal(t, st.Validators()[0].EffectiveBalance, params.BeaconConfig().MinActivationBalance)
	require.Equal(t, copiedState.Validators()[0].EffectiveBalance, params.BeaconConfig().MaxEffectiveBalanceElectra)
}

func TestProcessEffectiveBalnceUpdates(t *testing.T) {
	effBalanceInc := params.BeaconConfig().EffectiveBalanceIncrement
	hysteresisInc := effBalanceInc / params.BeaconConfig().HysteresisQuotient
	downwardThreshold := hysteresisInc * params.BeaconConfig().HysteresisDownwardMultiplier
	upwardThreshold := hysteresisInc * params.BeaconConfig().HysteresisUpwardMultiplier

	tests := []struct {
		name    string
		state   state.BeaconState
		wantErr bool
		check   func(*testing.T, state.BeaconState)
	}{
		{
			name: "validator with compounding withdrawal credentials updates effective balance",
			state: func() state.BeaconState {
				pb := &eth.BeaconStateElectra{
					Validators: []*eth.Validator{
						{
							EffectiveBalance:      params.BeaconConfig().MinActivationBalance,
							WithdrawalCredentials: []byte{params.BeaconConfig().CompoundingWithdrawalPrefixByte, 0x11},
						},
					},
					Balances: []uint64{
						params.BeaconConfig().MaxEffectiveBalanceElectra * 2,
					},
				}
				st, err := state_native.InitializeFromProtoElectra(pb)
				require.NoError(t, err)
				return st
			}(),
			check: func(t *testing.T, bs state.BeaconState) {
				val, err := bs.ValidatorAtIndex(0)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MaxEffectiveBalanceElectra, val.EffectiveBalance)
			},
		},
		{
			name: "validator without compounding withdrawal credentials updates effective balance",
			state: func() state.BeaconState {
				pb := &eth.BeaconStateElectra{
					Validators: []*eth.Validator{
						{
							EffectiveBalance:      params.BeaconConfig().MinActivationBalance / 2,
							WithdrawalCredentials: make([]byte, 32),
						},
					},
					Balances: []uint64{
						params.BeaconConfig().MaxEffectiveBalanceElectra,
					},
				}
				st, err := state_native.InitializeFromProtoElectra(pb)
				require.NoError(t, err)
				return st
			}(),
			check: func(t *testing.T, bs state.BeaconState) {
				val, err := bs.ValidatorAtIndex(0)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MinActivationBalance, val.EffectiveBalance)
			},
		},
		{
			name: "validator effective balance moves only when outside of threshold",
			state: func() state.BeaconState {
				pb := &eth.BeaconStateElectra{
					Validators: []*eth.Validator{
						{
							EffectiveBalance:      params.BeaconConfig().MinActivationBalance,
							WithdrawalCredentials: []byte{params.BeaconConfig().CompoundingWithdrawalPrefixByte, 0x11},
						},
						{
							EffectiveBalance:      params.BeaconConfig().MinActivationBalance,
							WithdrawalCredentials: []byte{params.BeaconConfig().CompoundingWithdrawalPrefixByte, 0x11},
						},
						{
							EffectiveBalance:      params.BeaconConfig().MinActivationBalance,
							WithdrawalCredentials: []byte{params.BeaconConfig().CompoundingWithdrawalPrefixByte, 0x11},
						},
						{
							EffectiveBalance:      params.BeaconConfig().MinActivationBalance,
							WithdrawalCredentials: []byte{params.BeaconConfig().CompoundingWithdrawalPrefixByte, 0x11},
						},
					},
					Balances: []uint64{
						params.BeaconConfig().MinActivationBalance - downwardThreshold - 1, // beyond downward threshold
						params.BeaconConfig().MinActivationBalance - downwardThreshold + 1, // within downward threshold
						params.BeaconConfig().MinActivationBalance + upwardThreshold + 1,   // beyond upward threshold
						params.BeaconConfig().MinActivationBalance + upwardThreshold - 1,   // within upward threshold
					},
				}
				st, err := state_native.InitializeFromProtoElectra(pb)
				require.NoError(t, err)
				return st
			}(),
			check: func(t *testing.T, bs state.BeaconState) {
				// validator 0 has a balance diff exceeding the threshold so a diff should be applied to
				// effective balance and it moves by effective balance increment.
				val, err := bs.ValidatorAtIndex(0)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MinActivationBalance-params.BeaconConfig().EffectiveBalanceIncrement, val.EffectiveBalance)

				// validator 1 has a balance diff within the threshold so the effective balance should not
				// have changed.
				val, err = bs.ValidatorAtIndex(1)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MinActivationBalance, val.EffectiveBalance)

				// Validator 2 has a balance diff exceeding the threshold so a diff should be applied to the
				// effective balance and it moves by effective balance increment.
				val, err = bs.ValidatorAtIndex(2)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MinActivationBalance+params.BeaconConfig().EffectiveBalanceIncrement, val.EffectiveBalance)

				// Validator 3 has a balance diff within the threshold so the effective balance should not
				// have changed.
				val, err = bs.ValidatorAtIndex(3)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MinActivationBalance, val.EffectiveBalance)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := electra.ProcessEffectiveBalanceUpdates(tt.state)
			require.Equal(t, tt.wantErr, err != nil, "unexpected error returned wanted error=nil (%s), got error=%s", tt.wantErr, err)
			if tt.check != nil {
				tt.check(t, tt.state)
			}
		})
	}
}
