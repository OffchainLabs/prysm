package electra

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/math"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// ComputeConsolidationEpochAndUpdateChurn fulfills the consensus spec definition below. This method
// calls mutating methods to the beacon state.
//
// Spec definition:
//
//	def compute_consolidation_epoch_and_update_churn(state: BeaconState, consolidation_balance: Gwei) -> Epoch:
//	    earliest_consolidation_epoch = max(
//	        state.earliest_consolidation_epoch, compute_activation_exit_epoch(get_current_epoch(state)))
//	    per_epoch_consolidation_churn = get_consolidation_churn_limit(state)
//	    # New epoch for consolidations.
//	    if state.earliest_consolidation_epoch < earliest_consolidation_epoch:
//	        consolidation_balance_to_consume = per_epoch_consolidation_churn
//	    else:
//	        consolidation_balance_to_consume = state.consolidation_balance_to_consume
//
//	    # Consolidation doesn't fit in the current earliest epoch.
//	    if consolidation_balance > consolidation_balance_to_consume:
//	        balance_to_process = consolidation_balance - consolidation_balance_to_consume
//	        additional_epochs = (balance_to_process - 1) // per_epoch_consolidation_churn + 1
//	        earliest_consolidation_epoch += additional_epochs
//	        consolidation_balance_to_consume += additional_epochs * per_epoch_consolidation_churn
//
//	    # Consume the balance and update state variables.
//	    state.consolidation_balance_to_consume = consolidation_balance_to_consume - consolidation_balance
//	    state.earliest_consolidation_epoch = earliest_consolidation_epoch
//
//	    return state.earliest_consolidation_epoch
func ComputeConsolidationEpochAndUpdateChurn(ctx context.Context, s state.BeaconState, consolidationBalance primitives.Gwei) (primitives.Epoch, error) {
	earliestEpoch, err := s.EarliestConsolidationEpoch()
	if err != nil {
		return 0, err
	}
	earliestConsolidationEpoch := max(earliestEpoch, helpers.ActivationExitEpoch(slots.ToEpoch(s.Slot())))
	activeBal, err := helpers.TotalActiveBalance(s)
	if err != nil {
		return 0, err
	}
	perEpochConsolidationChurn := helpers.ConsolidationChurnLimit(primitives.Gwei(activeBal))

	// New epoch for consolidations.
	var consolidationBalanceToConsume primitives.Gwei
	if earliestEpoch < earliestConsolidationEpoch {
		consolidationBalanceToConsume = perEpochConsolidationChurn
	} else {
		consolidationBalanceToConsume, err = s.ConsolidationBalanceToConsume()
		if err != nil {
			return 0, err
		}
	}

	// Consolidation doesn't fit in the current earliest epoch.
	if consolidationBalance > consolidationBalanceToConsume {
		balanceToProcess := consolidationBalance - consolidationBalanceToConsume
		// additional_epochs = (balance_to_process - 1) // per_epoch_consolidation_churn + 1
		additionalEpochs, err := math.Div64(uint64(balanceToProcess-1), uint64(perEpochConsolidationChurn))
		if err != nil {
			return 0, err
		}
		additionalEpochs++
		earliestConsolidationEpoch += primitives.Epoch(additionalEpochs)
		consolidationBalanceToConsume += primitives.Gwei(additionalEpochs) * perEpochConsolidationChurn
	}

	// Consume the balance and update state variables.
	if err := s.SetConsolidationBalanceToConsume(consolidationBalanceToConsume - consolidationBalance); err != nil {
		return 0, err
	}
	if err := s.SetEarliestConsolidationEpoch(earliestConsolidationEpoch); err != nil {
		return 0, err
	}

	return earliestConsolidationEpoch, nil
}
