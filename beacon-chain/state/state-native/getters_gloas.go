package state_native

import (
	"bytes"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// LatestBlockHash returns the hash of the latest execution block.
func (b *BeaconState) LatestBlockHash() ([32]byte, error) {
	if b.version < version.Gloas {
		return [32]byte{}, errNotSupported("LatestBlockHash", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.latestBlockHash == nil {
		return [32]byte{}, nil
	}

	return [32]byte(b.latestBlockHash), nil
}

// IsAttestationSameSlot checks if the attestation is for the same slot as the block root in the state.
// Spec v1.7.0-alpha pseudocode:
//
//	is_attestation_same_slot(state, data):
//	    if data.slot == 0:
//	        return True
//
//	    blockroot = data.beacon_block_root
//	    slot_blockroot = get_block_root_at_slot(state, data.slot)
//	    prev_blockroot = get_block_root_at_slot(state, Slot(data.slot - 1))
//
//	    return blockroot == slot_blockroot and blockroot != prev_blockroot
func (b *BeaconState) IsAttestationSameSlot(blockRoot [32]byte, slot primitives.Slot) (bool, error) {
	if b.version < version.Gloas {
		return false, errNotSupported("IsAttestationSameSlot", b.version)
	}
	if slot == 0 {
		return true, nil
	}

	blockRootAtSlot, err := helpers.BlockRootAtSlot(b, slot)
	if err != nil {
		return false, errors.Wrapf(err, "block root at slot %d", slot)
	}
	matchingBlockRoot := bytes.Equal(blockRoot[:], blockRootAtSlot)

	blockRootAtPrevSlot, err := helpers.BlockRootAtSlot(b, slot-1)
	if err != nil {
		return false, errors.Wrapf(err, "block root at slot %d", slot-1)
	}
	matchingPrevBlockRoot := bytes.Equal(blockRoot[:], blockRootAtPrevSlot)

	return matchingBlockRoot && !matchingPrevBlockRoot, nil
}

// BuilderPubkey returns the builder pubkey at the provided index.
func (b *BeaconState) BuilderPubkey(builderIndex primitives.BuilderIndex) ([fieldparams.BLSPubkeyLength]byte, error) {
	if b.version < version.Gloas {
		return [fieldparams.BLSPubkeyLength]byte{}, errNotSupported("BuilderPubkey", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	builder, err := b.builderAtIndex(builderIndex)
	if err != nil {
		return [fieldparams.BLSPubkeyLength]byte{}, err
	}

	var pk [fieldparams.BLSPubkeyLength]byte
	copy(pk[:], builder.Pubkey)
	return pk, nil
}

// IsActiveBuilder returns true if the builder placement is finalized and it has not initiated exit.
// Spec v1.7.0-alpha.0 (pseudocode):
// def is_active_builder(state: BeaconState, builder_index: BuilderIndex) -> bool:
//
//	builder = state.builders[builder_index]
//	return (
//	    builder.deposit_epoch < state.finalized_checkpoint.epoch
//	    and builder.withdrawable_epoch == FAR_FUTURE_EPOCH
//	)
func (b *BeaconState) IsActiveBuilder(builderIndex primitives.BuilderIndex) (bool, error) {
	if b.version < version.Gloas {
		return false, errNotSupported("IsActiveBuilder", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	builder, err := b.builderAtIndex(builderIndex)
	if err != nil {
		return false, err
	}

	finalizedEpoch := b.finalizedCheckpoint.Epoch
	return builder.DepositEpoch < finalizedEpoch && builder.WithdrawableEpoch == params.BeaconConfig().FarFutureEpoch, nil
}

// CanBuilderCoverBid returns true if the builder has enough balance to cover the given bid amount.
// Spec v1.7.0-alpha.0 (pseudocode):
// def can_builder_cover_bid(state: BeaconState, builder_index: BuilderIndex, bid_amount: Gwei) -> bool:
//
//	builder_balance = state.builders[builder_index].balance
//	pending_withdrawals_amount = get_pending_balance_to_withdraw_for_builder(state, builder_index)
//	min_balance = MIN_DEPOSIT_AMOUNT + pending_withdrawals_amount
//	if builder_balance < min_balance:
//	    return False
//	return builder_balance - min_balance >= bid_amount
func (b *BeaconState) CanBuilderCoverBid(builderIndex primitives.BuilderIndex, bidAmount primitives.Gwei) (bool, error) {
	if b.version < version.Gloas {
		return false, errNotSupported("CanBuilderCoverBid", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	builder, err := b.builderAtIndex(builderIndex)
	if err != nil {
		return false, err
	}

	pendingBalanceToWithdraw := b.builderPendingBalanceToWithdraw(builderIndex)
	minBalance := params.BeaconConfig().MinDepositAmount + pendingBalanceToWithdraw

	balance := uint64(builder.Balance)
	if balance < minBalance {
		return false, nil
	}

	return balance-minBalance >= uint64(bidAmount), nil
}

// builderAtIndex intentionally returns the underlying pointer without copying.
func (b *BeaconState) builderAtIndex(builderIndex primitives.BuilderIndex) (*ethpb.Builder, error) {
	idx := uint64(builderIndex)
	if idx >= uint64(len(b.builders)) {
		return nil, fmt.Errorf("builder index %d out of range (len=%d)", builderIndex, len(b.builders))
	}

	builder := b.builders[idx]
	if builder == nil {
		return nil, fmt.Errorf("builder at index %d is nil", builderIndex)
	}
	return builder, nil
}

// builderPendingBalanceToWithdraw mirrors get_pending_balance_to_withdraw_for_builder in the spec,
// summing both pending withdrawals and pending payments for a builder.
func (b *BeaconState) builderPendingBalanceToWithdraw(builderIndex primitives.BuilderIndex) uint64 {
	var total uint64
	for _, withdrawal := range b.builderPendingWithdrawals {
		if withdrawal.BuilderIndex == builderIndex {
			total += uint64(withdrawal.Amount)
		}
	}
	for _, payment := range b.builderPendingPayments {
		if payment.Withdrawal.BuilderIndex == builderIndex {
			total += uint64(payment.Withdrawal.Amount)
		}
	}
	return total
}

// BuilderPendingPayments returns a copy of the builder pending payments.
func (b *BeaconState) BuilderPendingPayments() ([]*ethpb.BuilderPendingPayment, error) {
	if b.version < version.Gloas {
		return nil, errNotSupported("BuilderPendingPayments", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.builderPendingPaymentsVal(), nil
}

// BuilderPendingPayment returns the builder pending payment for the given index.
func (b *BeaconState) BuilderPendingPayment(index uint64) (*ethpb.BuilderPendingPayment, error) {
	if b.version < version.Gloas {
		return nil, errNotSupported("BuilderPendingPayment", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if index >= uint64(len(b.builderPendingPayments)) {
		return nil, fmt.Errorf("builder pending payment index %d out of range (len=%d)", index, len(b.builderPendingPayments))
	}
	return ethpb.CopyBuilderPendingPayment(b.builderPendingPayments[index]), nil
}

// ExecutionPayloadAvailability returns the execution payload availability bit for the given slot.
func (b *BeaconState) ExecutionPayloadAvailability(slot primitives.Slot) (uint64, error) {
	if b.version < version.Gloas {
		return 0, errNotSupported("ExecutionPayloadAvailability", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	slotIndex := slot % params.BeaconConfig().SlotsPerHistoricalRoot
	byteIndex := slotIndex / 8
	bitIndex := slotIndex % 8

	bit := (b.executionPayloadAvailability[byteIndex] >> bitIndex) & 1

	return uint64(bit), nil
}
