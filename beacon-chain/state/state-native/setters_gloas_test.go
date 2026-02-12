package state_native

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

type testExecutionPayloadBid struct {
	parentBlockHash    [32]byte
	parentBlockRoot    [32]byte
	blockHash          [32]byte
	prevRandao         [32]byte
	blobKzgCommitments [][]byte
	feeRecipient       [20]byte
	gasLimit           uint64
	builderIndex       primitives.BuilderIndex
	slot               primitives.Slot
	value              primitives.Gwei
	executionPayment   primitives.Gwei
}

func (t testExecutionPayloadBid) ParentBlockHash() [32]byte { return t.parentBlockHash }
func (t testExecutionPayloadBid) ParentBlockRoot() [32]byte { return t.parentBlockRoot }
func (t testExecutionPayloadBid) PrevRandao() [32]byte      { return t.prevRandao }
func (t testExecutionPayloadBid) BlockHash() [32]byte       { return t.blockHash }
func (t testExecutionPayloadBid) GasLimit() uint64          { return t.gasLimit }
func (t testExecutionPayloadBid) BuilderIndex() primitives.BuilderIndex {
	return t.builderIndex
}
func (t testExecutionPayloadBid) Slot() primitives.Slot  { return t.slot }
func (t testExecutionPayloadBid) Value() primitives.Gwei { return t.value }
func (t testExecutionPayloadBid) ExecutionPayment() primitives.Gwei {
	return t.executionPayment
}
func (t testExecutionPayloadBid) BlobKzgCommitments() [][]byte { return t.blobKzgCommitments }
func (t testExecutionPayloadBid) BlobKzgCommitmentCount() uint64 {
	return uint64(len(t.blobKzgCommitments))
}
func (t testExecutionPayloadBid) FeeRecipient() [20]byte { return t.feeRecipient }
func (t testExecutionPayloadBid) IsNil() bool            { return false }

func TestSetExecutionPayloadBid(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.SetExecutionPayloadBid(testExecutionPayloadBid{})
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("sets bid and marks dirty", func(t *testing.T) {
		var (
			parentBlockHash = [32]byte(bytes.Repeat([]byte{0xAB}, 32))
			parentBlockRoot = [32]byte(bytes.Repeat([]byte{0xCD}, 32))
			blockHash       = [32]byte(bytes.Repeat([]byte{0xEF}, 32))
			prevRandao      = [32]byte(bytes.Repeat([]byte{0x11}, 32))
			blobCommitments = [][]byte{bytes.Repeat([]byte{0x22}, 48)}
			feeRecipient    [20]byte
		)
		copy(feeRecipient[:], bytes.Repeat([]byte{0x33}, len(feeRecipient)))
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
		}
		bid := testExecutionPayloadBid{
			parentBlockHash:    parentBlockHash,
			parentBlockRoot:    parentBlockRoot,
			blockHash:          blockHash,
			prevRandao:         prevRandao,
			blobKzgCommitments: blobCommitments,
			feeRecipient:       feeRecipient,
			gasLimit:           123,
			builderIndex:       7,
			slot:               9,
			value:              11,
			executionPayment:   22,
		}

		require.NoError(t, st.SetExecutionPayloadBid(bid))

		require.NotNil(t, st.latestExecutionPayloadBid)
		require.DeepEqual(t, parentBlockHash[:], st.latestExecutionPayloadBid.ParentBlockHash)
		require.DeepEqual(t, parentBlockRoot[:], st.latestExecutionPayloadBid.ParentBlockRoot)
		require.DeepEqual(t, blockHash[:], st.latestExecutionPayloadBid.BlockHash)
		require.DeepEqual(t, prevRandao[:], st.latestExecutionPayloadBid.PrevRandao)
		require.DeepEqual(t, blobCommitments, st.latestExecutionPayloadBid.BlobKzgCommitments)
		require.DeepEqual(t, feeRecipient[:], st.latestExecutionPayloadBid.FeeRecipient)
		require.Equal(t, uint64(123), st.latestExecutionPayloadBid.GasLimit)
		require.Equal(t, primitives.BuilderIndex(7), st.latestExecutionPayloadBid.BuilderIndex)
		require.Equal(t, primitives.Slot(9), st.latestExecutionPayloadBid.Slot)
		require.Equal(t, primitives.Gwei(11), st.latestExecutionPayloadBid.Value)
		require.Equal(t, primitives.Gwei(22), st.latestExecutionPayloadBid.ExecutionPayment)
		require.Equal(t, true, st.dirtyFields[types.LatestExecutionPayloadBid])
	})
}

