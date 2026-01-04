package gloas

import (
	"bytes"
	"testing"

	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func buildStateWithBlockRoots(t *testing.T, stateSlot primitives.Slot, roots map[primitives.Slot][]byte) *state_native.BeaconState {
	t.Helper()

	cfg := params.BeaconConfig()
	blockRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	for slot, root := range roots {
		blockRoots[slot%cfg.SlotsPerHistoricalRoot] = root
	}

	stateRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	for i := range stateRoots {
		stateRoots[i] = bytes.Repeat([]byte{0x11}, 32)
	}

	randaoMixes := make([][]byte, cfg.EpochsPerHistoricalVector)
	for i := range randaoMixes {
		randaoMixes[i] = bytes.Repeat([]byte{0x22}, 32)
	}

	execPayloadAvailability := make([]byte, cfg.SlotsPerHistoricalRoot/8)

	stProto := &ethpb.BeaconStateGloas{
		Slot:                  stateSlot,
		GenesisValidatorsRoot: bytes.Repeat([]byte{0x33}, 32),
		Fork: &ethpb.Fork{
			CurrentVersion:  bytes.Repeat([]byte{0x44}, 4),
			PreviousVersion: bytes.Repeat([]byte{0x44}, 4),
			Epoch:           0,
		},
		BlockRoots:                   blockRoots,
		StateRoots:                   stateRoots,
		RandaoMixes:                  randaoMixes,
		ExecutionPayloadAvailability: execPayloadAvailability,
		Validators:                   []*ethpb.Validator{},
		Balances:                     []uint64{},
		BuilderPendingPayments:       make([]*ethpb.BuilderPendingPayment, cfg.SlotsPerEpoch*2),
		BuilderPendingWithdrawals:    []*ethpb.BuilderPendingWithdrawal{},
	}

	state, err := state_native.InitializeFromProtoGloas(stProto)
	require.NoError(t, err)
	return state.(*state_native.BeaconState)
}

func TestSameSlotAttestation(t *testing.T) {
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
			state := buildStateWithBlockRoots(t, tt.stateSlot, tt.roots)
			var rootArr [32]byte
			copy(rootArr[:], tt.blockRoot)

			got, err := SameSlotAttestation(state, rootArr, tt.slot)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func buildStateForPaymentTest(t *testing.T, stateSlot primitives.Slot, paymentIdx int, amount primitives.Gwei, weight primitives.Gwei, roots map[primitives.Slot][]byte) *state_native.BeaconState {
	t.Helper()

	cfg := params.BeaconConfig()
	blockRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	for slot, root := range roots {
		blockRoots[slot%cfg.SlotsPerHistoricalRoot] = root
	}

	stateRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	for i := range stateRoots {
		stateRoots[i] = bytes.Repeat([]byte{0x44}, 32)
	}
	randaoMixes := make([][]byte, cfg.EpochsPerHistoricalVector)
	for i := range randaoMixes {
		randaoMixes[i] = bytes.Repeat([]byte{0x55}, 32)
	}

	validator := &ethpb.Validator{
		PublicKey:             bytes.Repeat([]byte{0x01}, 48),
		WithdrawalCredentials: append([]byte{cfg.ETH1AddressWithdrawalPrefixByte}, bytes.Repeat([]byte{0x02}, 31)...),
		EffectiveBalance:      cfg.MinActivationBalance,
	}

	payments := make([]*ethpb.BuilderPendingPayment, cfg.SlotsPerEpoch*2)
	for i := range payments {
		payments[i] = &ethpb.BuilderPendingPayment{
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, 20),
			},
		}
	}
	payments[paymentIdx] = &ethpb.BuilderPendingPayment{
		Weight: weight,
		Withdrawal: &ethpb.BuilderPendingWithdrawal{
			FeeRecipient: make([]byte, 20),
			Amount:       amount,
		},
	}

	execPayloadAvailability := make([]byte, cfg.SlotsPerHistoricalRoot/8)

	stProto := &ethpb.BeaconStateGloas{
		Slot:                         stateSlot,
		GenesisValidatorsRoot:        bytes.Repeat([]byte{0x33}, 32),
		BlockRoots:                   blockRoots,
		StateRoots:                   stateRoots,
		RandaoMixes:                  randaoMixes,
		ExecutionPayloadAvailability: execPayloadAvailability,
		Validators:                   []*ethpb.Validator{validator},
		Balances:                     []uint64{cfg.MinActivationBalance},
		CurrentEpochParticipation:    []byte{0},
		PreviousEpochParticipation:   []byte{0},
		BuilderPendingPayments:       payments,
		Fork: &ethpb.Fork{
			CurrentVersion:  bytes.Repeat([]byte{0x66}, 4),
			PreviousVersion: bytes.Repeat([]byte{0x66}, 4),
			Epoch:           0,
		},
	}

	state, err := state_native.InitializeFromProtoGloas(stProto)
	require.NoError(t, err)
	return state.(*state_native.BeaconState)
}

