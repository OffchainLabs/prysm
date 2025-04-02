package state_native

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// ExitEpochAndUpdateChurn computes the exit epoch and updates the churn. This method mutates the state.
//
// Spec definition:
//
//	def compute_exit_epoch_and_update_churn(state: BeaconState, exit_balance: Gwei) -> Epoch:
//	    earliest_exit_epoch = max(state.earliest_exit_epoch, compute_activation_exit_epoch(get_current_epoch(state)))
//	    per_epoch_churn = get_activation_exit_churn_limit(state)
//	    # New epoch for exits.
//	    if state.earliest_exit_epoch < earliest_exit_epoch:
//	        exit_balance_to_consume = per_epoch_churn
//	    else:
//	        exit_balance_to_consume = state.exit_balance_to_consume
//
//	    # Exit doesn't fit in the current earliest epoch.
//	    if exit_balance > exit_balance_to_consume:
//	        balance_to_process = exit_balance - exit_balance_to_consume
//	        additional_epochs = (balance_to_process - 1) // per_epoch_churn + 1
//	        earliest_exit_epoch += additional_epochs
//	        exit_balance_to_consume += additional_epochs * per_epoch_churn
//
//	    # Consume the balance and update state variables.
//	    state.exit_balance_to_consume = exit_balance_to_consume - exit_balance
//	    state.earliest_exit_epoch = earliest_exit_epoch
//
//	    return state.earliest_exit_epoch
func (b *BeaconState) ExitEpochAndUpdateChurn(exitBalance primitives.Gwei) (primitives.Epoch, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("ExitEpochAndUpdateChurn", b.version)
	}

	// This helper requires access to the RLock and cannot be called from within the write Lock.
	activeBal, err := helpers.TotalActiveBalance(b)
	if err != nil {
		return 0, err
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	earliestExitEpoch := max(b.earliestExitEpoch, helpers.ActivationExitEpoch(slots.ToEpoch(b.slot)))
	perEpochChurn := helpers.ActivationExitChurnLimit(primitives.Gwei(activeBal)) // Guaranteed to be non-zero.

	// New epoch for exits
	var exitBalanceToConsume primitives.Gwei
	if b.earliestExitEpoch < earliestExitEpoch {
		exitBalanceToConsume = perEpochChurn
	} else {
		exitBalanceToConsume = b.exitBalanceToConsume
	}

	// Exit doesn't fit in the current earliest epoch.
	if exitBalance > exitBalanceToConsume {
		balanceToProcess := exitBalance - exitBalanceToConsume
		additionalEpochs := primitives.Epoch((balanceToProcess-1)/perEpochChurn + 1)
		earliestExitEpoch += additionalEpochs
		exitBalanceToConsume += primitives.Gwei(additionalEpochs) * perEpochChurn
	}

	// Consume the balance and update state variables.
	b.exitBalanceToConsume = exitBalanceToConsume - exitBalance
	b.earliestExitEpoch = earliestExitEpoch

	b.markFieldAsDirty(types.ExitBalanceToConsume)
	b.markFieldAsDirty(types.EarliestExitEpoch)

	return b.earliestExitEpoch, nil
}