func TestSetBuilderPendingPayment(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.SetBuilderPendingPayment(0, &ethpb.BuilderPendingPayment{})
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("sets copy and marks dirty", func(t *testing.T) {
		st := &BeaconState{
			version:                version.Gloas,
			dirtyFields:            make(map[types.FieldIndex]bool),
			builderPendingPayments: make([]*ethpb.BuilderPendingPayment, 2),
		}
		payment := &ethpb.BuilderPendingPayment{
			Weight: 2,
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				Amount:       99,
				BuilderIndex: 1,
			},
		}

		require.NoError(t, st.SetBuilderPendingPayment(1, payment))
		require.DeepEqual(t, payment, st.builderPendingPayments[1])
		require.Equal(t, true, st.dirtyFields[types.BuilderPendingPayments])

		// Mutating the original should not affect the state copy.
		payment.Withdrawal.Amount = 12345
		require.Equal(t, primitives.Gwei(99), st.builderPendingPayments[1].Withdrawal.Amount)
	})

	t.Run("returns error on out of range index", func(t *testing.T) {
		st := &BeaconState{
			version:                version.Gloas,
			dirtyFields:            make(map[types.FieldIndex]bool),
			builderPendingPayments: make([]*ethpb.BuilderPendingPayment, 1),
		}

		err := st.SetBuilderPendingPayment(2, &ethpb.BuilderPendingPayment{})

		require.ErrorContains(t, "out of range", err)
		require.Equal(t, false, st.dirtyFields[types.BuilderPendingPayments])
	})
}

func TestClearBuilderPendingPayment(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.ClearBuilderPendingPayment(0)
		require.ErrorContains(t, "is not supported", err)
	})

	t.Run("clears and marks dirty", func(t *testing.T) {
		st := &BeaconState{
			version:                version.Gloas,
			dirtyFields:            make(map[types.FieldIndex]bool),
			builderPendingPayments: make([]*ethpb.BuilderPendingPayment, 2),
		}
		st.builderPendingPayments[1] = &ethpb.BuilderPendingPayment{
			Weight: 2,
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				Amount:       99,
				BuilderIndex: 1,
			},
		}

		require.NoError(t, st.ClearBuilderPendingPayment(1))
		require.Equal(t, emptyBuilderPendingPayment, st.builderPendingPayments[1])
		require.Equal(t, true, st.dirtyFields[types.BuilderPendingPayments])
	})

	t.Run("returns error on out of range index", func(t *testing.T) {
		st := &BeaconState{
			version:                version.Gloas,
			dirtyFields:            make(map[types.FieldIndex]bool),
			builderPendingPayments: make([]*ethpb.BuilderPendingPayment, 1),
		}

		err := st.ClearBuilderPendingPayment(2)

		require.ErrorContains(t, "out of range", err)
		require.Equal(t, false, st.dirtyFields[types.BuilderPendingPayments])
	})
}

