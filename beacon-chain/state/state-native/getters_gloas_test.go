package state_native_test

import (
	"bytes"
	"testing"

	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
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

func TestLatestExecutionPayloadBid(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		stIface, _ := util.DeterministicGenesisState(t, 1)
		native, ok := stIface.(*state_native.BeaconState)
		require.Equal(t, true, ok)

		_, err := native.LatestExecutionPayloadBid()
		require.ErrorContains(t, "is not supported", err)
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

func TestWithdrawalsMatchPayloadExpected(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		stIface, _ := util.DeterministicGenesisState(t, 1)
		native, ok := stIface.(*state_native.BeaconState)
		require.Equal(t, true, ok)

		_, err := native.WithdrawalsMatchPayloadExpected(nil)
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("returns true when roots match", func(t *testing.T) {
		withdrawals := []*enginev1.Withdrawal{
			{Index: 0, ValidatorIndex: 1, Address: bytes.Repeat([]byte{0x01}, 20), Amount: 10},
		}
		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			PayloadExpectedWithdrawals: withdrawals,
		})
		require.NoError(t, err)

		ok, err := st.WithdrawalsMatchPayloadExpected(withdrawals)
		require.NoError(t, err)
		require.Equal(t, true, ok)
	})

	t.Run("returns false when roots do not match", func(t *testing.T) {
		expected := []*enginev1.Withdrawal{
			{Index: 0, ValidatorIndex: 1, Address: bytes.Repeat([]byte{0x01}, 20), Amount: 10},
		}
		actual := []*enginev1.Withdrawal{
			{Index: 0, ValidatorIndex: 1, Address: bytes.Repeat([]byte{0x01}, 20), Amount: 11},
		}

		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			PayloadExpectedWithdrawals: expected,
		})
		require.NoError(t, err)

		ok, err := st.WithdrawalsMatchPayloadExpected(actual)
		require.NoError(t, err)
		require.Equal(t, false, ok)
	})
}

func TestBuilder(t *testing.T) {
	t.Run("nil builders returns nil", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: nil,
		})
		require.NoError(t, err)

		got, err := st.Builder(0)
		require.NoError(t, err)
		require.Equal(t, (*ethpb.Builder)(nil), got)
	})

	t.Run("out of bounds returns error", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{{}},
		})
		require.NoError(t, err)

		_, err = st.Builder(1)
		require.ErrorContains(t, "out of bounds", err)
	})

	t.Run("returns copy", func(t *testing.T) {
		pubkey := bytes.Repeat([]byte{0xAA}, fieldparams.BLSPubkeyLength)
		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
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

		got1, err := st.Builder(0)
		require.NoError(t, err)
		require.NotEqual(t, (*ethpb.Builder)(nil), got1)
		require.Equal(t, primitives.Gwei(42), got1.Balance)
		require.DeepEqual(t, pubkey, got1.Pubkey)

		// Mutate returned builder; state should be unchanged.
		got1.Pubkey[0] = 0xFF
		got2, err := st.Builder(0)
		require.NoError(t, err)
		require.Equal(t, byte(0xAA), got2.Pubkey[0])
	})
}

func TestBuilderIndexByPubkey(t *testing.T) {
	t.Run("not found returns false", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				{Pubkey: bytes.Repeat([]byte{0x11}, fieldparams.BLSPubkeyLength)},
			},
		})
		require.NoError(t, err)

		var pk [fieldparams.BLSPubkeyLength]byte
		copy(pk[:], bytes.Repeat([]byte{0x22}, fieldparams.BLSPubkeyLength))
		idx, ok := st.BuilderIndexByPubkey(pk)
		require.Equal(t, false, ok)
		require.Equal(t, primitives.BuilderIndex(0), idx)
	})

	t.Run("skips nil entries and finds match", func(t *testing.T) {
		wantIdx := primitives.BuilderIndex(1)
		wantPkBytes := bytes.Repeat([]byte{0xAB}, fieldparams.BLSPubkeyLength)

		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				nil,
				{Pubkey: wantPkBytes},
			},
		})
		require.NoError(t, err)

		var pk [fieldparams.BLSPubkeyLength]byte
		copy(pk[:], wantPkBytes)
		idx, ok := st.BuilderIndexByPubkey(pk)
		require.Equal(t, true, ok)
		require.Equal(t, wantIdx, idx)
	})
}
