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
// Spec v1.7.0 (pseudocode):
//
//	slot = header_1.slot
//	proposal_epoch = compute_epoch_at_slot(slot)
//	if proposal_epoch == get_current_epoch(state):
//	  payment_index = SLOTS_PER_EPOCH + slot % SLOTS_PER_EPOCH
//	  state.builder_pending_payments[payment_index] = BuilderPendingPayment()
//	elif proposal_epoch == get_previous_epoch(state):
//	  payment_index = slot % SLOTS_PER_EPOCH
//	  state.builder_pending_payments[payment_index] = BuilderPendingPayment()
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

	if err := st.ClearBuilderPendingPayment(paymentIndex); err != nil {
		return errors.Wrap(err, "could not clear builder pending payment")
	}

	return nil
}
