package state_native

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
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
func (t testExecutionPayloadBid) FeeRecipient() [20]byte          { return t.feeRecipient }
func (t testExecutionPayloadBid) ExecutionRequestsRoot() [32]byte { return [32]byte{} }
func (t testExecutionPayloadBid) IsNil() bool                     { return false }

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
		require.DeepEqual(t, emptyBuilderPendingPayment, st.builderPendingPayments[1])
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

func TestSetPayloadExpectedWithdrawals(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.SetPayloadExpectedWithdrawals([]*enginev1.Withdrawal{})
		require.ErrorContains(t, "SetPayloadExpectedWithdrawals", err)
	})

	t.Run("allows nil input and marks dirty", func(t *testing.T) {
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
		}

		require.NoError(t, st.SetPayloadExpectedWithdrawals(nil))
		require.Equal(t, true, st.payloadExpectedWithdrawals == nil)
		require.Equal(t, true, st.dirtyFields[types.PayloadExpectedWithdrawals])
	})

	t.Run("sets and marks dirty", func(t *testing.T) {
		st := &BeaconState{
			version:                    version.Gloas,
			dirtyFields:                make(map[types.FieldIndex]bool),
			payloadExpectedWithdrawals: []*enginev1.Withdrawal{{Index: 1}, {Index: 2}},
		}

		withdrawals := []*enginev1.Withdrawal{{Index: 3}}
		require.NoError(t, st.SetPayloadExpectedWithdrawals(withdrawals))

		require.DeepEqual(t, withdrawals, st.payloadExpectedWithdrawals)
		require.Equal(t, true, st.dirtyFields[types.PayloadExpectedWithdrawals])
	})
}

func TestDecreaseWithdrawalBalances(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.DecreaseWithdrawalBalances([]*enginev1.Withdrawal{{}})
		require.ErrorContains(t, "DecreaseWithdrawalBalances", err)
	})

	t.Run("rejects nil withdrawal", func(t *testing.T) {
		st := &BeaconState{version: version.Gloas}
		err := st.DecreaseWithdrawalBalances([]*enginev1.Withdrawal{nil})
		require.ErrorContains(t, "withdrawal is nil", err)
	})

	t.Run("no-op on empty input", func(t *testing.T) {
		st := &BeaconState{
			version:      version.Gloas,
			dirtyFields:  make(map[types.FieldIndex]bool),
			dirtyIndices: make(map[types.FieldIndex][]uint64),
			rebuildTrie:  make(map[types.FieldIndex]bool),
		}

		require.NoError(t, st.DecreaseWithdrawalBalances(nil))
		require.Equal(t, 0, len(st.dirtyFields))
		require.Equal(t, 0, len(st.dirtyIndices))
	})

	t.Run("updates validator and builder balances and tracks dirty indices", func(t *testing.T) {
		st := &BeaconState{
			version:      version.Gloas,
			dirtyFields:  make(map[types.FieldIndex]bool),
			dirtyIndices: make(map[types.FieldIndex][]uint64),
			rebuildTrie:  make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.Builders: stateutil.NewRef(1),
			},
			balancesMultiValue: NewMultiValueBalances([]uint64{100, 200, 300}),
			builders: []*ethpb.Builder{
				{Balance: 1000},
				{Balance: 50},
			},
		}

		withdrawals := []*enginev1.Withdrawal{
			{ValidatorIndex: primitives.ValidatorIndex(1), Amount: 20},
			{ValidatorIndex: primitives.BuilderIndex(1).ToValidatorIndex(), Amount: 30},
			{ValidatorIndex: primitives.ValidatorIndex(2), Amount: 400},
			{ValidatorIndex: primitives.BuilderIndex(0).ToValidatorIndex(), Amount: 2000},
			{ValidatorIndex: primitives.ValidatorIndex(0), Amount: 0},
		}

		require.NoError(t, st.DecreaseWithdrawalBalances(withdrawals))

		require.DeepEqual(t, []uint64{100, 180, 0}, st.Balances())
		require.Equal(t, primitives.Gwei(0), st.builders[0].Balance)
		require.Equal(t, primitives.Gwei(20), st.builders[1].Balance)

		require.Equal(t, true, st.dirtyFields[types.Balances])
		require.Equal(t, true, st.dirtyFields[types.Builders])
		require.DeepEqual(t, []uint64{1, 2}, st.dirtyIndices[types.Balances])
		require.DeepEqual(t, []uint64{1, 0}, st.dirtyIndices[types.Builders])
	})

	t.Run("returns error on builder index out of range", func(t *testing.T) {
		st := &BeaconState{
			version:      version.Gloas,
			dirtyFields:  make(map[types.FieldIndex]bool),
			dirtyIndices: make(map[types.FieldIndex][]uint64),
			rebuildTrie:  make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.Builders: stateutil.NewRef(1),
			},
			builders: []*ethpb.Builder{{Balance: 5}},
		}

		err := st.DecreaseWithdrawalBalances([]*enginev1.Withdrawal{
			{ValidatorIndex: primitives.BuilderIndex(2).ToValidatorIndex(), Amount: 1},
		})
		require.ErrorContains(t, "out of range", err)
		require.Equal(t, false, st.dirtyFields[types.Builders])
		require.Equal(t, 0, len(st.dirtyIndices[types.Builders]))
	})
}

