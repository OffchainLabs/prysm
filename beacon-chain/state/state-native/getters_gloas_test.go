package state_native

import (
	"bytes"
	"fmt"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func TestLatestBlockHash(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		_, err := st.LatestBlockHash()
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("returns zero hash when unset", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{})
		require.NoError(t, err)

		got, err := st.LatestBlockHash()
		require.NoError(t, err)
		require.Equal(t, [32]byte{}, got)
	})

	t.Run("returns configured hash", func(t *testing.T) {
		hashBytes := bytes.Repeat([]byte{0xAB}, 32)
		var want [32]byte
		copy(want[:], hashBytes)

		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
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
		st := &BeaconState{version: version.Fulu}
		_, err := st.LatestExecutionPayloadBid()
		require.ErrorContains(t, "is not supported", err)
	})
}

func TestIsAttestationSameSlot(t *testing.T) {
	buildStateWithBlockRoots := func(t *testing.T, stateSlot primitives.Slot, roots map[primitives.Slot][]byte) *BeaconState {
		t.Helper()

		cfg := params.BeaconConfig()
		blockRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
		for slot, root := range roots {
			blockRoots[slot%cfg.SlotsPerHistoricalRoot] = root
		}

		stIface, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Slot:       stateSlot,
			BlockRoots: blockRoots,
		})
		require.NoError(t, err)
		return stIface.(*BeaconState)
	}

	rootA := bytes.Repeat([]byte{0xAA}, 32)
	rootB := bytes.Repeat([]byte{0xBB}, 32)
	rootC := bytes.Repeat([]byte{0xCC}, 32)

	tests := []struct {
		name      string
		stateSlot primitives.Slot
		slot      primitives.Slot
		blockRoot []byte
		roots     map[primitives.Slot][]byte
		want      bool
	}{
		{
			name:      "slot zero always true",
			stateSlot: 1,
			slot:      0,
			blockRoot: rootA,
			roots:     map[primitives.Slot][]byte{},
			want:      true,
		},
		{
			name:      "matching current different previous",
			stateSlot: 6,
			slot:      4,
			blockRoot: rootA,
			roots: map[primitives.Slot][]byte{
				4: rootA,
				3: rootB,
			},
			want: true,
		},
		{
			name:      "matching current same previous",
			stateSlot: 6,
			slot:      4,
			blockRoot: rootA,
			roots: map[primitives.Slot][]byte{
				4: rootA,
				3: rootA,
			},
			want: false,
		},
		{
			name:      "non matching current",
			stateSlot: 6,
			slot:      4,
			blockRoot: rootC,
			roots: map[primitives.Slot][]byte{
				4: rootA,
				3: rootB,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := buildStateWithBlockRoots(t, tt.stateSlot, tt.roots)
			var rootArr [32]byte
			copy(rootArr[:], tt.blockRoot)

			got, err := st.IsAttestationSameSlot(rootArr, tt.slot)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBuilderPubkey(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		_, err := st.BuilderPubkey(0)
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("returns pubkey copy", func(t *testing.T) {
		pubkey := bytes.Repeat([]byte{0xAA}, 48)
		stIface, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
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
		stIface, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{},
		})
		require.NoError(t, err)

		st := stIface.(*BeaconState)
		_, err = st.BuilderPubkey(1)
		require.ErrorContains(t, "out of range", err)
	})
}

func TestBuilderHelpers(t *testing.T) {
	t.Run("is active builder", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
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
		stInactive, err := InitializeFromProtoGloas(stProto)
		require.NoError(t, err)

		active, err = stInactive.IsActiveBuilder(0)
		require.NoError(t, err)
		require.Equal(t, false, active)
	})

	t.Run("can builder cover bid", func(t *testing.T) {
		stIface, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
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

		st := stIface.(*BeaconState)
		ok, err := st.CanBuilderCoverBid(0, 20)
		require.NoError(t, err)
		require.Equal(t, true, ok)

		ok, err = st.CanBuilderCoverBid(0, 30)
		require.NoError(t, err)
		require.Equal(t, false, ok)
	})
}

func TestBuilderPendingPayments_UnsupportedVersion(t *testing.T) {
	stIface, err := InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
	require.NoError(t, err)
	st := stIface.(*BeaconState)

	_, err = st.BuilderPendingPayments()
	require.ErrorContains(t, "BuilderPendingPayments", err)
}

