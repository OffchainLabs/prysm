package state_native

import (
	"bytes"
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

func TestIsParentBlockFull(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		_, err := st.IsParentBlockFull()
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("returns false when bid is nil", func(t *testing.T) {
		st := &BeaconState{version: version.Gloas}
		got, err := st.IsParentBlockFull()
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

		got, err := st.IsParentBlockFull()
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

		got, err := st.IsParentBlockFull()
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
