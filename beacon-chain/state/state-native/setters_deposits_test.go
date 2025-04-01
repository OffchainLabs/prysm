package state_native_test

import (
	"testing"

	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestAppendPendingDeposit(t *testing.T) {
	s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{})
	require.NoError(t, err)
	pbd, err := s.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 0, len(pbd))
	creds := []byte{0xFA, 0xCC}
	pubkey := []byte{0xAA, 0xBB}
	sig := []byte{0xCC, 0xDD}
	require.NoError(t, s.AppendPendingDeposit(&eth.PendingDeposit{
		PublicKey:             pubkey,
		WithdrawalCredentials: creds,
		Amount:                10,
		Signature:             sig,
		Slot:                  1,
	}))
	pbd, err = s.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 1, len(pbd))
	require.DeepEqual(t, pubkey, pbd[0].PublicKey)
	require.Equal(t, uint64(10), pbd[0].Amount)
	require.DeepEqual(t, creds, pbd[0].WithdrawalCredentials)
	require.Equal(t, primitives.Slot(1), pbd[0].Slot)
	require.DeepEqual(t, sig, pbd[0].Signature)

	ds := make([]*eth.PendingDeposit, 0, 4)
	require.NoError(t, s.SetPendingDeposits(ds))
	require.NoError(t, s.AppendPendingDeposit(&eth.PendingDeposit{Amount: 1}))
	s2 := s.Copy()
	require.NoError(t, s2.AppendPendingDeposit(&eth.PendingDeposit{Amount: 3}))
	require.NoError(t, s.AppendPendingDeposit(&eth.PendingDeposit{Amount: 2}))
	d, err := s.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, uint64(1), d[0].Amount)
	require.Equal(t, uint64(2), d[1].Amount)
	d, err = s2.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, uint64(1), d[0].Amount)
	require.Equal(t, uint64(3), d[1].Amount)

	// Fails for versions older than electra
	s, err = state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)
	require.ErrorContains(t, "not supported", s.AppendPendingDeposit(&eth.PendingDeposit{}))
}

func TestSetPendingDeposits(t *testing.T) {
	s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{})
	require.NoError(t, err)
	pbd, err := s.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 0, len(pbd))
	require.NoError(t, s.SetPendingDeposits([]*eth.PendingDeposit{{}, {}, {}}))
	pbd, err = s.PendingDeposits()
	require.NoError(t, err)
	require.Equal(t, 3, len(pbd))

	// Fails for versions older than electra
	s, err = state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)
	require.ErrorContains(t, "not supported", s.SetPendingDeposits([]*eth.PendingDeposit{{}, {}, {}}))
}

func TestSetDepositBalanceToConsume(t *testing.T) {
	s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{})
	require.NoError(t, err)
	require.NoError(t, s.SetDepositBalanceToConsume(10))
	dbtc, err := s.DepositBalanceToConsume()
	require.NoError(t, err)
	require.Equal(t, primitives.Gwei(10), dbtc)

	// Fails for versions older than electra
	s, err = state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)
	require.ErrorContains(t, "not supported", s.SetDepositBalanceToConsume(10))
}
