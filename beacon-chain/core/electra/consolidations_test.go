package electra_test

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/electra"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestProcessPendingConsolidations(t *testing.T) {
	tests := []struct {
		name    string
		state   state.BeaconState
		check   func(*testing.T, state.BeaconState)
		wantErr bool
	}{
		{
			name:    "nil state",
			state:   nil,
			wantErr: true,
		},
		{
			name: "no pending consolidations",
			state: func() state.BeaconState {
				pb := &eth.BeaconStateElectra{}

				st, err := state_native.InitializeFromProtoUnsafeElectra(pb)
				require.NoError(t, err)
				return st
			}(),
			wantErr: false,
		},
		{
			name: "processes pending consolidation successfully",
			state: func() state.BeaconState {
				pb := &eth.BeaconStateElectra{
					Validators: []*eth.Validator{
						{
							WithdrawalCredentials: []byte{0x01, 0xFF},
							EffectiveBalance:      params.BeaconConfig().MinActivationBalance,
						},
						{
							WithdrawalCredentials: []byte{0x01, 0xAB},
						},
					},
					Balances: []uint64{
						params.BeaconConfig().MinActivationBalance,
						params.BeaconConfig().MinActivationBalance,
					},
					PendingConsolidations: []*eth.PendingConsolidation{
						{
							SourceIndex: 0,
							TargetIndex: 1,
						},
					},
				}

				st, err := state_native.InitializeFromProtoUnsafeElectra(pb)
				require.NoError(t, err)
				return st
			}(),
			check: func(t *testing.T, st state.BeaconState) {
				// Balances are transferred from v0 to v1.
				bal0, err := st.BalanceAtIndex(0)
				require.NoError(t, err)
				require.Equal(t, uint64(0), bal0)
				bal1, err := st.BalanceAtIndex(1)
				require.NoError(t, err)
				require.Equal(t, 2*params.BeaconConfig().MinActivationBalance, bal1)

				// The pending consolidation is removed from the list.
				num, err := st.NumPendingConsolidations()
				require.NoError(t, err)
				require.Equal(t, uint64(0), num)

				// v1 withdrawal credentials should not be updated.
				v1, err := st.ValidatorAtIndex(1)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().ETH1AddressWithdrawalPrefixByte, v1.WithdrawalCredentials[0])
			},
			wantErr: false,
		},
		{
			name: "stop processing when a source val withdrawable epoch is in the future",
			state: func() state.BeaconState {
				pb := &eth.BeaconStateElectra{
					Validators: []*eth.Validator{
						{
							WithdrawalCredentials: []byte{0x01, 0xFF},
							WithdrawableEpoch:     100,
						},
						{
							WithdrawalCredentials: []byte{0x01, 0xAB},
						},
					},
					Balances: []uint64{
						params.BeaconConfig().MinActivationBalance,
						params.BeaconConfig().MinActivationBalance,
					},
					PendingConsolidations: []*eth.PendingConsolidation{
						{
							SourceIndex: 0,
							TargetIndex: 1,
						},
					},
				}

				st, err := state_native.InitializeFromProtoUnsafeElectra(pb)
				require.NoError(t, err)
				return st
			}(),
			check: func(t *testing.T, st state.BeaconState) {
				// No balances are transferred from v0 to v1.
				bal0, err := st.BalanceAtIndex(0)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MinActivationBalance, bal0)
				bal1, err := st.BalanceAtIndex(1)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MinActivationBalance, bal1)

				// The pending consolidation is still in the list.
				num, err := st.NumPendingConsolidations()
				require.NoError(t, err)
				require.Equal(t, uint64(1), num)
			},
			wantErr: false,
		},
		{
			name: "slashed validator is not consolidated",
			state: func() state.BeaconState {
				pb := &eth.BeaconStateElectra{
					Validators: []*eth.Validator{
						{
							WithdrawalCredentials: []byte{0x01, 0xFF},
						},
						{
							WithdrawalCredentials: []byte{0x01, 0xAB},
						},
						{
							Slashed: true,
						},
						{
							WithdrawalCredentials: []byte{0x01, 0xCC},
						},
					},
					Balances: []uint64{
						params.BeaconConfig().MinActivationBalance,
						params.BeaconConfig().MinActivationBalance,
						params.BeaconConfig().MinActivationBalance,
						params.BeaconConfig().MinActivationBalance,
					},
					PendingConsolidations: []*eth.PendingConsolidation{
						{
							SourceIndex: 2,
							TargetIndex: 3,
						},
						{
							SourceIndex: 0,
							TargetIndex: 1,
						},
					},
				}

				st, err := state_native.InitializeFromProtoUnsafeElectra(pb)
				require.NoError(t, err)
				return st
			}(),
			check: func(t *testing.T, st state.BeaconState) {
				// No balances are transferred from v2 to v3.
				bal0, err := st.BalanceAtIndex(2)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MinActivationBalance, bal0)
				bal1, err := st.BalanceAtIndex(3)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MinActivationBalance, bal1)

				// No pending consolidation remaining.
				num, err := st.NumPendingConsolidations()
				require.NoError(t, err)
				require.Equal(t, uint64(0), num)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := electra.ProcessPendingConsolidations(context.TODO(), tt.state)
			require.Equal(t, tt.wantErr, err != nil)
			if tt.check != nil {
				tt.check(t, tt.state)
			}
		})
	}
}

