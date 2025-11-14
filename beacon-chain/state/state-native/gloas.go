package state_native

import (
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

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

// latestWithdrawalsRootVal returns a copy of the latest withdrawals root.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) latestWithdrawalsRootVal() []byte {
	if b.latestWithdrawalsRoot == nil {
		return nil
	}

	root := make([]byte, len(b.latestWithdrawalsRoot))
	copy(root, b.latestWithdrawalsRoot)

	return root
}