func TestWithdrawalsMatchPayloadExpected(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		_, err := st.WithdrawalsMatchPayloadExpected(nil)
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("returns true when roots match", func(t *testing.T) {
		withdrawals := []*enginev1.Withdrawal{
			{Index: 0, ValidatorIndex: 1, Address: bytes.Repeat([]byte{0x01}, 20), Amount: 10},
		}
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
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

		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			PayloadExpectedWithdrawals: expected,
		})
		require.NoError(t, err)

		ok, err := st.WithdrawalsMatchPayloadExpected(actual)
		require.NoError(t, err)
		require.Equal(t, false, ok)
	})
}

func TestBuilder(t *testing.T) {
	t.Run("nil builders returns error", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: nil,
		})
		require.NoError(t, err)

		_, err = st.Builder(0)
		require.ErrorContains(t, "out of bounds", err)
	})

	t.Run("out of bounds returns error", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{{}},
		})
		require.NoError(t, err)

		_, err = st.Builder(1)
		require.ErrorContains(t, "out of bounds", err)
	})

	t.Run("returns copy", func(t *testing.T) {
		pubkey := bytes.Repeat([]byte{0xAA}, fieldparams.BLSPubkeyLength)
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
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
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
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

		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
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

	t.Run("AddBuilderFromDeposit populates map", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{})
		require.NoError(t, err)

		var pk [fieldparams.BLSPubkeyLength]byte
		copy(pk[:], bytes.Repeat([]byte{0xCD}, fieldparams.BLSPubkeyLength))
		var wc [32]byte
		require.NoError(t, st.AddBuilderFromDeposit(pk, wc, 1))

		idx, ok := st.BuilderIndexByPubkey(pk)
		require.Equal(t, true, ok)
		require.Equal(t, primitives.BuilderIndex(0), idx)
	})

	t.Run("reused slot evicts old pubkey", func(t *testing.T) {
		oldPk := bytes.Repeat([]byte{0x11}, fieldparams.BLSPubkeyLength)
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				{
					Pubkey:            oldPk,
					WithdrawableEpoch: 0,
					Balance:           0,
				},
			},
		})
		require.NoError(t, err)

		var oldPkArr [fieldparams.BLSPubkeyLength]byte
		copy(oldPkArr[:], oldPk)
		_, ok := st.BuilderIndexByPubkey(oldPkArr)
		require.Equal(t, true, ok)

		var newPk [fieldparams.BLSPubkeyLength]byte
		copy(newPk[:], bytes.Repeat([]byte{0x22}, fieldparams.BLSPubkeyLength))
		var wc [32]byte
		require.NoError(t, st.AddBuilderFromDeposit(newPk, wc, 1))

		_, ok = st.BuilderIndexByPubkey(oldPkArr)
		require.Equal(t, false, ok)
		idx, ok := st.BuilderIndexByPubkey(newPk)
		require.Equal(t, true, ok)
		require.Equal(t, primitives.BuilderIndex(0), idx)
	})

	t.Run("SetBuilders rebuilds map", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				{Pubkey: bytes.Repeat([]byte{0x11}, fieldparams.BLSPubkeyLength)},
			},
		})
		require.NoError(t, err)

		var oldPk [fieldparams.BLSPubkeyLength]byte
		copy(oldPk[:], bytes.Repeat([]byte{0x11}, fieldparams.BLSPubkeyLength))
		newPkBytes := bytes.Repeat([]byte{0x33}, fieldparams.BLSPubkeyLength)
		require.NoError(t, st.SetBuilders([]*ethpb.Builder{{Pubkey: newPkBytes}}))

		_, ok := st.BuilderIndexByPubkey(oldPk)
		require.Equal(t, false, ok)
		var newPk [fieldparams.BLSPubkeyLength]byte
		copy(newPk[:], newPkBytes)
		idx, ok := st.BuilderIndexByPubkey(newPk)
		require.Equal(t, true, ok)
		require.Equal(t, primitives.BuilderIndex(0), idx)
	})

	t.Run("UpdateBuilderAtIndex swaps pubkey mapping", func(t *testing.T) {
		oldPk := bytes.Repeat([]byte{0x11}, fieldparams.BLSPubkeyLength)
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				{Pubkey: oldPk},
			},
		})
		require.NoError(t, err)

		newPkBytes := bytes.Repeat([]byte{0x44}, fieldparams.BLSPubkeyLength)
		require.NoError(t, st.UpdateBuilderAtIndex(0, &ethpb.Builder{Pubkey: newPkBytes}))

		var oldPkArr [fieldparams.BLSPubkeyLength]byte
		copy(oldPkArr[:], oldPk)
		_, ok := st.BuilderIndexByPubkey(oldPkArr)
		require.Equal(t, false, ok)
		var newPk [fieldparams.BLSPubkeyLength]byte
		copy(newPk[:], newPkBytes)
		idx, ok := st.BuilderIndexByPubkey(newPk)
		require.Equal(t, true, ok)
		require.Equal(t, primitives.BuilderIndex(0), idx)
	})

	t.Run("Copy yields independent maps", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{
				{Pubkey: bytes.Repeat([]byte{0x55}, fieldparams.BLSPubkeyLength)},
			},
		})
		require.NoError(t, err)

		copied := st.Copy()
		var newPk [fieldparams.BLSPubkeyLength]byte
		copy(newPk[:], bytes.Repeat([]byte{0x66}, fieldparams.BLSPubkeyLength))
		var wc [32]byte
		require.NoError(t, copied.AddBuilderFromDeposit(newPk, wc, 1))

		_, ok := st.BuilderIndexByPubkey(newPk)
		require.Equal(t, false, ok)
		_, ok = copied.BuilderIndexByPubkey(newPk)
		require.Equal(t, true, ok)
	})
}

