package gloas

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// RemoveBuilderPendingPayment removes the pending builder payment for the proposal slot.
//
//	<spec fn="process_proposer_slashing" fork="gloas" lines="22-36" hash="todo">
//	# [New in Gloas:EIP7732]
//	# Remove the BuilderPendingPayment corresponding to this proposal if it is
//	# still in the 2-epoch window. Only clear it when the slashed validator is
//	# the proposer associated with the payment; otherwise an unrelated same-slot
//	# equivocation could grief an honest proposer's payment.
//	slot = header_1.slot
//	proposal_epoch = compute_epoch_at_slot(slot)
//	if proposal_epoch == get_current_epoch(state):
//	    payment_index = SLOTS_PER_EPOCH + slot % SLOTS_PER_EPOCH
//	    payment = state.builder_pending_payments[payment_index]
//	    if payment.proposer_index == header_1.proposer_index:
//	        state.builder_pending_payments[payment_index] = BuilderPendingPayment()
//	elif proposal_epoch == get_previous_epoch(state):
//	    payment_index = slot % SLOTS_PER_EPOCH
//	    payment = state.builder_pending_payments[payment_index]
//	    if payment.proposer_index == header_1.proposer_index:
//	        state.builder_pending_payments[payment_index] = BuilderPendingPayment()
//	</spec>
func RemoveBuilderPendingPayment(st state.BeaconState, header *eth.BeaconBlockHeader) error {
	proposalEpoch := slots.ToEpoch(header.Slot)
	currentEpoch := time.CurrentEpoch(st)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch

	var paymentIndex primitives.Slot
	if proposalEpoch == currentEpoch {
		paymentIndex = slotsPerEpoch + header.Slot%slotsPerEpoch
	} else if proposalEpoch+1 == currentEpoch {
		paymentIndex = header.Slot % slotsPerEpoch
	} else {
		return nil
	}

	payment, err := st.BuilderPendingPayment(uint64(paymentIndex))
	if err != nil {
		return errors.Wrap(err, "could not get builder pending payment")
	}
	if payment.ProposerIndex != header.ProposerIndex {
		return nil
	}

	if err := st.ClearBuilderPendingPayment(paymentIndex); err != nil {
		return errors.Wrap(err, "could not clear builder pending payment")
	}

	return nil
}
