package state_native

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestSetNextWithdrawalIndex(t *testing.T) {
	s := BeaconState{
		version:             version.Capella,
		nextWithdrawalIndex: 3,
		dirtyFields:         make(map[types.FieldIndex]bool),
	}
	require.NoError(t, s.SetNextWithdrawalIndex(5))
	require.Equal(t, uint64(5), s.nextWithdrawalIndex)
	require.Equal(t, true, s.dirtyFields[types.NextWithdrawalIndex])
}

func TestSetNextWithdrawalValidatorIndex(t *testing.T) {
	s := BeaconState{
		version:                      version.Capella,
		nextWithdrawalValidatorIndex: 3,
		dirtyFields:                  make(map[types.FieldIndex]bool),
	}
	require.NoError(t, s.SetNextWithdrawalValidatorIndex(5))
	require.Equal(t, primitives.ValidatorIndex(5), s.nextWithdrawalValidatorIndex)
	require.Equal(t, true, s.dirtyFields[types.NextWithdrawalValidatorIndex])
}

func TestSetNextWithdrawalIndex_Deneb(t *testing.T) {
	s := BeaconState{
		version:             version.Deneb,
		nextWithdrawalIndex: 3,
		dirtyFields:         make(map[types.FieldIndex]bool),
	}
	require.NoError(t, s.SetNextWithdrawalIndex(5))
	require.Equal(t, uint64(5), s.nextWithdrawalIndex)
	require.Equal(t, true, s.dirtyFields[types.NextWithdrawalIndex])
}

func TestSetNextWithdrawalValidatorIndex_Deneb(t *testing.T) {
	s := BeaconState{
		version:                      version.Deneb,
		nextWithdrawalValidatorIndex: 3,
		dirtyFields:                  make(map[types.FieldIndex]bool),
	}
	require.NoError(t, s.SetNextWithdrawalValidatorIndex(5))
	require.Equal(t, primitives.ValidatorIndex(5), s.nextWithdrawalValidatorIndex)
	require.Equal(t, true, s.dirtyFields[types.NextWithdrawalValidatorIndex])
}

func TestDequeuePendingWithdrawals(t *testing.T) {
	s, err := InitializeFromProtoElectra(&eth.BeaconStateElectra{
		PendingPartialWithdrawals: []*eth.PendingPartialWithdrawal{
			{},
			{},
			{},
		},
	})
	require.NoError(t, err)

	// 2 of 3 should be OK
	num, err := s.NumPendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, uint64(3), num)
	s2 := s.Copy()
	require.NoError(t, s.DequeuePendingPartialWithdrawals(2))
	num, err = s.NumPendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, uint64(1), num)
	num, err = s2.NumPendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, uint64(3), num)

	// 2 of 1 exceeds the limit and an error should be returned
	require.ErrorContains(t, "cannot dequeue more withdrawals than are in the queue", s.DequeuePendingPartialWithdrawals(2))

	// Removing all pending partial withdrawals should be OK.
	num, err = s.NumPendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, uint64(1), num)
	require.NoError(t, s.DequeuePendingPartialWithdrawals(1))
	num, err = s.Copy().NumPendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, uint64(0), num)

	s, err = InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)

	require.ErrorContains(t, "is not supported", s.DequeuePendingPartialWithdrawals(0))
}

func TestAppendPendingWithdrawals(t *testing.T) {
	s, err := InitializeFromProtoElectra(&eth.BeaconStateElectra{
		PendingPartialWithdrawals: []*eth.PendingPartialWithdrawal{
			{},
			{},
			{},
		},
	})
	require.NoError(t, err)
	num, err := s.NumPendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, uint64(3), num)
	require.NoError(t, s.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{}))
	num, err = s.NumPendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, uint64(4), num)

	require.NoError(t, s.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{Index: 1}))
	s2 := s.Copy()
	require.NoError(t, s2.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{Index: 3}))
	require.NoError(t, s.AppendPendingPartialWithdrawal(&eth.PendingPartialWithdrawal{Index: 2}))
	w, err := s.PendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, primitives.ValidatorIndex(1), w[4].Index)
	require.Equal(t, primitives.ValidatorIndex(2), w[5].Index)
	w, err = s2.PendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, primitives.ValidatorIndex(1), w[4].Index)
	require.Equal(t, primitives.ValidatorIndex(3), w[5].Index)

	require.ErrorContains(t, "cannot append nil pending partial withdrawal", s.AppendPendingPartialWithdrawal(nil))

	s, err = InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)

	require.ErrorContains(t, "is not supported", s.AppendPendingPartialWithdrawal(nil))
}
