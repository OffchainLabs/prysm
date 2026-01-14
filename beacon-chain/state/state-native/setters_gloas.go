package state_native

import (
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// RotateBuilderPendingPayments rotates the queue by dropping slots per epoch payments from the
// front and appending slots per epoch empty payments to the end.
// This implements: state.builder_pending_payments = state.builder_pending_payments[SLOTS_PER_EPOCH:] + [BuilderPendingPayment() for _ in range(SLOTS_PER_EPOCH)]
func (b *BeaconState) RotateBuilderPendingPayments() error {
	if b.version < version.Gloas {
		return errNotSupported("RotateBuilderPendingPayments", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	oldPayments := b.builderPendingPayments
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	newPayments := make([]*ethpb.BuilderPendingPayment, 2*slotsPerEpoch)

	copy(newPayments, oldPayments[slotsPerEpoch:])

	for i := uint64(len(oldPayments)) - slotsPerEpoch; i < uint64(len(newPayments)); i++ {
		newPayments[i] = emptyPayment()
	}

	b.builderPendingPayments = newPayments
	b.markFieldAsDirty(types.BuilderPendingPayments)
	b.rebuildTrie[types.BuilderPendingPayments] = true
	return nil
}

// AppendBuilderPendingWithdrawal appends a builder pending withdrawal to the beacon state.
// If the withdrawals slice is shared, it copies the slice first to preserve references.
func (b *BeaconState) AppendBuilderPendingWithdrawal(withdrawal *ethpb.BuilderPendingWithdrawal) error {
	if b.version < version.Gloas {
		return errNotSupported("AppendBuilderPendingWithdrawal", b.version)
	}
	if withdrawal == nil {
		return errors.New("cannot append nil builder pending withdrawal")
	}

	if b.version < version.Gloas {
		return errNotSupported("AppendBuilderPendingWithdrawal", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	withdrawals := b.builderPendingWithdrawals
	if b.sharedFieldReferences[types.BuilderPendingWithdrawals].Refs() > 1 {
		withdrawals = make([]*ethpb.BuilderPendingWithdrawal, len(b.builderPendingWithdrawals), len(b.builderPendingWithdrawals)+1)
		copy(withdrawals, b.builderPendingWithdrawals)
		b.sharedFieldReferences[types.BuilderPendingWithdrawals].MinusRef()
		b.sharedFieldReferences[types.BuilderPendingWithdrawals] = stateutil.NewRef(1)
	}

	b.builderPendingWithdrawals = append(withdrawals, withdrawal)
	b.markFieldAsDirty(types.BuilderPendingWithdrawals)
	return nil
}

func emptyPayment() *ethpb.BuilderPendingPayment {
	return &ethpb.BuilderPendingPayment{
		Weight: 0,
		Withdrawal: &ethpb.BuilderPendingWithdrawal{
			FeeRecipient: make([]byte, 20),
			Amount:       0,
			BuilderIndex: 0,
		},
	}
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