func TestBuilderPendingPayment(t *testing.T) {
	t.Run("returns copy", func(t *testing.T) {
		slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
		payments := make([]*ethpb.BuilderPendingPayment, 2*slotsPerEpoch)
		target := uint64(slotsPerEpoch + 1)
		payments[target] = &ethpb.BuilderPendingPayment{Weight: 10}

		st, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
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
	})

	t.Run("unsupported version", func(t *testing.T) {
		stIface, err := InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)
		st := stIface.(*BeaconState)

		_, err = st.BuilderPendingPayment(0)
		require.ErrorContains(t, "BuilderPendingPayment", err)
	})

	t.Run("out of range", func(t *testing.T) {
		stIface, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
			BuilderPendingPayments: []*ethpb.BuilderPendingPayment{},
		})
		require.NoError(t, err)

		_, err = stIface.BuilderPendingPayment(0)
		require.ErrorContains(t, "out of range", err)
	})
}

func TestExecutionPayloadAvailability(t *testing.T) {
	t.Run("unsupported version", func(t *testing.T) {
		stIface, err := InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)
		st := stIface.(*BeaconState)

		_, err = st.ExecutionPayloadAvailability(0)
		require.ErrorContains(t, "ExecutionPayloadAvailability", err)
	})

	t.Run("reads expected bit", func(t *testing.T) {
		// Ensure the backing slice is large enough.
		availability := make([]byte, params.BeaconConfig().SlotsPerHistoricalRoot/8)

		// Pick a slot and set its corresponding bit.
		slot := primitives.Slot(9) // byteIndex=1, bitIndex=1
		availability[1] = 0b00000010

		stIface, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
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

func TestLatestBlockHashMatchesBidBlockHash(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		_, err := st.LatestBlockHashMatchesBidBlockHash()
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("returns false when bid is nil", func(t *testing.T) {
		st := &BeaconState{version: version.Gloas}
		got, err := st.LatestBlockHashMatchesBidBlockHash()
		require.NoError(t, err)
		require.Equal(t, false, got)
	})

	t.Run("returns true when hashes match", func(t *testing.T) {
		hash := bytes.Repeat([]byte{0xAB}, 32)
		st := &BeaconState{
			version: version.Gloas,
			latestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
				BlockHash: hash,
			},
			latestBlockHash: hash,
		}

		got, err := st.LatestBlockHashMatchesBidBlockHash()
		require.NoError(t, err)
		require.Equal(t, true, got)
	})

	t.Run("returns false when hashes differ", func(t *testing.T) {
		hash := bytes.Repeat([]byte{0xAB}, 32)
		other := bytes.Repeat([]byte{0xCD}, 32)
		st := &BeaconState{
			version: version.Gloas,
			latestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
				BlockHash: hash,
			},
			latestBlockHash: other,
		}

		got, err := st.LatestBlockHashMatchesBidBlockHash()
		require.NoError(t, err)
		require.Equal(t, false, got)
	})
}

