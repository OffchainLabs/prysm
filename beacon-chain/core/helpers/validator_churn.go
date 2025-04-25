package helpers

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
)

// BalanceChurnLimit for the current active balance, in gwei.
// New in Electra EIP-7251: https://eips.ethereum.org/EIPS/eip-7251
//
// Spec definition:
//
//	def get_balance_churn_limit(state: BeaconState) -> Gwei:
//	    """
//	    Return the churn limit for the current epoch.
//	    """
//	    churn = max(
//	        MIN_PER_EPOCH_CHURN_LIMIT_ELECTRA,
//	        get_total_active_balance(state) // CHURN_LIMIT_QUOTIENT
//	    )
//	    return churn - churn % EFFECTIVE_BALANCE_INCREMENT
func BalanceChurnLimit(activeBalance primitives.Gwei) primitives.Gwei {
	churn := max(
		params.BeaconConfig().MinPerEpochChurnLimitElectra,
		uint64(activeBalance)/params.BeaconConfig().ChurnLimitQuotient,
	)
	return primitives.Gwei(churn - churn%params.BeaconConfig().EffectiveBalanceIncrement)
}

// ActivationExitChurnLimit for the current active balance, in gwei.
// New in Electra EIP-7251: https://eips.ethereum.org/EIPS/eip-7251
//
// Spec definition:
//
//	def get_activation_exit_churn_limit(state: BeaconState) -> Gwei:
//	    """
//	    Return the churn limit for the current epoch dedicated to activations and exits.
//	    """
//	    return min(MAX_PER_EPOCH_ACTIVATION_EXIT_CHURN_LIMIT, get_balance_churn_limit(state))
func ActivationExitChurnLimit(activeBalance primitives.Gwei) primitives.Gwei {
	return min(primitives.Gwei(params.BeaconConfig().MaxPerEpochActivationExitChurnLimit), BalanceChurnLimit(activeBalance))
}

// ConsolidationChurnLimit for the current active balance, in gwei.
// New in EIP-7251: https://eips.ethereum.org/EIPS/eip-7251
//
// Spec definition:
//
//	def get_consolidation_churn_limit(state: BeaconState) -> Gwei:
//	    return get_balance_churn_limit(state) - get_activation_exit_churn_limit(state)
func ConsolidationChurnLimit(activeBalance primitives.Gwei) primitives.Gwei {
	return BalanceChurnLimit(activeBalance) - ActivationExitChurnLimit(activeBalance)
}
