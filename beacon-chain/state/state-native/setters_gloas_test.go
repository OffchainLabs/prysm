package state_native

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

type testExecutionPayloadBid struct {
	parentBlockHash        [32]byte
	parentBlockRoot        [32]byte
	blockHash              [32]byte
	prevRandao             [32]byte
	blobKzgCommitmentsRoot [32]byte
	feeRecipient           [20]byte
	gasLimit               uint64
	builderIndex           primitives.BuilderIndex
	slot                   primitives.Slot
	value                  primitives.Gwei
	executionPayment       primitives.Gwei
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
func (t testExecutionPayloadBid) BlobKzgCommitmentsRoot() [32]byte { return t.blobKzgCommitmentsRoot }
func (t testExecutionPayloadBid) FeeRecipient() [20]byte           { return t.feeRecipient }
func (t testExecutionPayloadBid) IsNil() bool                      { return false }

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
			blobRoot        = [32]byte(bytes.Repeat([]byte{0x22}, 32))
			feeRecipient    [20]byte
		)
		copy(feeRecipient[:], bytes.Repeat([]byte{0x33}, len(feeRecipient)))
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
		}
		bid := testExecutionPayloadBid{
			parentBlockHash:        parentBlockHash,
			parentBlockRoot:        parentBlockRoot,
			blockHash:              blockHash,
			prevRandao:             prevRandao,
			blobKzgCommitmentsRoot: blobRoot,
			feeRecipient:           feeRecipient,
			gasLimit:               123,
			builderIndex:           7,
			slot:                   9,
			value:                  11,
			executionPayment:       22,
		}

		require.NoError(t, st.SetExecutionPayloadBid(bid))

		require.NotNil(t, st.latestExecutionPayloadBid)
		require.DeepEqual(t, parentBlockHash[:], st.latestExecutionPayloadBid.ParentBlockHash)
		require.DeepEqual(t, parentBlockRoot[:], st.latestExecutionPayloadBid.ParentBlockRoot)
		require.DeepEqual(t, blockHash[:], st.latestExecutionPayloadBid.BlockHash)
		require.DeepEqual(t, prevRandao[:], st.latestExecutionPayloadBid.PrevRandao)
		require.DeepEqual(t, blobRoot[:], st.latestExecutionPayloadBid.BlobKzgCommitmentsRoot)
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

	t.Run("rejects nil input", func(t *testing.T) {
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.PayloadExpectedWithdrawals: stateutil.NewRef(1),
			},
		}

		err := st.SetPayloadExpectedWithdrawals(nil)
		require.ErrorContains(t, "cannot set nil payload expected withdrawals", err)
		require.Equal(t, false, st.dirtyFields[types.PayloadExpectedWithdrawals])
	})

	t.Run("sets and marks dirty", func(t *testing.T) {
		oldRef := stateutil.NewRef(2)
		st := &BeaconState{
			version:     version.Gloas,
			dirtyFields: make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.PayloadExpectedWithdrawals: oldRef,
			},
			payloadExpectedWithdrawals: []*enginev1.Withdrawal{{Index: 1}, {Index: 2}},
		}

		withdrawals := []*enginev1.Withdrawal{{Index: 3}}
		require.NoError(t, st.SetPayloadExpectedWithdrawals(withdrawals))

		require.DeepEqual(t, withdrawals, st.payloadExpectedWithdrawals)
		require.Equal(t, true, st.dirtyFields[types.PayloadExpectedWithdrawals])

		require.Equal(t, uint(1), oldRef.Refs())
		require.Equal(t, uint(1), st.sharedFieldReferences[types.PayloadExpectedWithdrawals].Refs())
		require.Equal(t, false, st.sharedFieldReferences[types.PayloadExpectedWithdrawals] == oldRef)
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

func TestDecreaseBuilderBalance(t *testing.T) {
	t.Run("previous fork returns expected error", func(t *testing.T) {
		st := &BeaconState{version: version.Fulu}
		err := st.DecreaseBuilderBalance(0, 1)
		require.ErrorContains(t, "DecreaseBuilderBalance", err)
	})

	t.Run("decreases and saturates", func(t *testing.T) {
		st := &BeaconState{
			version:      version.Gloas,
			dirtyFields:  make(map[types.FieldIndex]bool),
			dirtyIndices: make(map[types.FieldIndex][]uint64),
			rebuildTrie:  make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.Builders: stateutil.NewRef(1),
			},
			builders: []*ethpb.Builder{
				{Balance: 10},
			},
		}

		require.NoError(t, st.DecreaseBuilderBalance(0, 3))
		require.Equal(t, uint64(7), uint64(st.builders[0].Balance))
		require.Equal(t, true, st.dirtyFields[types.Builders])

		require.NoError(t, st.DecreaseBuilderBalance(0, 100))
		require.Equal(t, uint64(0), uint64(st.builders[0].Balance))
	})

	t.Run("copy-on-write preserves shared state", func(t *testing.T) {
		sharedRef := stateutil.NewRef(2)
		sharedBuilders := []*ethpb.Builder{
			{Balance: 10},
		}

		st1 := &BeaconState{
			version:      version.Gloas,
			dirtyFields:  make(map[types.FieldIndex]bool),
			dirtyIndices: make(map[types.FieldIndex][]uint64),
			rebuildTrie:  make(map[types.FieldIndex]bool),
			sharedFieldReferences: map[types.FieldIndex]*stateutil.Reference{
				types.Builders: sharedRef,
			},
			builders: sharedBuilders,
		}
		st2 := &BeaconState{
			builders: sharedBuilders,
		}

		require.NoError(t, st1.DecreaseBuilderBalance(0, 3))
		require.Equal(t, uint64(7), uint64(st1.builders[0].Balance))
		require.Equal(t, uint64(10), uint64(st2.builders[0].Balance))
	})
}

func newGloasStateWithAvailability(t *testing.T, availability []byte) *BeaconState {
	t.Helper()

	st, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		ExecutionPayloadAvailability: availability,
	})
	require.NoError(t, err)

	return st.(*BeaconState)
}