func TestAppendBuilderWithdrawals(t *testing.T) {
	t.Run("errors when prior withdrawals exceed limit", func(t *testing.T) {
		st := &BeaconState{}
		limit := params.BeaconConfig().MaxWithdrawalsPerPayload - 1
		withdrawals := make([]*enginev1.Withdrawal, limit+1)

		nextIndex, processed, err := st.appendBuilderWithdrawals(5, &withdrawals)
		require.ErrorContains(t, "exceeds limit", err)
		require.Equal(t, uint64(5), nextIndex)
		require.Equal(t, uint64(0), processed)
		require.Equal(t, int(limit+1), len(withdrawals))
	})

	t.Run("appends builder withdrawals and increments index", func(t *testing.T) {
		st := &BeaconState{
			builderPendingWithdrawals: []*ethpb.BuilderPendingWithdrawal{
				{BuilderIndex: 1, FeeRecipient: []byte{0x01}, Amount: 11},
				{BuilderIndex: 2, FeeRecipient: []byte{0x02}, Amount: 22},
				{BuilderIndex: 3, FeeRecipient: []byte{0x03}, Amount: 33},
			},
		}
		withdrawals := []*enginev1.Withdrawal{
			{Index: 7, ValidatorIndex: 9, Address: []byte{0xAA}, Amount: 99},
		}

		nextIndex, processed, err := st.appendBuilderWithdrawals(10, &withdrawals)
		require.NoError(t, err)
		require.Equal(t, uint64(13), nextIndex)
		require.Equal(t, uint64(3), processed)
		require.Equal(t, 4, len(withdrawals))

		require.DeepEqual(t, &enginev1.Withdrawal{
			Index:          10,
			ValidatorIndex: primitives.BuilderIndex(1).ToValidatorIndex(),
			Address:        []byte{0x01},
			Amount:         11,
		}, withdrawals[1])
		require.DeepEqual(t, &enginev1.Withdrawal{
			Index:          11,
			ValidatorIndex: primitives.BuilderIndex(2).ToValidatorIndex(),
			Address:        []byte{0x02},
			Amount:         22,
		}, withdrawals[2])
		require.DeepEqual(t, &enginev1.Withdrawal{
			Index:          12,
			ValidatorIndex: primitives.BuilderIndex(3).ToValidatorIndex(),
			Address:        []byte{0x03},
			Amount:         33,
		}, withdrawals[3])
	})

	t.Run("respects per-payload limit", func(t *testing.T) {
		limit := params.BeaconConfig().MaxWithdrawalsPerPayload - 1
		st := &BeaconState{
			builderPendingWithdrawals: []*ethpb.BuilderPendingWithdrawal{
				{BuilderIndex: 4, FeeRecipient: []byte{0x04}, Amount: 44},
				{BuilderIndex: 5, FeeRecipient: []byte{0x05}, Amount: 55},
			},
		}
		withdrawals := make([]*enginev1.Withdrawal, limit-1)

		nextIndex, processed, err := st.appendBuilderWithdrawals(20, &withdrawals)
		require.NoError(t, err)
		require.Equal(t, uint64(21), nextIndex)
		require.Equal(t, uint64(1), processed)
		require.Equal(t, int(limit), len(withdrawals))
		require.DeepEqual(t, &enginev1.Withdrawal{
			Index:          20,
			ValidatorIndex: primitives.BuilderIndex(4).ToValidatorIndex(),
			Address:        []byte{0x04},
			Amount:         44,
		}, withdrawals[len(withdrawals)-1])
	})

	t.Run("does not append when already at limit", func(t *testing.T) {
		limit := params.BeaconConfig().MaxWithdrawalsPerPayload - 1
		if limit == 0 {
			t.Skip("withdrawals limit too small")
		}
		st := &BeaconState{
			builderPendingWithdrawals: []*ethpb.BuilderPendingWithdrawal{
				{BuilderIndex: 6, FeeRecipient: []byte{0x06}, Amount: 66},
			},
		}
		withdrawals := make([]*enginev1.Withdrawal, limit)

		nextIndex, processed, err := st.appendBuilderWithdrawals(30, &withdrawals)
		require.NoError(t, err)
		require.Equal(t, uint64(30), nextIndex)
		require.Equal(t, uint64(0), processed)
		require.Equal(t, int(limit), len(withdrawals))
	})
}