func TestDequeueBuilderPendingWithdrawals(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.DequeueBuilderPendingWithdrawals(1)
		require.ErrorContains(t, "DequeueBuilderPendingWithdrawals", err)
	})

	t.Run("returns error when dequeueing more than length", func(t *testing.T) {
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.BuilderPendingWithdrawals: stateutil.NewRef(1),
			},
			builderPendingWithdrawals: []*ethpb.BuilderPendingWithdrawal{{Amount: 1}},
		}

		err := st.DequeueBuilderPendingWithdrawals(2)
		require.ErrorContains(t, "cannot dequeue more builder withdrawals", err)
		require.Equal(t, 1, len(st.builderPendingWithdrawals))
		require.Equal(t, false, st.dirtyFields[types.BuilderPendingWithdrawals])
	})

	t.Run("no-op on zero", func(t *testing.T) {
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.BuilderPendingWithdrawals: stateutil.NewRef(1),
			},
			builderPendingWithdrawals: []*ethpb.BuilderPendingWithdrawal{{Amount: 1}},
		}

		require.NoError(t, st.DequeueBuilderPendingWithdrawals(0))
		require.Equal(t, 1, len(st.builderPendingWithdrawals))
		require.Equal(t, false, st.dirtyFields[types.BuilderPendingWithdrawals])
		require.Equal(t, false, st.rebuildTrie[types.BuilderPendingWithdrawals])
	})

	t.Run("dequeues and marks dirty", func(t *testing.T) {
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.BuilderPendingWithdrawals: stateutil.NewRef(1),
			},
			builderPendingWithdrawals: []*ethpb.BuilderPendingWithdrawal{
				{Amount: 1},
				{Amount: 2},
				{Amount: 3},
			},
			rebuildTrie: make(map[types.FieldIndex]bool),
		}

		require.NoError(t, st.DequeueBuilderPendingWithdrawals(2))
		require.Equal(t, 1, len(st.builderPendingWithdrawals))
		require.Equal(t, primitives.Gwei(3), st.builderPendingWithdrawals[0].Amount)
		require.Equal(t, true, st.dirtyFields[types.BuilderPendingWithdrawals])
		require.Equal(t, true, st.rebuildTrie[types.BuilderPendingWithdrawals])
	})

	t.Run("copy-on-write preserves shared state", func(t *testing.T) {
		sharedRef := stateutil.NewRef(2)
		sharedWithdrawals := []*ethpb.BuilderPendingWithdrawal{
			{Amount: 1},
			{Amount: 2},
			{Amount: 3},
		}

		st1 := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.BuilderPendingWithdrawals: sharedRef,
			},
			builderPendingWithdrawals: sharedWithdrawals,
			rebuildTrie:               make(map[types.FieldIndex]bool),
		}
		st2 := &BeaconState{
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.BuilderPendingWithdrawals: sharedRef,
			},
			builderPendingWithdrawals: sharedWithdrawals,
		}

		require.NoError(t, st1.DequeueBuilderPendingWithdrawals(2))
		require.Equal(t, primitives.Gwei(3), st1.builderPendingWithdrawals[0].Amount)
		require.Equal(t, 3, len(st2.builderPendingWithdrawals))
		require.Equal(t, primitives.Gwei(1), st2.builderPendingWithdrawals[0].Amount)
		require.Equal(t, uint(1), st1.sharedFieldReferences[types.BuilderPendingWithdrawals].Refs())
		require.Equal(t, uint(1), st2.sharedFieldReferences[types.BuilderPendingWithdrawals].Refs())
	})
}

