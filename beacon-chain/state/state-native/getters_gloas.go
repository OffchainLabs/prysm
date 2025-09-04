package state_native

import (
	"fmt"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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

// executionPayloadAvailabilityVal returns a copy of the execution payload availability.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) executionPayloadAvailabilityVal() []byte {
	if b.executionPayloadAvailability == nil {
		return nil
	}

	availability := make([]byte, len(b.executionPayloadAvailability))
	copy(availability, b.executionPayloadAvailability)

	return availability
}

// BuilderPendingPaymentsNoCopy returns the builder pending payments without copying.
func (b *BeaconState) BuilderPendingPaymentsNoCopy() ([]*ethpb.BuilderPendingPayment, error) {
	if b.version < version.Gloas {
		return nil, errNotSupported("BuilderPendingPaymentsNoCopy", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.builderPendingPayments, nil
}

// builderPendingPaymentsVal returns a copy of the builder pending payments.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) builderPendingPaymentsVal() []*ethpb.BuilderPendingPayment {
	if b.builderPendingPayments == nil {
		return nil
	}

	payments := make([]*ethpb.BuilderPendingPayment, len(b.builderPendingPayments))
	for i, payment := range b.builderPendingPayments {
		payments[i] = payment.Copy()
	}

	return payments
}

// builderPendingWithdrawalsVal returns a copy of the builder pending withdrawals.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) builderPendingWithdrawalsVal() []*ethpb.BuilderPendingWithdrawal {
	if b.builderPendingWithdrawals == nil {
		return nil
	}

	withdrawals := make([]*ethpb.BuilderPendingWithdrawal, len(b.builderPendingWithdrawals))
	for i, withdrawal := range b.builderPendingWithdrawals {
		withdrawals[i] = withdrawal.Copy()
	}

	return withdrawals
}

// buildersVal returns a copy of the builders registry.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) buildersVal() []*ethpb.Builder {
	if b.builders == nil {
		return nil
	}

	builders := make([]*ethpb.Builder, len(b.builders))
	for i := range builders {
		builder := b.builders[i]
		builders[i] = ethpb.CopyBuilder(builder)
	}

	return builders
}

// latestBlockHashVal returns a copy of the latest block hash.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) latestBlockHashVal() []byte {
	if b.latestBlockHash == nil {
		return nil
	}

	hash := make([]byte, len(b.latestBlockHash))
	copy(hash, b.latestBlockHash)

	return hash
}

// payloadExpectedWithdrawalsVal returns a copy of the payload expected withdrawals.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) payloadExpectedWithdrawalsVal() []*enginev1.Withdrawal {
	if b.payloadExpectedWithdrawals == nil {
		return nil
	}

	withdrawals := make([]*enginev1.Withdrawal, len(b.payloadExpectedWithdrawals))
	for i, withdrawal := range b.payloadExpectedWithdrawals {
		withdrawals[i] = withdrawal.Copy()
	}

	return withdrawals
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
