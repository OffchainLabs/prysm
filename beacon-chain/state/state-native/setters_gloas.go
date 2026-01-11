package state_native

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
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

// QueueBuilderPayment implements the builder payment queuing logic for Gloas.
// Spec v1.7.0-alpha.0 (pseudocode):
// payment = state.builder_pending_payments[SLOTS_PER_EPOCH + state.slot % SLOTS_PER_EPOCH]
// amount = payment.withdrawal.amount
// if amount > 0:
//
//	state.builder_pending_withdrawals.append(payment.withdrawal)
//
// state.builder_pending_payments[SLOTS_PER_EPOCH + state.slot % SLOTS_PER_EPOCH] = BuilderPendingPayment()
func (b *BeaconState) QueueBuilderPayment() error {
	if b.version < version.Gloas {
		return errNotSupported("QueueBuilderPayment", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	slot := b.slot
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	paymentIndex := slotsPerEpoch + (slot % slotsPerEpoch)
	if uint64(paymentIndex) >= uint64(len(b.builderPendingPayments)) {
		return fmt.Errorf("builder pending payments index %d out of range (len=%d)", paymentIndex, len(b.builderPendingPayments))
	}

	payment := b.builderPendingPayments[paymentIndex]
	if payment != nil && payment.Withdrawal != nil && payment.Withdrawal.Amount > 0 {
		b.builderPendingWithdrawals = append(b.builderPendingWithdrawals, ethpb.CopyBuilderPendingWithdrawal(payment.Withdrawal))
		b.markFieldAsDirty(types.BuilderPendingWithdrawals)
	}

	b.builderPendingPayments[paymentIndex] = emptyBuilderPendingPayment
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

// SetLatestBlockHash sets the latest execution block hash.
func (b *BeaconState) SetLatestBlockHash(hash [32]byte) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.latestBlockHash = hash[:]
	b.markFieldAsDirty(types.LatestBlockHash)
	return nil
}

// SetExecutionPayloadAvailability sets the execution payload availability bit for a specific slot.
func (b *BeaconState) SetExecutionPayloadAvailability(index primitives.Slot, available bool) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	bitIndex := index % params.BeaconConfig().SlotsPerHistoricalRoot
	byteIndex := bitIndex / 8
	bitPosition := bitIndex % 8

	// Set or clear the bit
	if available {
		b.executionPayloadAvailability[byteIndex] |= 1 << bitPosition
	} else {
		b.executionPayloadAvailability[byteIndex] &^= 1 << bitPosition
	}

	b.markFieldAsDirty(types.ExecutionPayloadAvailability)
	return nil
}

// IncreaseBuilderBalance increases the balance of the builder at the given index.
func (b *BeaconState) IncreaseBuilderBalance(index primitives.BuilderIndex, amount uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.builders == nil || uint64(index) >= uint64(len(b.builders)) {
		return fmt.Errorf("builder index %d out of bounds", index)
	}
	if b.builders[index] == nil {
		return fmt.Errorf("builder at index %d is nil", index)
	}

	builder := ethpb.CopyBuilder(b.builders[index])
	builder.Balance += primitives.Gwei(amount)
	b.builders[index] = builder

	b.markFieldAsDirty(types.Builders)
	return nil
}

// AddBuilderFromDeposit creates or replaces a builder entry derived from a deposit.
func (b *BeaconState) AddBuilderFromDeposit(pubkey [fieldparams.BLSPubkeyLength]byte, withdrawalCredentials [fieldparams.RootLength]byte, amount uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	currentEpoch := slots.ToEpoch(b.slot)
	index := b.builderInsertionIndex(currentEpoch)

	builder := &ethpb.Builder{
		Pubkey:            bytesutil.SafeCopyBytes(pubkey[:]),
		Version:           []byte{withdrawalCredentials[0]},
		ExecutionAddress:  bytesutil.SafeCopyBytes(withdrawalCredentials[12:]),
		Balance:           primitives.Gwei(amount),
		DepositEpoch:      currentEpoch,
		WithdrawableEpoch: params.BeaconConfig().FarFutureEpoch,
	}

	if index < primitives.BuilderIndex(len(b.builders)) {
		b.builders[index] = builder
	} else {
		gap := index - primitives.BuilderIndex(len(b.builders)) + 1
		b.builders = append(b.builders, make([]*ethpb.Builder, gap)...)
		b.builders[index] = builder
	}

	b.markFieldAsDirty(types.Builders)
	return nil
}

func (b *BeaconState) builderInsertionIndex(currentEpoch primitives.Epoch) primitives.BuilderIndex {
	for i, builder := range b.builders {
		if builder.WithdrawableEpoch <= currentEpoch && builder.Balance == 0 {
			return primitives.BuilderIndex(i)
		}
	}
	return primitives.BuilderIndex(len(b.builders))
}