func TestSetNextWithdrawalBuilderIndex(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.SetNextWithdrawalBuilderIndex(1)
		require.ErrorContains(t, "SetNextWithdrawalBuilderIndex", err)
	})

	t.Run("sets and marks dirty", func(t *testing.T) {
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
		}

		require.NoError(t, st.SetNextWithdrawalBuilderIndex(7))
		require.Equal(t, primitives.BuilderIndex(7), st.nextWithdrawalBuilderIndex)
		require.Equal(t, true, st.dirtyFields[types.NextWithdrawalBuilderIndex])
	})
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

func TestSetLatestBlockHash(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		var hash [32]byte
		st := &BeaconState{version: version.Fulu}
		err := st.SetLatestBlockHash(hash)
		require.ErrorContains(t, "SetLatestBlockHash", err)
	})

	var hash [32]byte
	copy(hash[:], []byte("latest-block-hash"))

	state := &BeaconState{
		version:     version.Gloas,
		dirtyFields: make(map[types.FieldIndex]bool),
	}

	require.NoError(t, state.SetLatestBlockHash(hash))
	require.Equal(t, true, state.dirtyFields[types.LatestBlockHash])
	require.DeepEqual(t, hash[:], state.latestBlockHash)
}

func TestSetExecutionPayloadAvailability(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.SetExecutionPayloadAvailability(0, true)
		require.ErrorContains(t, "SetExecutionPayloadAvailability", err)
	})

	state := &BeaconState{
		version:                      version.Gloas,
		executionPayloadAvailability: make([]byte, params.BeaconConfig().SlotsPerHistoricalRoot/8),
		dirtyFields:                  make(map[types.FieldIndex]bool),
	}

	slot := primitives.Slot(10)
	bitIndex := slot % params.BeaconConfig().SlotsPerHistoricalRoot
	byteIndex := bitIndex / 8
	bitPosition := bitIndex % 8

	require.NoError(t, state.SetExecutionPayloadAvailability(slot, true))
	require.Equal(t, true, state.dirtyFields[types.ExecutionPayloadAvailability])
	require.Equal(t, byte(1<<bitPosition), state.executionPayloadAvailability[byteIndex]&(1<<bitPosition))

	require.NoError(t, state.SetExecutionPayloadAvailability(slot, false))
	require.Equal(t, byte(0), state.executionPayloadAvailability[byteIndex]&(1<<bitPosition))
}

func TestSetExecutionPayloadAvailability_OutOfRange(t *testing.T) {
	state := &BeaconState{
		version:                      version.Gloas,
		executionPayloadAvailability: []byte{},
		dirtyFields:                  make(map[types.FieldIndex]bool),
	}

	err := state.SetExecutionPayloadAvailability(0, true)
	require.ErrorContains(t, "out of range", err)
	require.Equal(t, false, state.dirtyFields[types.ExecutionPayloadAvailability])
}