func TestAppendBuildersSweepWithdrawals(t *testing.T) {
	t.Run("errors when prior withdrawals exceed limit", func(t *testing.T) {
		st := &BeaconState{}
		limit := params.BeaconConfig().MaxWithdrawalsPerPayload - 1
		withdrawals := make([]*enginev1.Withdrawal, limit+1)

		nextIndex, nextBuilderIndex, err := st.appendBuildersSweepWithdrawals(5, &withdrawals)
		require.ErrorContains(t, "exceeds limit", err)
		require.Equal(t, uint64(5), nextIndex)
		require.Equal(t, primitives.BuilderIndex(0), nextBuilderIndex)
		require.Equal(t, int(limit+1), len(withdrawals))
	})

	t.Run("no builders returns without error", func(t *testing.T) {
		st := &BeaconState{
			nextWithdrawalBuilderIndex: 3,
			builders:                   nil,
		}
		withdrawals := []*enginev1.Withdrawal{}

		nextIndex, nextBuilderIndex, err := st.appendBuildersSweepWithdrawals(5, &withdrawals)
		require.NoError(t, err)
		require.Equal(t, uint64(5), nextIndex)
		require.Equal(t, primitives.BuilderIndex(3), nextBuilderIndex)
		require.Equal(t, 0, len(withdrawals))
	})

	t.Run("appends eligible builders, skips ineligible", func(t *testing.T) {
		epoch := primitives.Epoch(3)
		st := &BeaconState{
			slot:                       slots.UnsafeEpochStart(epoch),
			nextWithdrawalBuilderIndex: 2,
			builders: []*ethpb.Builder{
				{ExecutionAddress: []byte{0x01}, Balance: 0, WithdrawableEpoch: epoch},
				{ExecutionAddress: []byte{0x02}, Balance: 10, WithdrawableEpoch: epoch + 1},
				{ExecutionAddress: []byte{0x03}, Balance: 20, WithdrawableEpoch: epoch},
			},
		}
		withdrawals := []*enginev1.Withdrawal{}

		nextIndex, nextBuilderIndex, err := st.appendBuildersSweepWithdrawals(100, &withdrawals)
		require.NoError(t, err)
		require.Equal(t, uint64(101), nextIndex)
		require.Equal(t, primitives.BuilderIndex(2), nextBuilderIndex)
		require.Equal(t, 1, len(withdrawals))
		require.DeepEqual(t, &enginev1.Withdrawal{
			Index:          100,
			ValidatorIndex: primitives.BuilderIndex(2).ToValidatorIndex(),
			Address:        []byte{0x03},
			Amount:         20,
		}, withdrawals[0])
	})

	t.Run("respects max builders per sweep", func(t *testing.T) {
		cfg := params.BeaconConfig()
		max := int(cfg.MaxBuildersPerWithdrawalsSweep)
		epoch := primitives.Epoch(1)
		builders := make([]*ethpb.Builder, max+2)
		for i := range builders {
			builders[i] = &ethpb.Builder{
				ExecutionAddress:  []byte{byte(i + 1)},
				Balance:           1,
				WithdrawableEpoch: epoch,
			}
		}
		start := len(builders) - 1
		st := &BeaconState{
			slot:                       slots.UnsafeEpochStart(epoch),
			nextWithdrawalBuilderIndex: primitives.BuilderIndex(start),
			builders:                   builders,
		}
		withdrawals := []*enginev1.Withdrawal{}

		nextIndex, nextBuilderIndex, err := st.appendBuildersSweepWithdrawals(7, &withdrawals)
		require.NoError(t, err)
		limit := int(cfg.MaxWithdrawalsPerPayload - 1)
		expectedCount := min(max, limit)
		require.Equal(t, uint64(7)+uint64(expectedCount), nextIndex)
		require.Equal(t, expectedCount, len(withdrawals))
		expectedNext := primitives.BuilderIndex((uint64(start) + uint64(expectedCount)) % uint64(len(builders)))
		require.Equal(t, expectedNext, nextBuilderIndex)
	})

	t.Run("stops when payload limit reached", func(t *testing.T) {
		cfg := params.BeaconConfig()
		limit := cfg.MaxWithdrawalsPerPayload - 1
		if limit < 1 {
			t.Skip("withdrawals limit too small")
		}
		epoch := primitives.Epoch(2)
		builders := []*ethpb.Builder{
			{ExecutionAddress: []byte{0x0A}, Balance: 3, WithdrawableEpoch: epoch},
			{ExecutionAddress: []byte{0x0B}, Balance: 4, WithdrawableEpoch: epoch},
		}
		st := &BeaconState{
			slot:                       slots.UnsafeEpochStart(epoch),
			nextWithdrawalBuilderIndex: 0,
			builders:                   builders,
		}
		withdrawals := make([]*enginev1.Withdrawal, limit)

		nextIndex, nextBuilderIndex, err := st.appendBuildersSweepWithdrawals(20, &withdrawals)
		require.NoError(t, err)
		require.Equal(t, uint64(20), nextIndex)
		require.Equal(t, int(limit), len(withdrawals))
		require.Equal(t, primitives.BuilderIndex(0), nextBuilderIndex)
	})
}

