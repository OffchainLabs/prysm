package state_native

import (
	"bytes"
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
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

// LatestExecutionPayloadBid returns the cached latest execution payload bid for Gloas.
func (b *BeaconState) LatestExecutionPayloadBid() (interfaces.ROExecutionPayloadBid, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.latestExecutionPayloadBid == nil {
		return nil, nil
	}

	return blocks.WrappedROExecutionPayloadBid(b.latestExecutionPayloadBid.Copy())
}

// WithdrawalsMatchPayloadExpected returns true if the given withdrawals root matches the state's
// payload_expected_withdrawals root.
func (b *BeaconState) WithdrawalsMatchPayloadExpected(withdrawals []*enginev1.Withdrawal) (bool, error) {
	if b.version < version.Gloas {
		return false, errNotSupported("WithdrawalsMatchPayloadExpected", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	cfg := params.BeaconConfig()

	withdrawalsRoot, err := ssz.WithdrawalSliceRoot(withdrawals, cfg.MaxWithdrawalsPerPayload)
	if err != nil {
		return false, fmt.Errorf("could not compute withdrawals root: %w", err)
	}

	expected := b.payloadExpectedWithdrawals
	if expected == nil {
		expected = []*enginev1.Withdrawal{}
	}
	expectedRoot, err := ssz.WithdrawalSliceRoot(expected, cfg.MaxWithdrawalsPerPayload)
	if err != nil {
		return false, fmt.Errorf("could not compute expected withdrawals root: %w", err)
	}

	return withdrawalsRoot == expectedRoot, nil
}

// Builder returns the builder at the given index.
func (b *BeaconState) Builder(index primitives.BuilderIndex) (*ethpb.Builder, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.builders == nil {
		return nil, nil
	}
	if uint64(index) >= uint64(len(b.builders)) {
		return nil, fmt.Errorf("builder index %d out of bounds", index)
	}
	if b.builders[index] == nil {
		return nil, nil
	}

	return ethpb.CopyBuilder(b.builders[index]), nil
}

// BuilderIndexByPubkey returns the builder index for the given pubkey, if present.
func (b *BeaconState) BuilderIndexByPubkey(pubkey [fieldparams.BLSPubkeyLength]byte) (primitives.BuilderIndex, bool) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	for i, builder := range b.builders {
		if builder == nil {
			continue
		}
		if bytes.Equal(builder.Pubkey, pubkey[:]) {
			return primitives.BuilderIndex(i), true
		}
	}
	return 0, false
}