func TestIncreaseBuilderBalance(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.IncreaseBuilderBalance(0, 1)
		require.ErrorContains(t, "IncreaseBuilderBalance", err)
	})

	t.Run("out of bounds returns error", func(t *testing.T) {
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.Builders: stateutil.NewRef(1),
			},
			builders: []*ethpb.Builder{},
		}

		err := st.IncreaseBuilderBalance(0, 1)
		require.ErrorContains(t, "out of bounds", err)
		require.Equal(t, false, st.dirtyFields[types.Builders])
	})

	t.Run("nil builder returns error", func(t *testing.T) {
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.Builders: stateutil.NewRef(1),
			},
			builders: []*ethpb.Builder{nil},
		}

		err := st.IncreaseBuilderBalance(0, 1)
		require.ErrorContains(t, "is nil", err)
		require.Equal(t, false, st.dirtyFields[types.Builders])
	})

	t.Run("increments and marks dirty", func(t *testing.T) {
		orig := &ethpb.Builder{Balance: 10}
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.Builders: stateutil.NewRef(1),
			},
			builders: []*ethpb.Builder{orig},
		}

		require.NoError(t, st.IncreaseBuilderBalance(0, 5))
		require.Equal(t, primitives.Gwei(15), st.builders[0].Balance)
		require.Equal(t, true, st.dirtyFields[types.Builders])
		// Copy-on-write semantics: builder pointer replaced.
		require.NotEqual(t, orig, st.builders[0])
	})
}

func TestIncreaseBuilderBalance_CopyOnWrite(t *testing.T) {
	orig := &ethpb.Builder{Balance: 10}
	statePb, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		Builders: []*ethpb.Builder{orig},
	})
	require.NoError(t, err)

	st, ok := statePb.(*BeaconState)
	require.Equal(t, true, ok)

	copied := st.Copy().(*BeaconState)
	require.Equal(t, uint(2), st.sharedFieldReferences[types.Builders].Refs())

	require.NoError(t, copied.IncreaseBuilderBalance(0, 5))
	require.Equal(t, primitives.Gwei(10), st.builders[0].Balance)
	require.Equal(t, primitives.Gwei(15), copied.builders[0].Balance)
	require.Equal(t, uint(1), st.sharedFieldReferences[types.Builders].Refs())
	require.Equal(t, uint(1), copied.sharedFieldReferences[types.Builders].Refs())
}

func TestAddBuilderFromDeposit(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		var pubkey [48]byte
		var wc [32]byte
		st := &BeaconState{version: version.Fulu}
		err := st.AddBuilderFromDeposit(pubkey, wc, 1)
		require.ErrorContains(t, "AddBuilderFromDeposit", err)
	})

	t.Run("reuses empty withdrawable slot", func(t *testing.T) {
		var pubkey [48]byte
		copy(pubkey[:], bytes.Repeat([]byte{0xAA}, 48))
		var wc [32]byte
		copy(wc[:], bytes.Repeat([]byte{0xBB}, 32))
		wc[0] = 0x42 // version byte

		st := &BeaconState{
			version:     version.Gloas,
			slot:        0, // epoch 0
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.Builders: stateutil.NewRef(1),
			},
			builders: []*ethpb.Builder{
				{
					WithdrawableEpoch: 0,
					Balance:           0,
				},
			},
		}

		require.NoError(t, st.AddBuilderFromDeposit(pubkey, wc, 123))
		require.Equal(t, 1, len(st.builders))
		got := st.builders[0]
		require.NotNil(t, got)
		require.DeepEqual(t, pubkey[:], got.Pubkey)
		require.DeepEqual(t, []byte{0x42}, got.Version)
		require.DeepEqual(t, wc[12:], got.ExecutionAddress)
		require.Equal(t, primitives.Gwei(123), got.Balance)
		require.Equal(t, primitives.Epoch(0), got.DepositEpoch)
		require.Equal(t, params.BeaconConfig().FarFutureEpoch, got.WithdrawableEpoch)
		require.Equal(t, true, st.dirtyFields[types.Builders])
	})

	t.Run("appends new builder when no reusable slot", func(t *testing.T) {
		var pubkey [48]byte
		copy(pubkey[:], bytes.Repeat([]byte{0xAA}, 48))
		var wc [32]byte
		copy(wc[:], bytes.Repeat([]byte{0xBB}, 32))

		st := &BeaconState{
			version:     version.Gloas,
			slot:        0,
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.Builders: stateutil.NewRef(1),
			},
			builders: []*ethpb.Builder{
				{
					WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
					Balance:           1,
				},
			},
		}

		require.NoError(t, st.AddBuilderFromDeposit(pubkey, wc, 5))
		require.Equal(t, 2, len(st.builders))
		require.NotNil(t, st.builders[1])
		require.Equal(t, primitives.Gwei(5), st.builders[1].Balance)
	})
}