func TestBuilderPendingWithdrawals(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		stIface, err := InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)
		st := stIface.(*BeaconState)

		_, err = st.BuilderPendingWithdrawals()
		require.ErrorContains(t, "BuilderPendingWithdrawals", err)
	})

	t.Run("returns copy", func(t *testing.T) {
		original := []*ethpb.BuilderPendingWithdrawal{
			{Amount: 10, BuilderIndex: 1},
		}
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			BuilderPendingWithdrawals: original,
		})
		require.NoError(t, err)

		got1, err := st.BuilderPendingWithdrawals()
		require.NoError(t, err)
		require.Equal(t, len(original), len(got1))
		require.Equal(t, original[0].Amount, got1[0].Amount)
		require.Equal(t, original[0].BuilderIndex, got1[0].BuilderIndex)

		got1[0].Amount = 99
		got2, err := st.BuilderPendingWithdrawals()
		require.NoError(t, err)
		require.Equal(t, len(original), len(got2))
		require.Equal(t, original[0].Amount, got2[0].Amount)
		require.Equal(t, original[0].BuilderIndex, got2[0].BuilderIndex)

	})
}

func TestBuildersGetter(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		stIface, err := InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)
		st := stIface.(*BeaconState)

		_, err = st.Builders()
		require.ErrorContains(t, "Builders", err)
	})

	t.Run("returns copy", func(t *testing.T) {
		pubkey := bytes.Repeat([]byte{0xAB}, fieldparams.BLSPubkeyLength)
		buildr := &ethpb.Builder{
			Pubkey:            pubkey,
			Balance:           42,
			DepositEpoch:      3,
			WithdrawableEpoch: 4,
		}
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Builders: []*ethpb.Builder{buildr},
		})
		require.NoError(t, err)

		got1, err := st.Builders()
		require.NoError(t, err)
		require.DeepEqual(t, buildr, got1[0])

		got1[0].Pubkey[0] = 0xFF
		got2, err := st.Builders()
		require.NoError(t, err)
		require.DeepEqual(t, buildr, got2[0])
	})
}

func TestNextWithdrawalBuilderIndex(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		stIface, err := InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)
		st := stIface.(*BeaconState)

		_, err = st.NextWithdrawalBuilderIndex()
		require.ErrorContains(t, "NextWithdrawalBuilderIndex", err)
	})

	t.Run("returns configured value", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			NextWithdrawalBuilderIndex: 2,
		})
		require.NoError(t, err)

		got, err := st.NextWithdrawalBuilderIndex()
		require.NoError(t, err)
		require.Equal(t, primitives.BuilderIndex(2), got)
	})
}

func TestPayloadExpectedWithdrawals(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		stIface, err := InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)
		st := stIface.(*BeaconState)

		_, err = st.PayloadExpectedWithdrawals()
		require.ErrorContains(t, "PayloadExpectedWithdrawals", err)
	})

	t.Run("returns copy", func(t *testing.T) {
		original := enginev1.Withdrawal{
			Index:          1,
			ValidatorIndex: 2,
			Address:        bytes.Repeat([]byte{0x01}, 20),
			Amount:         10,
		}
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			PayloadExpectedWithdrawals: []*enginev1.Withdrawal{&original},
		})
		require.NoError(t, err)

		got1, err := st.PayloadExpectedWithdrawals()
		require.NoError(t, err)
		require.DeepEqual(t, &original, got1[0])

		got1[0].Amount = 99
		got2, err := st.PayloadExpectedWithdrawals()
		require.NoError(t, err)
		require.DeepEqual(t, &original, got2[0])
	})
}

func TestWithdrawalsForPayload(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		_, err := st.WithdrawalsForPayload()
		require.ErrorContains(t, "WithdrawalsForPayload", err)
	})

	t.Run("returns existing withdrawals when parent empty", func(t *testing.T) {
		existing := []*enginev1.Withdrawal{
			{Index: 5, ValidatorIndex: 10, Address: bytes.Repeat([]byte{0x26}, 20), Amount: 100},
		}
		// Parent is empty: bid block hash differs from latest block hash.
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
				BlockHash: bytes.Repeat([]byte{0xAA}, 32),
			},
			LatestBlockHash:            bytes.Repeat([]byte{0xBB}, 32),
			PayloadExpectedWithdrawals: existing,
		})
		require.NoError(t, err)

		got, err := st.WithdrawalsForPayload()
		require.NoError(t, err)
		require.DeepEqual(t, existing, got)
	})

	t.Run("computes fresh withdrawals when parent full", func(t *testing.T) {
		hash := bytes.Repeat([]byte{0xAB}, 32)
		stale := []*enginev1.Withdrawal{
			{Index: 1, ValidatorIndex: 2, Address: bytes.Repeat([]byte{0x01}, 20), Amount: 999},
		}
		// Parent is full: bid block hash == latest block hash.
		// With no validators/pending withdrawals, fresh computation returns empty.
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
				BlockHash: hash,
			},
			LatestBlockHash:            hash,
			PayloadExpectedWithdrawals: stale,
		})
		require.NoError(t, err)

		got, err := st.WithdrawalsForPayload()
		require.NoError(t, err)
		// Fresh computation with no validators yields empty, not the stale value.
		require.Equal(t, 0, len(got))
	})
}

