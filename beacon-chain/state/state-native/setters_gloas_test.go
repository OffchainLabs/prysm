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

func newGloasStateWithAvailability(t *testing.T, availability []byte) *BeaconState {
	t.Helper()

	st, err := InitializeFromProtoUnsafeGloas(&ethpb.BeaconStateGloas{
		ExecutionPayloadAvailability: availability,
	})
	require.NoError(t, err)

	return st.(*BeaconState)
}