func TestAddBuilderFromDeposit_CopyOnWrite(t *testing.T) {
	var pubkey [48]byte
	copy(pubkey[:], bytes.Repeat([]byte{0xAA}, 48))
	var wc [32]byte
	copy(wc[:], bytes.Repeat([]byte{0xBB}, 32))
	wc[0] = 0x42 // version byte

	statePb, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		Slot: 0,
		Builders: []*ethpb.Builder{
			{
				WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
				Balance:           1,
			},
		},
	})
	require.NoError(t, err)

	st, ok := statePb.(*BeaconState)
	require.Equal(t, true, ok)

	copied := st.Copy().(*BeaconState)
	require.Equal(t, uint(2), st.sharedFieldReferences[types.Builders].Refs())

	require.NoError(t, copied.AddBuilderFromDeposit(pubkey, wc, 5))
	require.Equal(t, 1, len(st.builders))
	require.Equal(t, 2, len(copied.builders))
	require.Equal(t, uint(1), st.sharedFieldReferences[types.Builders].Refs())
	require.Equal(t, uint(1), copied.sharedFieldReferences[types.Builders].Refs())
}

func TestOnboardBuildersFromPendingDeposits(t *testing.T) {
	t.Run("returns error before gloas", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.OnboardBuildersFromPendingDeposits()
		require.ErrorContains(t, "OnboardBuildersFromPendingDeposits", err)
	})

	t.Run("keeps pending deposits for existing validator", func(t *testing.T) {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		pubkey := sk.PublicKey().Marshal()
		validator := &ethpb.Validator{PublicKey: pubkey}
		builderCreds := builderWithdrawalCredentials(0xAB)
		deposit := &ethpb.PendingDeposit{
			PublicKey:             pubkey,
			WithdrawalCredentials: builderCreds,
			Amount:                10,
			Signature:             make([]byte, fieldparams.BLSSignatureLength),
			Slot:                  0,
		}

		st := newGloasState(t, []*ethpb.Validator{validator}, nil, []*ethpb.PendingDeposit{deposit}, 0)
		require.NoError(t, st.OnboardBuildersFromPendingDeposits())
		require.Equal(t, 1, len(st.pendingDeposits))
		require.Equal(t, 0, len(st.builders))
	})

	t.Run("creates builder for valid builder deposit", func(t *testing.T) {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		builderCreds := builderWithdrawalCredentials(0xCC)
		amount := uint64(42)
		depSlot := primitives.Slot(params.BeaconConfig().SlotsPerEpoch*2 + 1)
		deposit := newPendingDeposit(t, sk, builderCreds, amount, depSlot, true)

		st := newGloasState(t, nil, nil, []*ethpb.PendingDeposit{deposit}, 0)
		require.NoError(t, st.OnboardBuildersFromPendingDeposits())
		require.Equal(t, 0, len(st.pendingDeposits))
		require.Equal(t, 1, len(st.builders))

		builder := st.builders[0]
		require.DeepEqual(t, sk.PublicKey().Marshal(), builder.Pubkey)
		require.DeepEqual(t, builderCreds[12:], builder.ExecutionAddress)
		require.Equal(t, primitives.Gwei(amount), builder.Balance)
		require.Equal(t, slots.ToEpoch(depSlot), builder.DepositEpoch)
	})

	t.Run("increases balance for existing builder", func(t *testing.T) {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		pubkey := sk.PublicKey().Marshal()
		builder := &ethpb.Builder{
			Pubkey:            pubkey,
			Balance:           10,
			WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
		}
		nonBuilderCreds := nonBuilderWithdrawalCredentials()
		deposit := newPendingDeposit(t, sk, nonBuilderCreds, 5, 0, false)

		st := newGloasState(t, nil, []*ethpb.Builder{builder}, []*ethpb.PendingDeposit{deposit}, 0)
		require.NoError(t, st.OnboardBuildersFromPendingDeposits())
		require.Equal(t, 0, len(st.pendingDeposits))
		require.Equal(t, 1, len(st.builders))
		require.Equal(t, primitives.Gwei(15), st.builders[0].Balance)
	})

	t.Run("drops invalid builder deposit", func(t *testing.T) {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		builderCreds := builderWithdrawalCredentials(0xDD)
		deposit := newPendingDeposit(t, sk, builderCreds, 10, 0, false)

		st := newGloasState(t, nil, nil, []*ethpb.PendingDeposit{deposit}, 0)
		require.NoError(t, st.OnboardBuildersFromPendingDeposits())
		require.Equal(t, 0, len(st.pendingDeposits))
		require.Equal(t, 0, len(st.builders))
	})

	t.Run("validator deposit blocks later builder deposit for same pubkey", func(t *testing.T) {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		validatorCreds := nonBuilderWithdrawalCredentials()
		builderCreds := builderWithdrawalCredentials(0xEE)

		depositValidator := newPendingDeposit(t, sk, validatorCreds, 5, 0, true)
		depositBuilder := newPendingDeposit(t, sk, builderCreds, 7, 0, true)

		st := newGloasState(t, nil, nil, []*ethpb.PendingDeposit{depositValidator, depositBuilder}, 0)
		require.NoError(t, st.OnboardBuildersFromPendingDeposits())
		require.Equal(t, 2, len(st.pendingDeposits))
		require.Equal(t, 0, len(st.builders))
	})

	t.Run("drops invalid non-builder deposit", func(t *testing.T) {
		sk, err := bls.RandKey()
		require.NoError(t, err)
		validatorCreds := nonBuilderWithdrawalCredentials()
		deposit := newPendingDeposit(t, sk, validatorCreds, 5, 0, false)

		st := newGloasState(t, nil, nil, []*ethpb.PendingDeposit{deposit}, 0)
		require.NoError(t, st.OnboardBuildersFromPendingDeposits())
		require.Equal(t, 0, len(st.pendingDeposits))
		require.Equal(t, 0, len(st.builders))
	})
}

