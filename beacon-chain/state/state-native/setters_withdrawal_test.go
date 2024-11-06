package state_native

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
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
	require.NoError(t, s.DequeuePendingPartialWithdrawals(2))
	num, err = s.NumPendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, uint64(1), num)

	// 2 of 1 exceeds the limit and an error should be returned
	num, err = s.NumPendingPartialWithdrawals()
	require.NoError(t, err)
	require.Equal(t, uint64(1), num)
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

	require.ErrorContains(t, "cannot append nil pending partial withdrawal", s.AppendPendingPartialWithdrawal(nil))

	s, err = InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)

	require.ErrorContains(t, "is not supported", s.AppendPendingPartialWithdrawal(nil))
}