func TestUpdatePendingPaymentWeight(t *testing.T) {
	cfg := params.BeaconConfig()
	slotsPerEpoch := cfg.SlotsPerEpoch
	slot := primitives.Slot(4)
	stateSlot := slot + 1
	currentEpoch := slots.ToEpoch(stateSlot)

	rootA := bytes.Repeat([]byte{0xAA}, 32)
	rootB := bytes.Repeat([]byte{0xBB}, 32)

	tests := []struct {
		name          string
		targetEpoch   primitives.Epoch
		blockRoot     []byte
		initialAmount primitives.Gwei
		initialWeight primitives.Gwei
		wantWeight    primitives.Gwei
	}{
		{
			name:          "same slot current epoch adds weight",
			targetEpoch:   currentEpoch,
			blockRoot:     rootA,
			initialAmount: 10,
			initialWeight: 0,
			wantWeight:    primitives.Gwei(cfg.MinActivationBalance),
		},
		{
			name:          "same slot zero amount no weight change",
			targetEpoch:   currentEpoch,
			blockRoot:     rootA,
			initialAmount: 0,
			initialWeight: 5,
			wantWeight:    5,
		},
		{
			name:          "non matching block root no change",
			targetEpoch:   currentEpoch,
			blockRoot:     rootB,
			initialAmount: 10,
			initialWeight: 7,
			wantWeight:    7,
		},
		{
			name:          "previous epoch target uses earlier slot",
			targetEpoch:   currentEpoch - 1,
			blockRoot:     rootA,
			initialAmount: 20,
			initialWeight: 0,
			wantWeight:    primitives.Gwei(cfg.MinActivationBalance),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var paymentIdx int
			if tt.targetEpoch == currentEpoch {
				paymentIdx = int(slotsPerEpoch + (slot % slotsPerEpoch))
			} else {
				paymentIdx = int(slot % slotsPerEpoch)
			}
			state := buildStateForPaymentTest(t, stateSlot, paymentIdx, tt.initialAmount, tt.initialWeight, map[primitives.Slot][]byte{
				slot:     tt.blockRoot,
				slot - 1: rootB,
			})

			att := &ethpb.Attestation{
				Data: &ethpb.AttestationData{
					Slot:            slot,
					CommitteeIndex:  0,
					BeaconBlockRoot: tt.blockRoot,
					Source:          &ethpb.Checkpoint{},
					Target: &ethpb.Checkpoint{
						Epoch: tt.targetEpoch,
					},
				},
			}

			participatedFlags := map[uint8]bool{
				cfg.TimelySourceFlagIndex: true,
				cfg.TimelyTargetFlagIndex: true,
				cfg.TimelyHeadFlagIndex:   true,
			}
			indices := []uint64{0}

			gotState, err := UpdatePendingPaymentWeight(state, att, indices, participatedFlags)
			require.NoError(t, err)

			payment, err := gotState.BuilderPendingPayment(uint64(paymentIdx))
			require.NoError(t, err)
			require.Equal(t, tt.wantWeight, payment.Weight)
		})
	}
}