func TestUpdatePendingPaymentWeight(t *testing.T) {
	cfg := params.BeaconConfig()
	slotsPerEpoch := cfg.SlotsPerEpoch
	slot := primitives.Slot(4)
	stateSlot := slot + 1
	stateEpoch := slots.ToEpoch(stateSlot)

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
			targetEpoch:   stateEpoch,
			blockRoot:     rootA,
			initialAmount: 1,
			initialWeight: 0,
			wantWeight:    primitives.Gwei(cfg.MinActivationBalance),
		},
		{
			name:          "same slot zero amount no weight change",
			targetEpoch:   stateEpoch,
			blockRoot:     rootA,
			initialAmount: 0,
			initialWeight: 5,
			wantWeight:    5,
		},
		{
			name:          "non matching block root no change",
			targetEpoch:   stateEpoch,
			blockRoot:     rootB,
			initialAmount: 1,
			initialWeight: 7,
			wantWeight:    7,
		},
		{
			name:          "previous epoch target uses earlier slot",
			targetEpoch:   stateEpoch - 1,
			blockRoot:     rootA,
			initialAmount: 1,
			initialWeight: 0,
			wantWeight:    primitives.Gwei(cfg.MinActivationBalance),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var paymentIdx int
			if tt.targetEpoch == stateEpoch {
				paymentIdx = int(slotsPerEpoch + (slot % slotsPerEpoch))
			} else {
				paymentIdx = int(slot % slotsPerEpoch)
			}
			state := buildGloasStateForPaymentWeightTest(t, stateSlot, paymentIdx, tt.initialAmount, tt.initialWeight, map[primitives.Slot][]byte{
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

			require.NoError(t, state.UpdatePendingPaymentWeight(att, indices, participatedFlags))

			payment, err := state.BuilderPendingPayment(uint64(paymentIdx))
			require.NoError(t, err)
			require.Equal(t, tt.wantWeight, payment.Weight)
		})
	}
}

func TestRotateBuilderPendingPayments(t *testing.T) {
	totalPayments := 2 * params.BeaconConfig().SlotsPerEpoch
	payments := make([]*ethpb.BuilderPendingPayment, totalPayments)
	for i := range payments {
		idx := uint64(i)
		payments[i] = &ethpb.BuilderPendingPayment{
			Weight: primitives.Gwei(idx * 100e9),
			Withdrawal: &ethpb.BuilderPendingWithdrawal{
				FeeRecipient: make([]byte, 20),
				Amount:       primitives.Gwei(idx * 1e9),
				BuilderIndex: primitives.BuilderIndex(idx + 100),
			},
		}
	}

	statePb, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		BuilderPendingPayments: payments,
	})
	require.NoError(t, err)
	st, ok := statePb.(*BeaconState)
	require.Equal(t, true, ok)

	oldPayments, err := st.BuilderPendingPayments()
	require.NoError(t, err)
	require.NoError(t, st.RotateBuilderPendingPayments())

	newPayments, err := st.BuilderPendingPayments()
	require.NoError(t, err)
	slotsPerEpoch := int(params.BeaconConfig().SlotsPerEpoch)
	for i := range slotsPerEpoch {
		require.DeepEqual(t, oldPayments[slotsPerEpoch+i], newPayments[i])
	}

	for i := slotsPerEpoch; i < 2*slotsPerEpoch; i++ {
		payment := newPayments[i]
		require.Equal(t, primitives.Gwei(0), payment.Weight)
		require.Equal(t, 20, len(payment.Withdrawal.FeeRecipient))
		require.Equal(t, primitives.Gwei(0), payment.Withdrawal.Amount)
		require.Equal(t, primitives.BuilderIndex(0), payment.Withdrawal.BuilderIndex)
	}
}

func TestRotateBuilderPendingPayments_UnsupportedVersion(t *testing.T) {
	st := &BeaconState{version: version.Electra}
	err := st.RotateBuilderPendingPayments()
	require.ErrorContains(t, "RotateBuilderPendingPayments", err)
}

func TestAppendBuilderPendingWithdrawal_CopyOnWrite(t *testing.T) {
	wd := &ethpb.BuilderPendingWithdrawal{
		FeeRecipient: make([]byte, 20),
		Amount:       1,
		BuilderIndex: 2,
	}
	statePb, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		BuilderPendingWithdrawals: []*ethpb.BuilderPendingWithdrawal{wd},
	})
	require.NoError(t, err)

	st, ok := statePb.(*BeaconState)
	require.Equal(t, true, ok)

	copied := st.Copy().(*BeaconState)
	require.Equal(t, uint(2), st.sharedFieldReferences[types.BuilderPendingWithdrawals].Refs())

	appended := &ethpb.BuilderPendingWithdrawal{
		FeeRecipient: make([]byte, 20),
		Amount:       4,
		BuilderIndex: 5,
	}
	require.NoError(t, copied.AppendBuilderPendingWithdrawals([]*ethpb.BuilderPendingWithdrawal{appended}))

	require.Equal(t, 1, len(st.builderPendingWithdrawals))
	require.Equal(t, 2, len(copied.builderPendingWithdrawals))
	require.DeepEqual(t, wd, copied.builderPendingWithdrawals[0])
	require.DeepEqual(t, appended, copied.builderPendingWithdrawals[1])
	require.DeepEqual(t, wd, st.builderPendingWithdrawals[0])
	require.Equal(t, uint(1), st.sharedFieldReferences[types.BuilderPendingWithdrawals].Refs())
	require.Equal(t, uint(1), copied.sharedFieldReferences[types.BuilderPendingWithdrawals].Refs())
}

