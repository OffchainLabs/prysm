package state_native_test

import (
	"testing"

	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestDepositBalanceToConsume(t *testing.T) {
	s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{
		DepositBalanceToConsume: 44,
	})
	require.NoError(t, err)
	dbtc, err := s.DepositBalanceToConsume()
	require.NoError(t, err)
	require.Equal(t, primitives.Gwei(44), dbtc)

	// Fails for older than electra state
	s, err = state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)
	_, err = s.DepositBalanceToConsume()
	require.ErrorContains(t, "not supported", err)
}

func TestPendingDeposits(t *testing.T) {
	s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{
		PendingDeposits: []*eth.PendingDeposit{
			{
				PublicKey:             []byte{1, 2, 3},
				WithdrawalCredentials: []byte{4, 5, 6},
				Amount:                2,
				Signature:             []byte{7, 8, 9},
				Slot:                  1,
			},
			{
				PublicKey:             []byte{11, 22, 33},
				WithdrawalCredentials: []byte{44, 55, 66},
				Amount:                4,
				Signature:             []byte{77, 88, 99},
				Slot:                  2,
			},
		},
	})
	require.NoError(t, err)
	pbd, err := s.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 2, len(pbd))
	require.DeepEqual(t, []byte{1, 2, 3}, pbd[0].PublicKey)
	require.DeepEqual(t, []byte{4, 5, 6}, pbd[0].WithdrawalCredentials)
	require.Equal(t, uint64(2), pbd[0].Amount)
	require.DeepEqual(t, []byte{7, 8, 9}, pbd[0].Signature)
	require.Equal(t, primitives.Slot(1), pbd[0].Slot)

	require.DeepEqual(t, []byte{11, 22, 33}, pbd[1].PublicKey)
	require.DeepEqual(t, []byte{44, 55, 66}, pbd[1].WithdrawalCredentials)
	require.Equal(t, uint64(4), pbd[1].Amount)
	require.DeepEqual(t, []byte{77, 88, 99}, pbd[1].Signature)
	require.Equal(t, primitives.Slot(2), pbd[1].Slot)

	// Fails for older than electra state
	s, err = state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)
	_, err = s.DepositBalanceToConsume()
	require.ErrorContains(t, "not supported", err)
}