func TestExecutionPayloadAvailabilityVector(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		stIface, err := InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)
		st := stIface.(*BeaconState)

		_, err = st.ExecutionPayloadAvailabilityVector()
		require.ErrorContains(t, "ExecutionPayloadAvailabilityVector", err)
	})

	t.Run("returns copy", func(t *testing.T) {
		availability := []byte{0xAA, 0xBB, 0xCC}
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			ExecutionPayloadAvailability: availability,
		})
		require.NoError(t, err)

		got1, err := st.ExecutionPayloadAvailabilityVector()
		require.NoError(t, err)
		require.DeepEqual(t, availability, got1)

		got1[0] = 0xFF
		got2, err := st.ExecutionPayloadAvailabilityVector()
		require.NoError(t, err)
		require.DeepEqual(t, availability, got2)
	})
}

// testPTCWindow creates a PTC window of the expected size where each slot's
// first validator index encodes the slot offset for easy identification.
func testPTCWindow(t *testing.T) []*ethpb.PTCs {
	t.Helper()
	size := int(expectedPTCWindowSize())
	window := make([]*ethpb.PTCs, size)
	for i := range window {
		indices := make([]primitives.ValidatorIndex, fieldparams.PTCSize)
		indices[0] = primitives.ValidatorIndex(i) + 1 // non-zero marker
		window[i] = &ethpb.PTCs{ValidatorIndices: indices}
	}
	return window
}

func TestPtcWindowOffset(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	t.Run("current epoch slot", func(t *testing.T) {
		// State at epoch 2, query slot in epoch 2 → offset = (2-2+1)*32 + slot%32 = 32 + slot%32
		stateSlot := slotsPerEpoch * 2 // slot 64, epoch 2
		querySlot := stateSlot + 3     // slot 67, epoch 2
		offset, err := ptcWindowOffset(stateSlot, querySlot)
		require.NoError(t, err)
		expected := slotsPerEpoch + (querySlot % slotsPerEpoch) // offset in "current epoch" region
		require.Equal(t, expected, offset)
	})

	t.Run("previous epoch slot", func(t *testing.T) {
		stateSlot := slotsPerEpoch * 2 // epoch 2
		querySlot := slotsPerEpoch + 5 // slot 37, epoch 1 (previous)
		offset, err := ptcWindowOffset(stateSlot, querySlot)
		require.NoError(t, err)
		require.Equal(t, querySlot%slotsPerEpoch, offset) // previous epoch region
	})

	t.Run("lookahead epoch slot", func(t *testing.T) {
		stateSlot := slotsPerEpoch * 2    // epoch 2
		querySlot := slotsPerEpoch*3 + 10 // slot 106, epoch 3 (lookahead with MinSeedLookahead=1)
		offset, err := ptcWindowOffset(stateSlot, querySlot)
		require.NoError(t, err)
		// epoch_diff = 3-2 = 1, offset = (1+1)*32 + 10 = 74
		expected := slotsPerEpoch.Mul(uint64(slots.ToEpoch(querySlot)-slots.ToEpoch(stateSlot)+1)) + (querySlot % slotsPerEpoch)
		require.Equal(t, expected, offset)
	})

	t.Run("error: epoch too far in past", func(t *testing.T) {
		stateSlot := slotsPerEpoch * 3 // epoch 3
		querySlot := slotsPerEpoch + 1 // slot 33, epoch 1 (two epochs back)
		_, err := ptcWindowOffset(stateSlot, querySlot)
		require.ErrorContains(t, "only supports previous epoch lookups", err)
	})

	t.Run("error: epoch too far in future", func(t *testing.T) {
		stateSlot := slotsPerEpoch * 2   // epoch 2
		querySlot := slotsPerEpoch*4 + 1 // epoch 4 (beyond MinSeedLookahead=1 from epoch 2)
		_, err := ptcWindowOffset(stateSlot, querySlot)
		require.ErrorContains(t, "out of range", err)
	})
}