func newGloasState(
	t *testing.T,
	validators []*ethpb.Validator,
	builders []*ethpb.Builder,
	pendingDeposits []*ethpb.PendingDeposit,
	slot primitives.Slot,
) *BeaconState {
	t.Helper()
	statePb, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		Slot:            slot,
		Validators:      validators,
		Builders:        builders,
		PendingDeposits: pendingDeposits,
	})
	require.NoError(t, err)

	st, ok := statePb.(*BeaconState)
	require.Equal(t, true, ok)
	return st
}

func builderWithdrawalCredentials(addrByte byte) []byte {
	wc := make([]byte, fieldparams.RootLength)
	wc[0] = params.BeaconConfig().BuilderWithdrawalPrefixByte
	for i := 12; i < len(wc); i++ {
		wc[i] = addrByte
	}
	return wc
}

func nonBuilderWithdrawalCredentials() []byte {
	wc := make([]byte, fieldparams.RootLength)
	wc[0] = params.BeaconConfig().BLSWithdrawalPrefixByte
	return wc
}

func newPendingDeposit(
	t *testing.T,
	sk bls.SecretKey,
	withdrawalCredentials []byte,
	amount uint64,
	slot primitives.Slot,
	valid bool,
) *ethpb.PendingDeposit {
	t.Helper()
	signature := make([]byte, fieldparams.BLSSignatureLength)
	if valid {
		signature = signDeposit(t, sk, withdrawalCredentials, amount)
	}
	return &ethpb.PendingDeposit{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: withdrawalCredentials,
		Amount:                amount,
		Signature:             signature,
		Slot:                  slot,
	}
}

func signDeposit(t *testing.T, sk bls.SecretKey, withdrawalCredentials []byte, amount uint64) []byte {
	t.Helper()
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	msg := &ethpb.DepositMessage{
		PublicKey:             sk.PublicKey().Marshal(),
		WithdrawalCredentials: withdrawalCredentials,
		Amount:                amount,
	}
	signingRoot, err := signing.ComputeSigningRoot(msg, domain)
	require.NoError(t, err)
	sig := sk.Sign(signingRoot[:])
	return sig.Marshal()
}