func TestIsValidSwitchToCompoundingRequest(t *testing.T) {
	st, _ := util.DeterministicGenesisStateElectra(t, 1)
	t.Run("nil source pubkey", func(t *testing.T) {
		ok := electra.IsValidSwitchToCompoundingRequest(st, &enginev1.ConsolidationRequest{
			SourcePubkey: nil,
			TargetPubkey: []byte{'a'},
		})
		require.Equal(t, false, ok)
	})
	t.Run("nil target pubkey", func(t *testing.T) {
		ok := electra.IsValidSwitchToCompoundingRequest(st, &enginev1.ConsolidationRequest{
			TargetPubkey: nil,
			SourcePubkey: []byte{'a'},
		})
		require.Equal(t, false, ok)
	})
	t.Run("different source and target pubkey", func(t *testing.T) {
		ok := electra.IsValidSwitchToCompoundingRequest(st, &enginev1.ConsolidationRequest{
			TargetPubkey: []byte{'a'},
			SourcePubkey: []byte{'b'},
		})
		require.Equal(t, false, ok)
	})
	t.Run("source validator not found in state", func(t *testing.T) {
		ok := electra.IsValidSwitchToCompoundingRequest(st, &enginev1.ConsolidationRequest{
			SourceAddress: make([]byte, 20),
			TargetPubkey:  []byte{'a'},
			SourcePubkey:  []byte{'a'},
		})
		require.Equal(t, false, ok)
	})
	t.Run("incorrect source address", func(t *testing.T) {
		v, err := st.ValidatorAtIndex(0)
		require.NoError(t, err)
		pubkey := v.PublicKey
		ok := electra.IsValidSwitchToCompoundingRequest(st, &enginev1.ConsolidationRequest{
			SourceAddress: make([]byte, 20),
			TargetPubkey:  pubkey,
			SourcePubkey:  pubkey,
		})
		require.Equal(t, false, ok)
	})
	t.Run("incorrect eth1 withdrawal credential", func(t *testing.T) {
		v, err := st.ValidatorAtIndex(0)
		require.NoError(t, err)
		pubkey := v.PublicKey
		wc := v.WithdrawalCredentials
		ok := electra.IsValidSwitchToCompoundingRequest(st, &enginev1.ConsolidationRequest{
			SourceAddress: wc[12:],
			TargetPubkey:  pubkey,
			SourcePubkey:  pubkey,
		})
		require.Equal(t, false, ok)
	})
	t.Run("is valid compounding request", func(t *testing.T) {
		v, err := st.ValidatorAtIndex(0)
		require.NoError(t, err)
		pubkey := v.PublicKey
		wc := v.WithdrawalCredentials
		v.WithdrawalCredentials[0] = 1
		require.NoError(t, st.UpdateValidatorAtIndex(0, v))
		ok := electra.IsValidSwitchToCompoundingRequest(st, &enginev1.ConsolidationRequest{
			SourceAddress: wc[12:],
			TargetPubkey:  pubkey,
			SourcePubkey:  pubkey,
		})
		require.Equal(t, true, ok)
	})
	t.Run("already has an exit epoch", func(t *testing.T) {
		v, err := st.ValidatorAtIndex(0)
		require.NoError(t, err)
		pubkey := v.PublicKey
		wc := v.WithdrawalCredentials
		v.ExitEpoch = 100
		require.NoError(t, st.UpdateValidatorAtIndex(0, v))
		ok := electra.IsValidSwitchToCompoundingRequest(st, &enginev1.ConsolidationRequest{
			SourceAddress: wc[12:],
			TargetPubkey:  pubkey,
			SourcePubkey:  pubkey,
		})
		require.Equal(t, false, ok)
	})
}