func TestPayloadCommitteeReadOnly(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		_, err := st.PayloadCommitteeReadOnly(0)
		require.ErrorContains(t, "PayloadCommitteeReadOnly", err)
	})

	t.Run("returns committee from current epoch", func(t *testing.T) {
		window := testPTCWindow(t)
		st := &BeaconState{
			version:   version.Gloas,
			slot:      slotsPerEpoch * 2, // epoch 2
			ptcWindow: window,
		}
		// Query slot 64 (first slot of epoch 2) → offset = 32
		ptc, err := st.PayloadCommitteeReadOnly(slotsPerEpoch * 2)
		require.NoError(t, err)
		require.Equal(t, primitives.ValidatorIndex(slotsPerEpoch+1), ptc[0])
	})

	t.Run("returns committee from previous epoch", func(t *testing.T) {
		window := testPTCWindow(t)
		st := &BeaconState{
			version:   version.Gloas,
			slot:      slotsPerEpoch * 2, // epoch 2
			ptcWindow: window,
		}
		// Query slot 35 (epoch 1, offset 3) → previous epoch region, offset = 3
		ptc, err := st.PayloadCommitteeReadOnly(slotsPerEpoch + 3)
		require.NoError(t, err)
		require.Equal(t, primitives.ValidatorIndex(4), ptc[0]) // window[3] has marker 4
	})

	t.Run("error on nil ptc slot", func(t *testing.T) {
		window := testPTCWindow(t)
		window[slotsPerEpoch] = nil // nil out the first current-epoch slot
		st := &BeaconState{
			version:   version.Gloas,
			slot:      slotsPerEpoch * 2,
			ptcWindow: window,
		}
		_, err := st.PayloadCommitteeReadOnly(slotsPerEpoch * 2)
		require.ErrorContains(t, "is nil", err)
	})
}

func TestSetPTCWindow(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.SetPTCWindow(nil)
		require.ErrorContains(t, "SetPTCWindow", err)
	})

	t.Run("rejects wrong size", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			PtcWindow: testPTCWindow(t),
		})
		require.NoError(t, err)
		err = st.SetPTCWindow(make([]*ethpb.PTCs, 10))
		require.ErrorContains(t, "invalid size", err)
	})

	t.Run("sets and copies window", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			PtcWindow: testPTCWindow(t),
		})
		require.NoError(t, err)

		newWindow := testPTCWindow(t)
		newWindow[0].ValidatorIndices[0] = 999
		require.NoError(t, st.SetPTCWindow(newWindow))

		got, err := st.PTCWindow()
		require.NoError(t, err)
		require.Equal(t, primitives.ValidatorIndex(999), got[0].ValidatorIndices[0])

		// Verify it's a copy — mutating the input doesn't affect state.
		newWindow[0].ValidatorIndices[0] = 0
		got2, err := st.PTCWindow()
		require.NoError(t, err)
		require.Equal(t, primitives.ValidatorIndex(999), got2[0].ValidatorIndices[0])
	})
}

func TestRotatePTCWindow(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.RotatePTCWindow(nil)
		require.ErrorContains(t, "RotatePTCWindow", err)
	})

	t.Run("rejects wrong new epoch size", func(t *testing.T) {
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			PtcWindow: testPTCWindow(t),
		})
		require.NoError(t, err)
		err = st.RotatePTCWindow(make([]*ethpb.PTCs, 5))
		require.ErrorContains(t, "invalid new epoch slots size", err)
	})

	t.Run("rotates window correctly", func(t *testing.T) {
		origWindow := testPTCWindow(t)
		st, err := InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			PtcWindow: origWindow,
		})
		require.NoError(t, err)

		// Build new epoch slots with distinct markers.
		newEpoch := make([]*ethpb.PTCs, slotsPerEpoch)
		for i := range newEpoch {
			indices := make([]primitives.ValidatorIndex, fieldparams.PTCSize)
			indices[0] = primitives.ValidatorIndex(1000 + i)
			newEpoch[i] = &ethpb.PTCs{ValidatorIndices: indices}
		}
		require.NoError(t, st.RotatePTCWindow(newEpoch))

		got, err := st.PTCWindow()
		require.NoError(t, err)

		// First two epochs should be shifted from original epochs 1 and 2.
		for i := range 2 * slotsPerEpoch {
			expected := origWindow[slotsPerEpoch+i].ValidatorIndices[0]
			require.Equal(t, expected, got[i].ValidatorIndices[0],
				fmt.Sprintf("mismatch at shifted slot %d", i))
		}

		// Last epoch should be the new epoch slots.
		lastStart := 2 * slotsPerEpoch
		for i := range slotsPerEpoch {
			require.Equal(t, primitives.ValidatorIndex(1000+i), got[lastStart+i].ValidatorIndices[0],
				fmt.Sprintf("mismatch at new slot %d", i))
		}
	})
}
