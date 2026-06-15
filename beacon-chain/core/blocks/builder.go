package blocks

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// InitiateBuilderExit initiates the exit of a builder by setting its withdrawable epoch.
//
//	<spec fn="initiate_builder_exit" fork="gloas" hash="f71d22b9">
//	def initiate_builder_exit(state: BeaconState, builder_index: BuilderIndex) -> None:
//	    """
//	    Initiate the exit of the builder with index ``index``.
//	    """
//	    # Set builder exit epoch
//	    builder = state.builders[builder_index]
//	    builder.withdrawable_epoch = get_current_epoch(state) + MIN_BUILDER_WITHDRAWABILITY_DELAY
//	</spec>
func InitiateBuilderExit(s state.BeaconState, builderIndex primitives.BuilderIndex) error {
	builder, err := s.Builder(builderIndex)
	if err != nil {
		return err
	}
	// Return if builder already initiated exit.
	if builder.WithdrawableEpoch != params.BeaconConfig().FarFutureEpoch {
		return nil
	}
	currentEpoch := slots.ToEpoch(s.Slot())
	builder.WithdrawableEpoch = currentEpoch + params.BeaconConfig().MinBuilderWithdrawabilityDelay
	return s.UpdateBuilderAtIndex(builderIndex, builder)
}

// RemoveBuilderPendingPayment removes the pending builder payment for the proposal slot.
//
//	<spec fn="process_proposer_slashing" fork="gloas" lines="22-32" hash="4da721ef">
//	# [New in Gloas:EIP7732]
//	# Remove the BuilderPendingPayment corresponding to
//	# this proposal if it is still in the 2-epoch window.
//	slot = header_1.slot
//	proposal_epoch = compute_epoch_at_slot(slot)
//	if proposal_epoch == get_current_epoch(state):
//	    payment_index = SLOTS_PER_EPOCH + slot % SLOTS_PER_EPOCH
//	    state.builder_pending_payments[payment_index] = BuilderPendingPayment()
//	elif proposal_epoch == get_previous_epoch(state):
//	    payment_index = slot % SLOTS_PER_EPOCH
//	    state.builder_pending_payments[payment_index] = BuilderPendingPayment()
//	</spec>
func RemoveBuilderPendingPayment(st state.BeaconState, header *ethpb.BeaconBlockHeader) error {
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
