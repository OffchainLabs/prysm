package state_native_test

import (
	"testing"

	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	stateTesting "github.com/OffchainLabs/prysm/v7/beacon-chain/state/testing"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
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

func TestIsPendingValidator(t *testing.T) {
	sk, err := bls.RandKey()
	require.NoError(t, err)
	validDeposit := stateTesting.GeneratePendingDeposit(t, sk, 1000, [32]byte{0x01}, 0)

	t.Run("valid signature returns true", func(t *testing.T) {
		s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{
			PendingDeposits: []*eth.PendingDeposit{validDeposit},
		})
		require.NoError(t, err)

		ok, err := s.IsPendingValidator(validDeposit.PublicKey)
		require.NoError(t, err)
		require.Equal(t, true, ok)
	})

	t.Run("invalid signature returns false", func(t *testing.T) {
		invalidDeposit := &eth.PendingDeposit{
			PublicKey:             validDeposit.PublicKey,
			WithdrawalCredentials: validDeposit.WithdrawalCredentials,
			Amount:                validDeposit.Amount,
			Signature:             make([]byte, 96), // invalid empty signature
		}
		s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{
			PendingDeposits: []*eth.PendingDeposit{invalidDeposit},
		})
		require.NoError(t, err)

		ok, err := s.IsPendingValidator(validDeposit.PublicKey)
		require.NoError(t, err)
		require.Equal(t, false, ok)
	})

	t.Run("unknown pubkey returns false", func(t *testing.T) {
		s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{
			PendingDeposits: []*eth.PendingDeposit{validDeposit},
		})
		require.NoError(t, err)

		ok, err := s.IsPendingValidator([]byte{9, 9, 9})
		require.NoError(t, err)
		require.Equal(t, false, ok)
	})

	t.Run("nil deposit skipped", func(t *testing.T) {
		s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{
			PendingDeposits: []*eth.PendingDeposit{nil, validDeposit},
		})
		require.NoError(t, err)

		ok, err := s.IsPendingValidator(validDeposit.PublicKey)
		require.NoError(t, err)
		require.Equal(t, true, ok)
	})

	t.Run("pre-electra not supported", func(t *testing.T) {
		s, err := state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
		require.NoError(t, err)
		_, err = s.IsPendingValidator([]byte{1, 2, 3})
		require.ErrorContains(t, "not supported", err)
	})
}
