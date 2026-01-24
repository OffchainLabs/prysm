package state_native_test

import (
	"bytes"
	"testing"

	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
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
	t.Run("returns error before gloas", func(t *testing.T) {
		stIface, _ := util.DeterministicGenesisState(t, 1)
		native, ok := stIface.(*state_native.BeaconState)
		require.Equal(t, true, ok)

		_, err := native.BuilderPubkey(0)
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("returns pubkey copy", func(t *testing.T) {
		pubkey := bytes.Repeat([]byte{0xAA}, 48)
		stIface, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				{
					Pubkey:            pubkey,
					Balance:           42,
					DepositEpoch:      3,
					WithdrawableEpoch: 4,
				},
			},
		})
		require.NoError(t, err)

		gotPk, err := stIface.BuilderPubkey(0)
		require.NoError(t, err)
		var wantPk [48]byte
		copy(wantPk[:], pubkey)
		require.Equal(t, wantPk, gotPk)

		// Mutate original to ensure copy.
		pubkey[0] = 0
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

func TestBuilderPendingPayments_UnsupportedVersion(t *testing.T) {
	stIface, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
	require.NoError(t, err)
	st := stIface.(*state_native.BeaconState)

	_, err = st.BuilderPendingPayments()
	require.ErrorContains(t, "BuilderPendingPayments", err)
}

func TestBuilderPendingPayment_ReturnsCopy(t *testing.T) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	payments := make([]*ethpb.BuilderPendingPayment, 2*slotsPerEpoch)
	target := uint64(slotsPerEpoch + 1)
	payments[target] = &ethpb.BuilderPendingPayment{Weight: 10}

	st, err := state_native.InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		BuilderPendingPayments: payments,
	})
	require.NoError(t, err)

	payment, err := st.BuilderPendingPayment(target)
	require.NoError(t, err)

	// mutate returned copy
	payment.Weight = 99

	original, err := st.BuilderPendingPayment(target)
	require.NoError(t, err)
	require.Equal(t, uint64(10), uint64(original.Weight))
}

func TestBuilderPendingPayment_UnsupportedVersion(t *testing.T) {
	stIface, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
	require.NoError(t, err)
	st := stIface.(*state_native.BeaconState)

	_, err = st.BuilderPendingPayment(0)
	require.ErrorContains(t, "BuilderPendingPayment", err)
}

func TestBuilderPendingPayment_OutOfRange(t *testing.T) {
	stIface, err := state_native.InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		BuilderPendingPayments: []*ethpb.BuilderPendingPayment{},
	})
	require.NoError(t, err)

	_, err = stIface.BuilderPendingPayment(0)
	require.ErrorContains(t, "out of range", err)
}

func TestExecutionPayloadAvailability(t *testing.T) {
	t.Run("unsupported version", func(t *testing.T) {
		stIface, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)
		st := stIface.(*state_native.BeaconState)

		_, err = st.ExecutionPayloadAvailability(0)
		require.ErrorContains(t, "ExecutionPayloadAvailability", err)
	})

	t.Run("reads expected bit", func(t *testing.T) {
		// Ensure the backing slice is large enough.
		availability := make([]byte, params.BeaconConfig().SlotsPerHistoricalRoot/8)

		// Pick a slot and set its corresponding bit.
		slot := primitives.Slot(9) // byteIndex=1, bitIndex=1
		availability[1] = 0b00000010

		stIface, err := state_native.InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
			ExecutionPayloadAvailability: availability,
		})
		require.NoError(t, err)

		bit, err := stIface.ExecutionPayloadAvailability(slot)
		require.NoError(t, err)
		require.Equal(t, uint64(1), bit)

		otherBit, err := stIface.ExecutionPayloadAvailability(8)
		require.NoError(t, err)
		require.Equal(t, uint64(0), otherBit)
	})
}
