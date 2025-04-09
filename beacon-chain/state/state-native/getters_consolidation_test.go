package state_native_test

import (
	"testing"

	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestEarliestConsolidationEpoch(t *testing.T) {
	t.Run("electra returns expected value", func(t *testing.T) {
		want := primitives.Epoch(10)
		st, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{
			EarliestConsolidationEpoch: want,
		})
		require.NoError(t, err)
		got, err := st.EarliestConsolidationEpoch()
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("earlier than electra returns error", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoDeneb(&ethpb.BeaconStateDeneb{})
		require.NoError(t, err)
		_, err = st.EarliestConsolidationEpoch()
		require.ErrorContains(t, "is not supported", err)
	})
}

func TestConsolidationBalanceToConsume(t *testing.T) {
	t.Run("electra returns expected value", func(t *testing.T) {
		want := primitives.Gwei(10)
		st, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{
			ConsolidationBalanceToConsume: want,
		})
		require.NoError(t, err)
		got, err := st.ConsolidationBalanceToConsume()
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("earlier than electra returns error", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoDeneb(&ethpb.BeaconStateDeneb{})
		require.NoError(t, err)
		_, err = st.ConsolidationBalanceToConsume()
		require.ErrorContains(t, "is not supported", err)
	})
}

func TestPendingConsolidations(t *testing.T) {
	t.Run("electra returns expected value", func(t *testing.T) {
		want := []*ethpb.PendingConsolidation{
			{
				SourceIndex: 1,
				TargetIndex: 2,
			},
			{
				SourceIndex: 3,
				TargetIndex: 4,
			},
			{
				SourceIndex: 5,
				TargetIndex: 6,
			},
			{
				SourceIndex: 7,
				TargetIndex: 8,
			},
		}
		st, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{
			PendingConsolidations: want,
		})
		require.NoError(t, err)
		got, err := st.PendingConsolidations()
		require.NoError(t, err)
		require.DeepEqual(t, want, got)
	})

	t.Run("earlier than electra returns error", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoDeneb(&ethpb.BeaconStateDeneb{})
		require.NoError(t, err)
		_, err = st.PendingConsolidations()
		require.ErrorContains(t, "is not supported", err)
	})
}

func TestNumPendingConsolidations(t *testing.T) {
	t.Run("electra returns expected value", func(t *testing.T) {
		want := uint64(4)
		st, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{
			PendingConsolidations: []*ethpb.PendingConsolidation{
				{
					SourceIndex: 1,
					TargetIndex: 2,
				},
				{
					SourceIndex: 3,
					TargetIndex: 4,
				},
				{
					SourceIndex: 5,
					TargetIndex: 6,
				},
				{
					SourceIndex: 7,
					TargetIndex: 8,
				},
			},
		})
		require.NoError(t, err)
		got, err := st.NumPendingConsolidations()
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}
