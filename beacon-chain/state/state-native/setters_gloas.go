package state_native

import (
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// SetPayloadExpectedWithdrawals stores the expected withdrawals for the next payload.
func (b *BeaconState) SetPayloadExpectedWithdrawals(withdrawals []*enginev1.Withdrawal) error {
	if b.version < version.Gloas {
		return errNotSupported("SetPayloadExpectedWithdrawals", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.payloadExpectedWithdrawals = withdrawals
	b.markFieldAsDirty(types.PayloadExpectedWithdrawals)

	return nil
}

// RotateBuilderPendingPayments rotates the queue by dropping slots per epoch payments from the
// front and appending slots per epoch empty payments to the end.
// This implements: state.builder_pending_payments = state.builder_pending_payments[SLOTS_PER_EPOCH:] + [BuilderPendingPayment() for _ in range(SLOTS_PER_EPOCH)]
func (b *BeaconState) RotateBuilderPendingPayments() error {
	if b.version < version.Gloas {
		return errNotSupported("RotateBuilderPendingPayments", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	copy(b.builderPendingPayments[:slotsPerEpoch], b.builderPendingPayments[slotsPerEpoch:2*slotsPerEpoch])

	for i := slotsPerEpoch; i < primitives.Slot(len(b.builderPendingPayments)); i++ {
		b.builderPendingPayments[i] = emptyBuilderPendingPayment
	}

	b.markFieldAsDirty(types.BuilderPendingPayments)
	b.rebuildTrie[types.BuilderPendingPayments] = true
	return nil
}

// emptyBuilderPendingPayment is a shared zero-value payment used to clear entries.
var emptyBuilderPendingPayment = &ethpb.BuilderPendingPayment{
	Withdrawal: &ethpb.BuilderPendingWithdrawal{
		FeeRecipient: make([]byte, 20),
	},
}

// AppendBuilderPendingWithdrawals appends builder pending withdrawals to the beacon state.
// If the withdrawals slice is shared, it copies the slice first to preserve references.
func (b *BeaconState) AppendBuilderPendingWithdrawals(withdrawals []*ethpb.BuilderPendingWithdrawal) error {
	if b.version < version.Gloas {
		return errNotSupported("AppendBuilderPendingWithdrawals", b.version)
	}

	if len(withdrawals) == 0 {
		return nil
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	pendingWithdrawals := b.builderPendingWithdrawals
	if b.sharedFieldReferences[types.BuilderPendingWithdrawals].Refs() > 1 {
		pendingWithdrawals = make([]*ethpb.BuilderPendingWithdrawal, 0, len(b.builderPendingWithdrawals)+len(withdrawals))
		pendingWithdrawals = append(pendingWithdrawals, b.builderPendingWithdrawals...)
		b.sharedFieldReferences[types.BuilderPendingWithdrawals].MinusRef()
		b.sharedFieldReferences[types.BuilderPendingWithdrawals] = stateutil.NewRef(1)
	}

	b.builderPendingWithdrawals = append(pendingWithdrawals, withdrawals...)
	b.markFieldAsDirty(types.BuilderPendingWithdrawals)
	return nil
}

// DequeueBuilderPendingWithdrawals removes processed builder withdrawals from the front of the queue.
func (b *BeaconState) DequeueBuilderPendingWithdrawals(n uint64) error {
	if b.version < version.Gloas {
		return errNotSupported("DequeueBuilderPendingWithdrawals", b.version)
	}

	if n > uint64(len(b.builderPendingWithdrawals)) {
		return errors.New("cannot dequeue more builder withdrawals than are in the queue")
	}

	if n == 0 {
		return nil
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if b.sharedFieldReferences[types.BuilderPendingWithdrawals].Refs() > 1 {
		withdrawals := make([]*ethpb.BuilderPendingWithdrawal, len(b.builderPendingWithdrawals))
		copy(withdrawals, b.builderPendingWithdrawals)
		b.builderPendingWithdrawals = withdrawals
		b.sharedFieldReferences[types.BuilderPendingWithdrawals].MinusRef()
		b.sharedFieldReferences[types.BuilderPendingWithdrawals] = stateutil.NewRef(1)
	}

	b.builderPendingWithdrawals = b.builderPendingWithdrawals[n:]
	b.markFieldAsDirty(types.BuilderPendingWithdrawals)
	b.rebuildTrie[types.BuilderPendingWithdrawals] = true

	return nil
}

// SetExecutionPayloadBid sets the latest execution payload bid in the state.
func (b *BeaconState) SetExecutionPayloadBid(h interfaces.ROExecutionPayloadBid) error {
	if b.version < version.Gloas {
		return errNotSupported("SetExecutionPayloadBid", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	parentBlockHash := h.ParentBlockHash()
	parentBlockRoot := h.ParentBlockRoot()
	blockHash := h.BlockHash()
	randao := h.PrevRandao()
	blobKzgCommitmentsRoot := h.BlobKzgCommitmentsRoot()
	feeRecipient := h.FeeRecipient()
	b.latestExecutionPayloadBid = &ethpb.ExecutionPayloadBid{
		ParentBlockHash:        parentBlockHash[:],
		ParentBlockRoot:        parentBlockRoot[:],
		BlockHash:              blockHash[:],
		PrevRandao:             randao[:],
		GasLimit:               h.GasLimit(),
		BuilderIndex:           h.BuilderIndex(),
		Slot:                   h.Slot(),
		Value:                  h.Value(),
		ExecutionPayment:       h.ExecutionPayment(),
		BlobKzgCommitmentsRoot: blobKzgCommitmentsRoot[:],
		FeeRecipient:           feeRecipient[:],
	}
	b.markFieldAsDirty(types.LatestExecutionPayloadBid)

	return nil
}

// ClearBuilderPendingPayment clears a builder pending payment at the specified index.
func (b *BeaconState) ClearBuilderPendingPayment(index primitives.Slot) error {
	if b.version < version.Gloas {
		return errNotSupported("ClearBuilderPendingPayment", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if uint64(index) >= uint64(len(b.builderPendingPayments)) {
		return fmt.Errorf("builder pending payments index %d out of range (len=%d)", index, len(b.builderPendingPayments))
	}

	b.builderPendingPayments[index] = emptyBuilderPendingPayment

	b.markFieldAsDirty(types.BuilderPendingPayments)
	return nil
}

// SetBuilderPendingPayment sets a builder pending payment at the specified index.
func (b *BeaconState) SetBuilderPendingPayment(index primitives.Slot, payment *ethpb.BuilderPendingPayment) error {
	if b.version < version.Gloas {
		return errNotSupported("SetBuilderPendingPayment", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if uint64(index) >= uint64(len(b.builderPendingPayments)) {
		return fmt.Errorf("builder pending payments index %d out of range (len=%d)", index, len(b.builderPendingPayments))
	}

	b.builderPendingPayments[index] = ethpb.CopyBuilderPendingPayment(payment)

	b.markFieldAsDirty(types.BuilderPendingPayments)
	return nil
}

// UpdateExecutionPayloadAvailabilityAtIndex updates the execution payload availability bit at a specific index.
func (b *BeaconState) UpdateExecutionPayloadAvailabilityAtIndex(idx uint64, val byte) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	byteIndex := idx / 8
	bitIndex := idx % 8

	if byteIndex >= uint64(len(b.executionPayloadAvailability)) {
		return fmt.Errorf("bit index %d (byte index %d) out of range for execution payload availability length %d", idx, byteIndex, len(b.executionPayloadAvailability))
	}

	if val != 0 {
		b.executionPayloadAvailability[byteIndex] |= (1 << bitIndex)
	} else {
		b.executionPayloadAvailability[byteIndex] &^= (1 << bitIndex)
	}

	b.markFieldAsDirty(types.ExecutionPayloadAvailability)
	return nil
}

// SetNextWithdrawalBuilderIndex sets the next builder index for the withdrawals sweep.
func (b *BeaconState) SetNextWithdrawalBuilderIndex(index primitives.BuilderIndex) error {
	if b.version < version.Gloas {
		return errNotSupported("SetNextWithdrawalBuilderIndex", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.nextWithdrawalBuilderIndex = index
	b.markFieldAsDirty(types.NextWithdrawalBuilderIndex)
	return nil
}

// DecreaseBuilderBalance decreases the builder's balance by amount (saturating at 0).
func (b *BeaconState) DecreaseBuilderBalance(builderIndex primitives.BuilderIndex, amount uint64) error {
	if b.version < version.Gloas {
		return errNotSupported("DecreaseBuilderBalance", b.version)
	}
	if amount == 0 {
		return nil
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	idx := uint64(builderIndex)
	if idx >= uint64(len(b.builders)) {
		return fmt.Errorf("builder index %d out of range (len=%d)", builderIndex, len(b.builders))
	}

	// Copy-on-write for shared builders registry.
	if b.sharedFieldReferences[types.Builders].Refs() > 1 {
		builders := make([]*ethpb.Builder, len(b.builders))
		copy(builders, b.builders)
		b.builders = builders
		b.sharedFieldReferences[types.Builders].MinusRef()
		b.sharedFieldReferences[types.Builders] = stateutil.NewRef(1)

		// Ensure we don't mutate a shared builder pointer.
		if b.builders[idx] != nil {
			b.builders[idx] = ethpb.CopyBuilder(b.builders[idx])
		}
	}

	builder := b.builders[idx]
	if builder == nil {
		return fmt.Errorf("builder at index %d is nil", builderIndex)
	}

	bal := uint64(builder.Balance)
	if amount >= bal {
		builder.Balance = 0
	} else {
		builder.Balance = primitives.Gwei(bal - amount)
	}

	b.markFieldAsDirty(types.Builders)
	b.addDirtyIndices(types.Builders, []uint64{idx})
	return nil
}