func TestAppendBuilderPendingWithdrawals(t *testing.T) {
	st := &BeaconState{
		version:     version.Gloas,
		dirtyFields: make(map[types.FieldIndex]bool),
		sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
			types.BuilderPendingWithdrawals: stateutil.NewRef(1),
		},
		builderPendingWithdrawals: make([]*ethpb.BuilderPendingWithdrawal, 0),
	}

	first := &ethpb.BuilderPendingWithdrawal{Amount: 1}
	second := &ethpb.BuilderPendingWithdrawal{Amount: 2}
	require.NoError(t, st.AppendBuilderPendingWithdrawals([]*ethpb.BuilderPendingWithdrawal{first, second}))

	require.Equal(t, 2, len(st.builderPendingWithdrawals))
	require.DeepEqual(t, first, st.builderPendingWithdrawals[0])
	require.DeepEqual(t, second, st.builderPendingWithdrawals[1])
	require.Equal(t, true, st.dirtyFields[types.BuilderPendingWithdrawals])
}

func TestAppendBuilderPendingWithdrawals_UnsupportedVersion(t *testing.T) {
	st := &BeaconState{version: version.Electra}
	err := st.AppendBuilderPendingWithdrawals([]*ethpb.BuilderPendingWithdrawal{{}})
	require.ErrorContains(t, "AppendBuilderPendingWithdrawals", err)
}

func TestUpdateExecutionPayloadAvailabilityAtIndex_SetAndClear(t *testing.T) {
	st := newGloasStateWithAvailability(t, make([]byte, 1024))

	otherIdx := uint64(8) // byte 1, bit 0
	idx := uint64(9)      // byte 1, bit 1

	require.NoError(t, st.UpdateExecutionPayloadAvailabilityAtIndex(otherIdx, 1))
	require.Equal(t, byte(0x01), st.executionPayloadAvailability[1])

	require.NoError(t, st.UpdateExecutionPayloadAvailabilityAtIndex(idx, 1))
	require.Equal(t, byte(0x03), st.executionPayloadAvailability[1])

	require.NoError(t, st.UpdateExecutionPayloadAvailabilityAtIndex(idx, 0))
	require.Equal(t, byte(0x01), st.executionPayloadAvailability[1])
}

func TestUpdateExecutionPayloadAvailabilityAtIndex_OutOfRange(t *testing.T) {
	st := newGloasStateWithAvailability(t, make([]byte, 1024))

	idx := uint64(len(st.executionPayloadAvailability)) * 8
	err := st.UpdateExecutionPayloadAvailabilityAtIndex(idx, 1)
	require.ErrorContains(t, "out of range", err)

	for _, b := range st.executionPayloadAvailability {
		if b != 0 {
			t.Fatalf("execution payload availability mutated on error")
		}
	}
}

func buildGloasStateForPaymentWeightTest(
	t *testing.T,
	stateSlot primitives.Slot,
	paymentIdx int,
	amount primitives.Gwei,
	weight primitives.Gwei,
	roots map[primitives.Slot][]byte,
) *BeaconState {
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

	statePb, err := InitializeFromProtoGloas(stProto)
	require.NoError(t, err)
	return statePb.(*BeaconState)
}

func newGloasStateWithAvailability(t *testing.T, availability []byte) *BeaconState {
	t.Helper()

	st, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		ExecutionPayloadAvailability: availability,
	})
	require.NoError(t, err)

	return st.(*BeaconState)
}
