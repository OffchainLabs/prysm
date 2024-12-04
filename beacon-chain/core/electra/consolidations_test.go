package electra_test

import (
	"context"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/electra"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
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

func TestProcessConsolidationRequests(t *testing.T) {
	tests := []struct {
		name     string
		state    state.BeaconState
		reqs     []*enginev1.ConsolidationRequest
		validate func(*testing.T, state.BeaconState)
	}{
		{
			name: "one valid request",
			state: func() state.BeaconState {
				st := &eth.BeaconStateElectra{
					Slot:       params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().ShardCommitteePeriod)),
					Validators: createValidatorsWithTotalActiveBalance(32000000000000000), // 32M ETH
				}
				// Validator scenario setup. See comments in reqs section.
				st.Validators[3].WithdrawalCredentials = bytesutil.Bytes32(0)
				st.Validators[8].WithdrawalCredentials = bytesutil.Bytes32(0)
				st.Validators[9].ActivationEpoch = params.BeaconConfig().FarFutureEpoch
				st.Validators[12].ActivationEpoch = params.BeaconConfig().FarFutureEpoch
				st.Validators[13].ExitEpoch = 10
				st.Validators[16].ExitEpoch = 10
				st.PendingPartialWithdrawals = []*eth.PendingPartialWithdrawal{
					{
						Index:  17,
						Amount: 100,
					},
				}
				s, err := state_native.InitializeFromProtoElectra(st)
				require.NoError(t, err)
				return s
			}(),
			reqs: []*enginev1.ConsolidationRequest{
				// Source doesn't have withdrawal credentials.
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(1)),
					SourcePubkey:  []byte("val_3"),
					TargetPubkey:  []byte("val_4"),
				},
				// Source withdrawal credentials don't match the consolidation address.
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(0)), // Should be 5
					SourcePubkey:  []byte("val_5"),
					TargetPubkey:  []byte("val_6"),
				},
				// Target does not have their withdrawal credentials set appropriately.
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(7)),
					SourcePubkey:  []byte("val_7"),
					TargetPubkey:  []byte("val_8"),
				},
				// Source is inactive.
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(9)),
					SourcePubkey:  []byte("val_9"),
					TargetPubkey:  []byte("val_10"),
				},
				// Target is inactive.
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(11)),
					SourcePubkey:  []byte("val_11"),
					TargetPubkey:  []byte("val_12"),
				},
				// Source is exiting.
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(13)),
					SourcePubkey:  []byte("val_13"),
					TargetPubkey:  []byte("val_14"),
				},
				// Target is exiting.
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(15)),
					SourcePubkey:  []byte("val_15"),
					TargetPubkey:  []byte("val_16"),
				},
				// Source doesn't exist
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(0)),
					SourcePubkey:  []byte("INVALID"),
					TargetPubkey:  []byte("val_0"),
				},
				// Target doesn't exist
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(0)),
					SourcePubkey:  []byte("val_0"),
					TargetPubkey:  []byte("INVALID"),
				},
				// Source == target
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(0)),
					SourcePubkey:  []byte("val_0"),
					TargetPubkey:  []byte("val_0"),
				},
				// Has pending partial withdrawal
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(0)),
					SourcePubkey:  []byte("val_17"),
					TargetPubkey:  []byte("val_1"),
				},
				// Valid consolidation request. This should be last to ensure invalid requests do
				// not end the processing early.
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(1)),
					SourcePubkey:  []byte("val_1"),
					TargetPubkey:  []byte("val_2"),
				},
			},
			validate: func(t *testing.T, st state.BeaconState) {
				// Verify a pending consolidation is created.
				numPC, err := st.NumPendingConsolidations()
				require.NoError(t, err)
				require.Equal(t, uint64(1), numPC)
				pcs, err := st.PendingConsolidations()
				require.NoError(t, err)
				require.Equal(t, primitives.ValidatorIndex(1), pcs[0].SourceIndex)
				require.Equal(t, primitives.ValidatorIndex(2), pcs[0].TargetIndex)

				// Verify the source validator is exiting.
				src, err := st.ValidatorAtIndex(1)
				require.NoError(t, err)
				require.NotEqual(t, params.BeaconConfig().FarFutureEpoch, src.ExitEpoch, "source validator exit epoch not updated")
				require.Equal(t, params.BeaconConfig().MinValidatorWithdrawabilityDelay, src.WithdrawableEpoch-src.ExitEpoch, "source validator withdrawable epoch not set correctly")
			},
		},
		{
			name: "pending consolidations limit reached",
			state: func() state.BeaconState {
				st := &eth.BeaconStateElectra{
					Validators:            createValidatorsWithTotalActiveBalance(32000000000000000), // 32M ETH
					PendingConsolidations: make([]*eth.PendingConsolidation, params.BeaconConfig().PendingConsolidationsLimit),
				}
				s, err := state_native.InitializeFromProtoElectra(st)
				require.NoError(t, err)
				return s
			}(),
			reqs: []*enginev1.ConsolidationRequest{
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(1)),
					SourcePubkey:  []byte("val_1"),
					TargetPubkey:  []byte("val_2"),
				},
			},
			validate: func(t *testing.T, st state.BeaconState) {
				// Verify no pending consolidation is created.
				numPC, err := st.NumPendingConsolidations()
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().PendingConsolidationsLimit, numPC)

				// Verify the source validator is not exiting.
				src, err := st.ValidatorAtIndex(1)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().FarFutureEpoch, src.ExitEpoch, "source validator exit epoch should not be updated")
				require.Equal(t, params.BeaconConfig().FarFutureEpoch, src.WithdrawableEpoch, "source validator withdrawable epoch should not be updated")
			},
		},
		{
			name: "pending consolidations limit reached during processing",
			state: func() state.BeaconState {
				st := &eth.BeaconStateElectra{
					Slot:                  params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().ShardCommitteePeriod)),
					Validators:            createValidatorsWithTotalActiveBalance(32000000000000000), // 32M ETH
					PendingConsolidations: make([]*eth.PendingConsolidation, params.BeaconConfig().PendingConsolidationsLimit-1),
				}
				s, err := state_native.InitializeFromProtoElectra(st)
				require.NoError(t, err)
				return s
			}(),
			reqs: []*enginev1.ConsolidationRequest{
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(1)),
					SourcePubkey:  []byte("val_1"),
					TargetPubkey:  []byte("val_2"),
				},
				{
					SourceAddress: append(bytesutil.PadTo(nil, 19), byte(3)),
					SourcePubkey:  []byte("val_3"),
					TargetPubkey:  []byte("val_4"),
				},
			},
			validate: func(t *testing.T, st state.BeaconState) {
				// Verify a pending consolidation is created.
				numPC, err := st.NumPendingConsolidations()
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().PendingConsolidationsLimit, numPC)

				// The first consolidation was appended.
				pcs, err := st.PendingConsolidations()
				require.NoError(t, err)
				require.Equal(t, primitives.ValidatorIndex(1), pcs[params.BeaconConfig().PendingConsolidationsLimit-1].SourceIndex)
				require.Equal(t, primitives.ValidatorIndex(2), pcs[params.BeaconConfig().PendingConsolidationsLimit-1].TargetIndex)

				// Verify the second source validator is not exiting.
				src, err := st.ValidatorAtIndex(3)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().FarFutureEpoch, src.ExitEpoch, "source validator exit epoch should not be updated")
				require.Equal(t, params.BeaconConfig().FarFutureEpoch, src.WithdrawableEpoch, "source validator withdrawable epoch should not be updated")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := electra.ProcessConsolidationRequests(context.TODO(), tt.state, tt.reqs)
			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, tt.state)
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
