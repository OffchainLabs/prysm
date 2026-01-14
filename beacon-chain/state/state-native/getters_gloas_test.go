package state_native_test

import (
	"bytes"
	"testing"

	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestLatestBlockHash(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		st, _ := util.DeterministicGenesisState(t, 1)
		_, err := st.LatestBlockHash()
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("returns zero hash when unset", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{})
		require.NoError(t, err)

		got, err := st.LatestBlockHash()
		require.NoError(t, err)
		require.Equal(t, [32]byte{}, got)
	})

	t.Run("returns configured hash", func(t *testing.T) {
		hashBytes := bytes.Repeat([]byte{0xAB}, 32)
		var want [32]byte
		copy(want[:], hashBytes)

		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			LatestBlockHash: hashBytes,
		})
		require.NoError(t, err)

		got, err := st.LatestBlockHash()
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}

func TestBuilderPubkey(t *testing.T) {
	t.Run("returns pubkey", func(t *testing.T) {
		stIface, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				{Pubkey: bytes.Repeat([]byte{0xAA}, 48)},
			},
		})
		require.NoError(t, err)

		st := stIface.(*state_native.BeaconState)
		gotPk, err := st.BuilderPubkey(0)
		require.NoError(t, err)
		require.Equal(t, byte(0xAA), gotPk[0])
	})

	t.Run("out of range returns error", func(t *testing.T) {
		stIface, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{},
		})
		require.NoError(t, err)

		st := stIface.(*state_native.BeaconState)
		_, err = st.BuilderPubkey(1)
		require.ErrorContains(t, "out of range", err)
	})
}

func TestBuilderHelpers(t *testing.T) {
	t.Run("is active builder", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				{
					Balance:           10,
					DepositEpoch:      0,
					WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
				},
			},
			FinalizedCheckpoint: &ethpb.Checkpoint{Epoch: 1},
		})
		require.NoError(t, err)

		active, err := st.IsActiveBuilder(0)
		require.NoError(t, err)
		require.Equal(t, true, active)

		// Not active when withdrawable epoch is set.
		stProto := &ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				{
					Balance:           10,
					DepositEpoch:      0,
					WithdrawableEpoch: 1,
				},
			},
			FinalizedCheckpoint: &ethpb.Checkpoint{Epoch: 2},
		}
		stInactive, err := state_native.InitializeFromProtoGloas(stProto)
		require.NoError(t, err)

		active, err = stInactive.IsActiveBuilder(0)
		require.NoError(t, err)
		require.Equal(t, false, active)
	})

	t.Run("can builder cover bid", func(t *testing.T) {
		stIface, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				{
					Balance:           primitives.Gwei(params.BeaconConfig().MinDepositAmount + 50),
					DepositEpoch:      0,
					WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
				},
			},
			BuilderPendingWithdrawals: []*ethpb.BuilderPendingWithdrawal{
				{Amount: 10, BuilderIndex: 0},
			},
			BuilderPendingPayments: []*ethpb.BuilderPendingPayment{
				{Withdrawal: &ethpb.BuilderPendingWithdrawal{Amount: 15, BuilderIndex: 0}},
			},
			FinalizedCheckpoint: &ethpb.Checkpoint{Epoch: 1},
		})
		require.NoError(t, err)

		st := stIface.(*state_native.BeaconState)
		ok, err := st.CanBuilderCoverBid(0, 20)
		require.NoError(t, err)
		require.Equal(t, true, ok)

		ok, err = st.CanBuilderCoverBid(0, 30)
		require.NoError(t, err)
		require.Equal(t, false, ok)
	})
}

func TestBuilderPendingPaymentsNoCopy_UnsupportedVersion(t *testing.T) {
	stIface, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
	require.NoError(t, err)
	st := stIface.(*state_native.BeaconState)

	_, err = st.BuilderPendingPaymentsNoCopy()
	require.ErrorContains(t, "BuilderPendingPaymentsNoCopy", err)
}
