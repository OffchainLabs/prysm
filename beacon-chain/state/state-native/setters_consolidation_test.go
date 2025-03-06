package state_native_test

import (
	"testing"

	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestAppendPendingConsolidation(t *testing.T) {
	s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{})
	require.NoError(t, err)
	num, err := s.NumPendingConsolidations()
	require.NoError(t, err)
	require.Equal(t, uint64(0), num)
	require.NoError(t, s.AppendPendingConsolidation(&eth.PendingConsolidation{}))
	num, err = s.NumPendingConsolidations()
	require.NoError(t, err)
	require.Equal(t, uint64(1), num)

	pc := make([]*eth.PendingConsolidation, 0, 4)
	require.NoError(t, s.SetPendingConsolidations(pc))
	require.NoError(t, s.AppendPendingConsolidation(&eth.PendingConsolidation{SourceIndex: 1}))
	s2 := s.Copy()
	require.NoError(t, s2.AppendPendingConsolidation(&eth.PendingConsolidation{SourceIndex: 3}))
	require.NoError(t, s.AppendPendingConsolidation(&eth.PendingConsolidation{SourceIndex: 2}))
	pc, err = s.PendingConsolidations()
	require.NoError(t, err)
	require.Equal(t, primitives.ValidatorIndex(1), pc[0].SourceIndex)
	require.Equal(t, primitives.ValidatorIndex(2), pc[1].SourceIndex)
	pc, err = s2.PendingConsolidations()
	require.NoError(t, err)
	require.Equal(t, primitives.ValidatorIndex(1), pc[0].SourceIndex)
	require.Equal(t, primitives.ValidatorIndex(3), pc[1].SourceIndex)

	// Fails for versions older than electra
	s, err = state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)
	require.ErrorContains(t, "not supported", s.AppendPendingConsolidation(&eth.PendingConsolidation{}))
}

func TestSetPendingConsolidations(t *testing.T) {
	s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{})
	require.NoError(t, err)
	num, err := s.NumPendingConsolidations()
	require.NoError(t, err)
	require.Equal(t, uint64(0), num)
	require.NoError(t, s.SetPendingConsolidations([]*eth.PendingConsolidation{{}, {}, {}}))
	num, err = s.NumPendingConsolidations()
	require.NoError(t, err)
	require.Equal(t, uint64(3), num)

	// Fails for versions older than electra
	s, err = state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)
	require.ErrorContains(t, "not supported", s.SetPendingConsolidations([]*eth.PendingConsolidation{{}, {}, {}}))
}

func TestSetEarliestConsolidationEpoch(t *testing.T) {
	s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{})
	require.NoError(t, err)
	ece, err := s.EarliestConsolidationEpoch()
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(0), ece)
	require.NoError(t, s.SetEarliestConsolidationEpoch(10))
	ece, err = s.EarliestConsolidationEpoch()
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(10), ece)

	// Fails for versions older than electra
	s, err = state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)
	require.ErrorContains(t, "not supported", s.SetEarliestConsolidationEpoch(10))
}

func TestSetConsolidationBalanceToConsume(t *testing.T) {
	s, err := state_native.InitializeFromProtoElectra(&eth.BeaconStateElectra{})
	require.NoError(t, err)
	require.NoError(t, s.SetConsolidationBalanceToConsume(10))
	cbtc, err := s.ConsolidationBalanceToConsume()
	require.NoError(t, err)
	require.Equal(t, primitives.Gwei(10), cbtc)

	// Fails for versions older than electra
	s, err = state_native.InitializeFromProtoDeneb(&eth.BeaconStateDeneb{})
	require.NoError(t, err)
	require.ErrorContains(t, "not supported", s.SetConsolidationBalanceToConsume(10))
}
