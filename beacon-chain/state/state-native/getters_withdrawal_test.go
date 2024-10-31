package state_native_test

import (
	"testing"

	"github.com/golang/snappy"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

func TestNextWithdrawalIndex(t *testing.T) {
	t.Run("ok for deneb", func(t *testing.T) {
		s, err := state_native.InitializeFromProtoDeneb(&ethpb.BeaconStateDeneb{NextWithdrawalIndex: 123})
		require.NoError(t, err)
		i, err := s.NextWithdrawalIndex()
		require.NoError(t, err)
		assert.Equal(t, uint64(123), i)
	})
	t.Run("ok", func(t *testing.T) {
		s, err := state_native.InitializeFromProtoCapella(&ethpb.BeaconStateCapella{NextWithdrawalIndex: 123})
		require.NoError(t, err)
		i, err := s.NextWithdrawalIndex()
		require.NoError(t, err)
		assert.Equal(t, uint64(123), i)
	})
	t.Run("version before Capella not supported", func(t *testing.T) {
		s, err := state_native.InitializeFromProtoBellatrix(&ethpb.BeaconStateBellatrix{})
		require.NoError(t, err)
		_, err = s.NextWithdrawalIndex()
		assert.ErrorContains(t, "NextWithdrawalIndex is not supported", err)
	})
}

func TestNextWithdrawalValidatorIndex(t *testing.T) {
	t.Run("ok for deneb", func(t *testing.T) {
		pb := &ethpb.BeaconStateDeneb{NextWithdrawalValidatorIndex: 123}
		s, err := state_native.InitializeFromProtoDeneb(pb)
		require.NoError(t, err)
		i, err := s.NextWithdrawalValidatorIndex()
		require.NoError(t, err)
		assert.Equal(t, primitives.ValidatorIndex(123), i)
	})
	t.Run("ok", func(t *testing.T) {
		pb := &ethpb.BeaconStateCapella{NextWithdrawalValidatorIndex: 123}
		s, err := state_native.InitializeFromProtoCapella(pb)
		require.NoError(t, err)
		i, err := s.NextWithdrawalValidatorIndex()
		require.NoError(t, err)
		assert.Equal(t, primitives.ValidatorIndex(123), i)
	})
	t.Run("version before Capella not supported", func(t *testing.T) {
		s, err := state_native.InitializeFromProtoBellatrix(&ethpb.BeaconStateBellatrix{})
		require.NoError(t, err)
		_, err = s.NextWithdrawalValidatorIndex()
		assert.ErrorContains(t, "NextWithdrawalValidatorIndex is not supported", err)
	})
}

func TestExpectedWithdrawals(t *testing.T) {
	for _, stateVersion := range []int{version.Capella, version.Deneb, version.Electra} {
		t.Run(version.String(stateVersion), func(t *testing.T) {
			t.Run("no withdrawals", func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, stateVersion)
				vals := make([]*ethpb.Validator, 100)
				balances := make([]uint64, 100)
				for i := range vals {
					balances[i] = params.BeaconConfig().MaxEffectiveBalance
					val := &ethpb.Validator{
						WithdrawalCredentials: make([]byte, 32),
						EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
						WithdrawableEpoch:     primitives.Epoch(1),
					}
					val.WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
					vals[i] = val
				}
				require.NoError(t, s.SetValidators(vals))
				require.NoError(t, s.SetBalances(balances))
				expected, _, err := s.ExpectedWithdrawals()
				require.NoError(t, err)
				require.Equal(t, 0, len(expected))
			})
			t.Run("one fully withdrawable", func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, stateVersion)
				require.NoError(t, s.SetNextWithdrawalValidatorIndex(20))

				vals := make([]*ethpb.Validator, 100)
				balances := make([]uint64, 100)
				for i := 0; i < 100; i++ {
					balances[i] = params.BeaconConfig().MaxEffectiveBalance
					val := &ethpb.Validator{
						WithdrawalCredentials: make([]byte, 32),
						EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
						WithdrawableEpoch:     primitives.Epoch(1),
					}
					val.WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte

					vals[i] = val
				}
				vals[3].WithdrawableEpoch = primitives.Epoch(0)
				require.NoError(t, s.SetValidators(vals))
				require.NoError(t, s.SetBalances(balances))

				expected, _, err := s.ExpectedWithdrawals()
				require.NoError(t, err)
				require.Equal(t, 1, len(expected))
				withdrawal := &enginev1.Withdrawal{
					Index:          0,
					ValidatorIndex: 3,
					Address:        vals[3].WithdrawalCredentials[12:],
					Amount:         params.BeaconConfig().MaxEffectiveBalance,
				}
				require.DeepEqual(t, withdrawal, expected[0])
			})
			t.Run("one partially withdrawable", func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, stateVersion)
				require.NoError(t, s.SetNextWithdrawalValidatorIndex(20))

				vals := make([]*ethpb.Validator, 100)
				balances := make([]uint64, 100)
				for i := 0; i < 100; i++ {
					balances[i] = params.BeaconConfig().MaxEffectiveBalance
					val := &ethpb.Validator{
						WithdrawalCredentials: make([]byte, 32),
						EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
						WithdrawableEpoch:     primitives.Epoch(1),
					}
					val.WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte

					vals[i] = val
				}
				balances[3] += params.BeaconConfig().MinDepositAmount
				require.NoError(t, s.SetValidators(vals))
				require.NoError(t, s.SetBalances(balances))
				expected, _, err := s.ExpectedWithdrawals()
				require.NoError(t, err)
				require.Equal(t, 1, len(expected))
				withdrawal := &enginev1.Withdrawal{
					Index:          0,
					ValidatorIndex: 3,
					Address:        vals[3].WithdrawalCredentials[12:],
					Amount:         params.BeaconConfig().MinDepositAmount,
				}
				require.DeepEqual(t, withdrawal, expected[0])
			})
			t.Run("one partially and one fully withdrawable", func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, stateVersion)
				vals := make([]*ethpb.Validator, 100)
				balances := make([]uint64, 100)
				for i := range vals {
					balances[i] = params.BeaconConfig().MaxEffectiveBalance
					val := &ethpb.Validator{
						WithdrawalCredentials: make([]byte, 32),
						EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
						WithdrawableEpoch:     primitives.Epoch(1),
					}
					val.WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
					val.WithdrawalCredentials[31] = byte(i)
					vals[i] = val
				}
				balances[3] += params.BeaconConfig().MinDepositAmount
				vals[7].WithdrawableEpoch = primitives.Epoch(0)
				require.NoError(t, s.SetValidators(vals))
				require.NoError(t, s.SetBalances(balances))
				expected, _, err := s.ExpectedWithdrawals()
				require.NoError(t, err)
				require.Equal(t, 2, len(expected))

				withdrawalFull := &enginev1.Withdrawal{
					Index:          1,
					ValidatorIndex: 7,
					Address:        vals[7].WithdrawalCredentials[12:],
					Amount:         balances[7],
				}
				withdrawalPartial := &enginev1.Withdrawal{
					Index:          0,
					ValidatorIndex: 3,
					Address:        vals[3].WithdrawalCredentials[12:],
					Amount:         params.BeaconConfig().MinDepositAmount,
				}
				require.DeepEqual(t, withdrawalPartial, expected[0])
				require.DeepEqual(t, withdrawalFull, expected[1])
			})
			t.Run("all partially withdrawable", func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, stateVersion)
				vals := make([]*ethpb.Validator, 100)
				balances := make([]uint64, 100)
				for i := range vals {
					balances[i] = params.BeaconConfig().MaxEffectiveBalance + 1
					val := &ethpb.Validator{
						WithdrawalCredentials: make([]byte, 32),
						EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
						WithdrawableEpoch:     primitives.Epoch(1),
					}
					val.WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
					val.WithdrawalCredentials[31] = byte(i)
					vals[i] = val
				}
				require.NoError(t, s.SetValidators(vals))
				require.NoError(t, s.SetBalances(balances))
				expected, _, err := s.ExpectedWithdrawals()
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MaxWithdrawalsPerPayload, uint64(len(expected)))
				withdrawal := &enginev1.Withdrawal{
					Index:          0,
					ValidatorIndex: 0,
					Address:        vals[0].WithdrawalCredentials[12:],
					Amount:         1,
				}
				require.DeepEqual(t, withdrawal, expected[0])
			})
			t.Run("all fully withdrawable", func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, stateVersion)
				vals := make([]*ethpb.Validator, 100)
				balances := make([]uint64, 100)
				for i := range vals {
					balances[i] = params.BeaconConfig().MaxEffectiveBalance
					val := &ethpb.Validator{
						WithdrawalCredentials: make([]byte, 32),
						EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
						WithdrawableEpoch:     primitives.Epoch(0),
					}
					val.WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
					val.WithdrawalCredentials[31] = byte(i)
					vals[i] = val
				}
				require.NoError(t, s.SetValidators(vals))
				require.NoError(t, s.SetBalances(balances))
				expected, _, err := s.ExpectedWithdrawals()
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MaxWithdrawalsPerPayload, uint64(len(expected)))
				withdrawal := &enginev1.Withdrawal{
					Index:          0,
					ValidatorIndex: 0,
					Address:        vals[0].WithdrawalCredentials[12:],
					Amount:         params.BeaconConfig().MaxEffectiveBalance,
				}
				require.DeepEqual(t, withdrawal, expected[0])
			})
			t.Run("all fully and partially withdrawable", func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, stateVersion)
				vals := make([]*ethpb.Validator, 100)
				balances := make([]uint64, 100)
				for i := range vals {
					balances[i] = params.BeaconConfig().MaxEffectiveBalance + 1
					val := &ethpb.Validator{
						WithdrawalCredentials: make([]byte, 32),
						EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
						WithdrawableEpoch:     primitives.Epoch(0),
					}
					val.WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
					val.WithdrawalCredentials[31] = byte(i)
					vals[i] = val
				}
				require.NoError(t, s.SetValidators(vals))
				require.NoError(t, s.SetBalances(balances))
				expected, _, err := s.ExpectedWithdrawals()
				require.NoError(t, err)
				require.Equal(t, params.BeaconConfig().MaxWithdrawalsPerPayload, uint64(len(expected)))
				withdrawal := &enginev1.Withdrawal{
					Index:          0,
					ValidatorIndex: 0,
					Address:        vals[0].WithdrawalCredentials[12:],
					Amount:         params.BeaconConfig().MaxEffectiveBalance + 1,
				}
				require.DeepEqual(t, withdrawal, expected[0])
			})
			t.Run("one fully withdrawable but zero balance", func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, stateVersion)
				require.NoError(t, s.SetNextWithdrawalValidatorIndex(20))
				vals := make([]*ethpb.Validator, 100)
				balances := make([]uint64, 100)
				for i := range vals {
					balances[i] = params.BeaconConfig().MaxEffectiveBalance
					val := &ethpb.Validator{
						WithdrawalCredentials: make([]byte, 32),
						EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
						WithdrawableEpoch:     primitives.Epoch(1),
					}
					val.WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
					val.WithdrawalCredentials[31] = byte(i)
					vals[i] = val
				}
				vals[3].WithdrawableEpoch = primitives.Epoch(0)
				balances[3] = 0
				require.NoError(t, s.SetValidators(vals))
				require.NoError(t, s.SetBalances(balances))

				expected, _, err := s.ExpectedWithdrawals()
				require.NoError(t, err)
				require.Equal(t, 0, len(expected))
			})
			t.Run("one partially withdrawable, one above sweep bound", func(t *testing.T) {
				s := state_native.EmptyStateFromVersion(t, stateVersion)
				vals := make([]*ethpb.Validator, 100)
				balances := make([]uint64, 100)
				for i := range vals {
					balances[i] = params.BeaconConfig().MaxEffectiveBalance
					val := &ethpb.Validator{
						WithdrawalCredentials: make([]byte, 32),
						EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
						WithdrawableEpoch:     primitives.Epoch(1),
					}
					val.WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
					val.WithdrawalCredentials[31] = byte(i)
					vals[i] = val
				}
				balances[3] += params.BeaconConfig().MinDepositAmount
				balances[10] += params.BeaconConfig().MinDepositAmount
				require.NoError(t, s.SetValidators(vals))
				require.NoError(t, s.SetBalances(balances))
				saved := params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep
				params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep = 10
				expected, _, err := s.ExpectedWithdrawals()
				require.NoError(t, err)
				require.Equal(t, 1, len(expected))
				withdrawal := &enginev1.Withdrawal{
					Index:          0,
					ValidatorIndex: 3,
					Address:        vals[3].WithdrawalCredentials[12:],
					Amount:         params.BeaconConfig().MinDepositAmount,
				}
				require.DeepEqual(t, withdrawal, expected[0])
				params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep = saved
			})
		})
	}

	t.Run("electra all pending partial withdrawals", func(t *testing.T) {
		// Load a serialized Electra state from disk.
		// This spectest has a fully hydrated beacon state with partial pending withdrawals.
		serializedBytes, err := util.BazelFileBytes("tests/mainnet/electra/operations/withdrawal_request/pyspec_tests/pending_withdrawals_consume_all_excess_balance/pre.ssz_snappy")
		require.NoError(t, err)
		serializedSSZ, err := snappy.Decode(nil /* dst */, serializedBytes)
		require.NoError(t, err)
		pb := &ethpb.BeaconStateElectra{}
		require.NoError(t, pb.UnmarshalSSZ(serializedSSZ))
		s, err := state_native.InitializeFromProtoElectra(pb)
		require.NoError(t, err)
		expected, partialWithdrawalsCount, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, 8, len(expected))
		require.Equal(t, uint64(8), partialWithdrawalsCount)
	})

	t.Run("electra some pending partial withdrawals", func(t *testing.T) {
		// Load a serialized Electra state from disk.
		// This spectest has a fully hydrated beacon state with partial pending withdrawals.
		serializedBytes, err := util.BazelFileBytes("tests/mainnet/electra/operations/withdrawal_request/pyspec_tests/pending_withdrawals_consume_all_excess_balance/pre.ssz_snappy")
		require.NoError(t, err)
		serializedSSZ, err := snappy.Decode(nil /* dst */, serializedBytes)
		require.NoError(t, err)
		pb := &ethpb.BeaconStateElectra{}
		require.NoError(t, pb.UnmarshalSSZ(serializedSSZ))
		s, err := state_native.InitializeFromProtoElectra(pb)
		require.NoError(t, err)
		p, err := s.PendingPartialWithdrawals()
		require.NoError(t, err)
		require.NoError(t, s.UpdateBalancesAtIndex(p[0].Index, 0)) // This should still count as partial withdrawal.
		_, partialWithdrawalsCount, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, uint64(10), partialWithdrawalsCount)
	})
	t.Run("electra same validator has one partially and one fully withdrawable", func(t *testing.T) {
		s, _ := util.DeterministicGenesisStateElectra(t, 1)
		vals := make([]*ethpb.Validator, 100)
		balances := make([]uint64, 100)
		for i := range vals {
			balances[i] = params.BeaconConfig().MaxEffectiveBalance
			val := &ethpb.Validator{
				WithdrawalCredentials: make([]byte, 32),
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				WithdrawableEpoch:     primitives.Epoch(1),
				ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			}
			val.WithdrawalCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
			val.WithdrawalCredentials[31] = byte(i)
			vals[i] = val
		}
		balances[1] += params.BeaconConfig().MinDepositAmount
		vals[1].WithdrawableEpoch = primitives.Epoch(0)
		require.NoError(t, s.SetValidators(vals))
		require.NoError(t, s.SetBalances(balances))
		// Give validator a pending balance to withdraw.
		require.NoError(t, s.AppendPendingPartialWithdrawal(&ethpb.PendingPartialWithdrawal{
			Index:             1,
			Amount:            balances[1], // will only deduct excess even though balance is more that minimum activation
			WithdrawableEpoch: primitives.Epoch(0),
		}))
		p, err := s.PendingPartialWithdrawals()
		require.NoError(t, err)
		require.Equal(t, 1, len(p))
		expected, _, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, 2, len(expected))

		withdrawalFull := &enginev1.Withdrawal{
			Index:          1,
			ValidatorIndex: 1,
			Address:        vals[1].WithdrawalCredentials[12:],
			Amount:         balances[1] - params.BeaconConfig().MinDepositAmount, // subtract the partial from this
		}
		withdrawalPartial := &enginev1.Withdrawal{
			Index:          0,
			ValidatorIndex: 1,
			Address:        vals[1].WithdrawalCredentials[12:],
			Amount:         params.BeaconConfig().MinDepositAmount,
		}
		require.DeepEqual(t, withdrawalPartial, expected[0])
		require.DeepEqual(t, withdrawalFull, expected[1])
	})
}
