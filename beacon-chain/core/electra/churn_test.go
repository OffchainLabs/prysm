package electra_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/electra"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
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
			PublicKey:             []byte(fmt.Sprintf("val_%d", i)),
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

func TestComputeConsolidationEpochAndUpdateChurn(t *testing.T) {
	// Test setup: create a state with 32M ETH total active balance.
	// In this state, the churn is expected to be 232 ETH per epoch.
	tests := []struct {
		name                                  string
		state                                 state.BeaconState
		consolidationBalance                  primitives.Gwei
		expectedEpoch                         primitives.Epoch
		expectedConsolidationBalanceToConsume primitives.Gwei
	}{
		{
			name: "compute consolidation with no consolidation balance",
			state: func(t *testing.T) state.BeaconState {
				s, err := state_native.InitializeFromProtoUnsafeElectra(&eth.BeaconStateElectra{
					Slot:                       slots.UnsafeEpochStart(10),
					EarliestConsolidationEpoch: 9,
					Validators:                 createValidatorsWithTotalActiveBalance(32000000000000000), // 32M ETH
				})
				require.NoError(t, err)
				return s
			}(t),
			consolidationBalance:                  0,            // 0 ETH
			expectedEpoch:                         15,           // current epoch + 1 + MaxSeedLookahead
			expectedConsolidationBalanceToConsume: 232000000000, // 232 ETH
		},
		{
			name: "new epoch for consolidations",
			state: func(t *testing.T) state.BeaconState {
				s, err := state_native.InitializeFromProtoUnsafeElectra(&eth.BeaconStateElectra{
					Slot:                       slots.UnsafeEpochStart(10),
					EarliestConsolidationEpoch: 9,
					Validators:                 createValidatorsWithTotalActiveBalance(32000000000000000), // 32M ETH
				})
				require.NoError(t, err)
				return s
			}(t),
			consolidationBalance:                  32000000000,  // 32 ETH
			expectedEpoch:                         15,           // current epoch + 1 + MaxSeedLookahead
			expectedConsolidationBalanceToConsume: 200000000000, // 200 ETH
		},
		{
			name: "flows into another epoch",
			state: func(t *testing.T) state.BeaconState {
				s, err := state_native.InitializeFromProtoUnsafeElectra(&eth.BeaconStateElectra{
					Slot:                       slots.UnsafeEpochStart(10),
					EarliestConsolidationEpoch: 9,
					Validators:                 createValidatorsWithTotalActiveBalance(32000000000000000), // 32M ETH
				})
				require.NoError(t, err)
				return s
			}(t),
			consolidationBalance:                  235000000000, // 235 ETH
			expectedEpoch:                         16,           // Flows into another epoch.
			expectedConsolidationBalanceToConsume: 229000000000, // 229 ETH
		},
		{
			name: "not a new epoch, fits in remaining balance of current epoch",
			state: func(t *testing.T) state.BeaconState {
				s, err := state_native.InitializeFromProtoUnsafeElectra(&eth.BeaconStateElectra{
					Slot:                          slots.UnsafeEpochStart(10),
					EarliestConsolidationEpoch:    15,
					ConsolidationBalanceToConsume: 200000000000,                                              // 200 ETH
					Validators:                    createValidatorsWithTotalActiveBalance(32000000000000000), // 32M ETH
				})
				require.NoError(t, err)
				return s
			}(t),
			consolidationBalance:                  32000000000,  // 32 ETH
			expectedEpoch:                         15,           // Fits into current earliest consolidation epoch.
			expectedConsolidationBalanceToConsume: 168000000000, // 126 ETH
		},
		{
			name: "not a new epoch, fits in remaining balance of current epoch",
			state: func(t *testing.T) state.BeaconState {
				s, err := state_native.InitializeFromProtoUnsafeElectra(&eth.BeaconStateElectra{
					Slot:                          slots.UnsafeEpochStart(10),
					EarliestConsolidationEpoch:    15,
					ConsolidationBalanceToConsume: 200000000000,                                              // 200 ETH
					Validators:                    createValidatorsWithTotalActiveBalance(32000000000000000), // 32M ETH
				})
				require.NoError(t, err)
				return s
			}(t),
			consolidationBalance:                  232000000000, // 232 ETH
			expectedEpoch:                         16,           // Flows into another epoch.
			expectedConsolidationBalanceToConsume: 200000000000, // 200 ETH
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEpoch, err := electra.ComputeConsolidationEpochAndUpdateChurn(context.TODO(), tt.state, tt.consolidationBalance)
			require.NoError(t, err)
			require.Equal(t, tt.expectedEpoch, gotEpoch)
			// Check consolidation balance to consume is set on the state.
			cbtc, err := tt.state.ConsolidationBalanceToConsume()
			require.NoError(t, err)
			require.Equal(t, tt.expectedConsolidationBalanceToConsume, cbtc)
			// Check earliest consolidation epoch was set on the state.
			gotEpoch, err = tt.state.EarliestConsolidationEpoch()
			require.NoError(t, err)
			require.Equal(t, tt.expectedEpoch, gotEpoch)
		})
	}
}
