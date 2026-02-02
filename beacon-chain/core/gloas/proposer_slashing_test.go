package gloas

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestRemoveBuilderPendingPayment_CurrentEpoch(t *testing.T) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	stateSlot := slotsPerEpoch*2 + 1
	headerSlot := slotsPerEpoch * 2

	st := newGloasStateWithPayments(t, stateSlot)
	paymentIndex := int(slotsPerEpoch + headerSlot%slotsPerEpoch)

	setPendingPayment(t, st, paymentIndex, 123)

	err := RemoveBuilderPendingPayment(st, &eth.BeaconBlockHeader{Slot: headerSlot})
	require.NoError(t, err)

	got := getPendingPayment(t, st, paymentIndex)
	require.NotNil(t, got.Withdrawal)
	require.DeepEqual(t, make([]byte, 20), got.Withdrawal.FeeRecipient)
	require.Equal(t, uint64(0), uint64(got.Withdrawal.Amount))
}

func TestRemoveBuilderPendingPayment_PreviousEpoch(t *testing.T) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	stateSlot := slotsPerEpoch*2 + 1
	headerSlot := slotsPerEpoch + 7

	st := newGloasStateWithPayments(t, stateSlot)
	paymentIndex := int(headerSlot % slotsPerEpoch)

	setPendingPayment(t, st, paymentIndex, 456)

	err := RemoveBuilderPendingPayment(st, &eth.BeaconBlockHeader{Slot: headerSlot})
	require.NoError(t, err)

	got := getPendingPayment(t, st, paymentIndex)
	require.NotNil(t, got.Withdrawal)
	require.DeepEqual(t, make([]byte, 20), got.Withdrawal.FeeRecipient)
	require.Equal(t, uint64(0), uint64(got.Withdrawal.Amount))
}

func TestRemoveBuilderPendingPayment_OlderThanTwoEpoch(t *testing.T) {
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	stateSlot := slotsPerEpoch*4 + 1 // current epoch far ahead
	headerSlot := slotsPerEpoch * 2  // two epochs behind

	st := newGloasStateWithPayments(t, stateSlot)
	paymentIndex := int(headerSlot % slotsPerEpoch)

	original := getPendingPayment(t, st, paymentIndex)

	err := RemoveBuilderPendingPayment(st, &eth.BeaconBlockHeader{Slot: headerSlot})
	require.NoError(t, err)

	after := getPendingPayment(t, st, paymentIndex)
	require.DeepEqual(t, original.Withdrawal.FeeRecipient, after.Withdrawal.FeeRecipient)
	require.Equal(t, original.Withdrawal.Amount, after.Withdrawal.Amount)
}

func newGloasStateWithPayments(t *testing.T, slot primitives.Slot) state.BeaconState {
	t.Helper()

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	paymentCount := int(slotsPerEpoch * 2)
	payments := make([]*eth.BuilderPendingPayment, paymentCount)
	for i := range payments {
		payments[i] = &eth.BuilderPendingPayment{
			Withdrawal: &eth.BuilderPendingWithdrawal{
				FeeRecipient: bytes.Repeat([]byte{0x01}, 20),
				Amount:       1,
			},
		}
	}

	st, err := state_native.InitializeFromProtoUnsafeGloas(&eth.BeaconStateGloas{
		Slot:                   slot,
		BuilderPendingPayments: payments,
	})
	require.NoError(t, err)
	return st
}

func setPendingPayment(t *testing.T, st state.BeaconState, index int, amount uint64) {
	t.Helper()

	payment := &eth.BuilderPendingPayment{
		Withdrawal: &eth.BuilderPendingWithdrawal{
			FeeRecipient: bytes.Repeat([]byte{0x02}, 20),
			Amount:       primitives.Gwei(amount),
		},
	}
	require.NoError(t, st.SetBuilderPendingPayment(primitives.Slot(index), payment))
}

func getPendingPayment(t *testing.T, st state.BeaconState, index int) *eth.BuilderPendingPayment {
	t.Helper()

	stateProto := st.ToProtoUnsafe().(*eth.BeaconStateGloas)

	return stateProto.BuilderPendingPayments[index]
}
