package requests_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/requests"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func createValidatorsWithTotalActiveBalance(totalBal primitives.Gwei) []*eth.Validator {
	num := totalBal / primitives.Gwei(params.BeaconConfig().MinActivationBalance)
	vals := make([]*eth.Validator, num)
	for i := range vals {
		wd := make([]byte, 32)
		wd[0] = params.BeaconConfig().CompoundingWithdrawalPrefixByte
		wd[31] = byte(i)

		vals[i] = &eth.Validator{
			ActivationEpoch:       primitives.Epoch(0),
			EffectiveBalance:      params.BeaconConfig().MinActivationBalance,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			PublicKey:             fmt.Appendf(nil, "val_%d", i),
			WithdrawableEpoch:     params.BeaconConfig().FarFutureEpoch,
			WithdrawalCredentials: wd,
		}
	}
	if totalBal%primitives.Gwei(params.BeaconConfig().MinActivationBalance) != 0 {
		vals = append(vals, &eth.Validator{
			ActivationEpoch:  primitives.Epoch(0),
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: uint64(totalBal) % params.BeaconConfig().MinActivationBalance,
		})
	}
	return vals
}

func TestProcessConsolidationRequests(t *testing.T) {
	tests := []struct {
		name     string
		state    state.BeaconState
		reqs     []*enginev1.ConsolidationRequest
		validate func(*testing.T, state.BeaconState)
		wantErr  bool
	}{
		{
			name: "nil request",
			state: func() state.BeaconState {
				st := &eth.BeaconStateElectra{}
				s, err := state_native.InitializeFromProtoElectra(st)
				require.NoError(t, err)
				return s
			}(),
			reqs: []*enginev1.ConsolidationRequest{nil},
			validate: func(t *testing.T, st state.BeaconState) {
				require.DeepEqual(t, st, st)
			},
			wantErr: true,
		},
		{
			name: "one valid request",
			state: func() state.BeaconState {
				st := &eth.BeaconStateElectra{
					Slot:       params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().ShardCommitteePeriod)),
					Validators: createValidatorsWithTotalActiveBalance(32000000000000000), // 32M ETH
				}
				// Validator scenario setup. See comments in reqs section.
				st.Validators[3].WithdrawalCredentials = bytesutil.Bytes32(0)
				st.Validators[8].WithdrawalCredentials = bytesutil.Bytes32(1)
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
				// Target does not have their withdrawal credentials set appropriately. (Using eth1 address prefix)
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
			wantErr: false,
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
			wantErr: false,
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
			wantErr: false,
		},
		{
			name: "pending consolidations limit reached and compounded consolidation after",
			state: func() state.BeaconState {
				st := &eth.BeaconStateElectra{
					Slot:                  params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().ShardCommitteePeriod)),
					Validators:            createValidatorsWithTotalActiveBalance(32000000000000000), // 32M ETH
					PendingConsolidations: make([]*eth.PendingConsolidation, params.BeaconConfig().PendingConsolidationsLimit),
				}
				// To allow compounding consolidation requests.
				st.Validators[3].WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
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
					TargetPubkey:  []byte("val_3"),
				},
			},
			validate: func(t *testing.T, st state.BeaconState) {
				// Verify a pending consolidation is created.
				numPC, err := st.NumPendingConsolidations()
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().PendingConsolidationsLimit, numPC)

				// Verify that the last consolidation was included
				src, err := st.ValidatorAtIndex(3)
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().CompoundingWithdrawalPrefixByte, src.WithdrawalCredentials[0], "source validator was not compounded")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := requests.ProcessConsolidationRequests(context.TODO(), tt.state, tt.reqs)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessWithdrawalRequests() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				require.NoError(t, err)
			}
			if tt.validate != nil {
				tt.validate(t, tt.state)
			}
		})
	}
}
