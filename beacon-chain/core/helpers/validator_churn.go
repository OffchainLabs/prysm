package helpers

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
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

// Per-epoch churn is a rate, so it scales with the slot duration in effect at the epoch (EIP-8198).
func scaleChurnBySlotDuration(churn uint64, epoch primitives.Epoch) uint64 {
	cfg := params.BeaconConfig()
	return churn * cfg.SlotDurationMillisAtEpoch(epoch) / cfg.SlotDurationMillis()
}

// activationChurnLimitGloas returns the per-epoch activation churn limit, capped by
// MAX_PER_EPOCH_ACTIVATION_CHURN_LIMIT_GLOAS. New in Gloas EIP-8061, scaled in EIP-8198.
//
// Spec definition:
//
//	def get_activation_churn_limit(state: BeaconState) -> Gwei:
//	    churn = max(
//	        MIN_PER_EPOCH_CHURN_LIMIT_ELECTRA,
//	        get_total_active_balance(state) // CHURN_LIMIT_QUOTIENT_GLOAS,
//	    )
//	    churn = min(MAX_PER_EPOCH_ACTIVATION_CHURN_LIMIT_GLOAS, churn)
//	    slot_duration_ms = get_slot_duration_ms(get_current_epoch(state))
//	    churn = churn * slot_duration_ms // get_slot_duration_ms(GENESIS_EPOCH)
//	    return Gwei(churn - churn % EFFECTIVE_BALANCE_INCREMENT)
func activationChurnLimitGloas(activeBalance primitives.Gwei, epoch primitives.Epoch) primitives.Gwei {
	cfg := params.BeaconConfig()
	churn := max(cfg.MinPerEpochChurnLimitElectra, uint64(activeBalance)/cfg.ChurnLimitQuotientGloas)
	churn = min(cfg.MaxPerEpochActivationChurnLimitGloas, churn)
	churn = scaleChurnBySlotDuration(churn, epoch)
	return primitives.Gwei(churn - churn%cfg.EffectiveBalanceIncrement)
}

// exitChurnLimitGloas returns the per-epoch exit churn limit. Uncapped in Gloas EIP-8061
// so that exits scale with total stake, scaled in EIP-8198.
//
// Spec definition:
//
//	def get_exit_churn_limit(state: BeaconState) -> Gwei:
//	    churn = max(
//	        MIN_PER_EPOCH_CHURN_LIMIT_ELECTRA,
//	        get_total_active_balance(state) // CHURN_LIMIT_QUOTIENT_GLOAS,
//	    )
//	    slot_duration_ms = get_slot_duration_ms(get_current_epoch(state))
//	    churn = churn * slot_duration_ms // get_slot_duration_ms(GENESIS_EPOCH)
//	    return Gwei(churn - churn % EFFECTIVE_BALANCE_INCREMENT)
func exitChurnLimitGloas(activeBalance primitives.Gwei, epoch primitives.Epoch) primitives.Gwei {
	cfg := params.BeaconConfig()
	churn := max(cfg.MinPerEpochChurnLimitElectra, uint64(activeBalance)/cfg.ChurnLimitQuotientGloas)
	churn = scaleChurnBySlotDuration(churn, epoch)
	return primitives.Gwei(churn - churn%cfg.EffectiveBalanceIncrement)
}

// consolidationChurnLimitGloas returns the per-epoch consolidation churn limit, derived
// independently from total active balance via CONSOLIDATION_CHURN_LIMIT_QUOTIENT.
// New in Gloas EIP-8061, scaled in EIP-8198.
//
// Spec definition:
//
//	def get_consolidation_churn_limit(state: BeaconState) -> Gwei:
//	    churn = get_total_active_balance(state) // CONSOLIDATION_CHURN_LIMIT_QUOTIENT
//	    slot_duration_ms = get_slot_duration_ms(get_current_epoch(state))
//	    churn = churn * slot_duration_ms // get_slot_duration_ms(GENESIS_EPOCH)
//	    return Gwei(churn - churn % EFFECTIVE_BALANCE_INCREMENT)
func consolidationChurnLimitGloas(activeBalance primitives.Gwei, epoch primitives.Epoch) primitives.Gwei {
	cfg := params.BeaconConfig()
	churn := uint64(activeBalance) / cfg.ConsolidationChurnLimitQuotient
	churn = scaleChurnBySlotDuration(churn, epoch)
	return primitives.Gwei(churn - churn%cfg.EffectiveBalanceIncrement)
}

// ActivationChurnLimitForVersion dispatches to the Gloas or pre-Gloas activation churn helper.
func ActivationChurnLimitForVersion(v int, activeBalance primitives.Gwei, epoch primitives.Epoch) primitives.Gwei {
	if v >= version.Gloas {
		return activationChurnLimitGloas(activeBalance, epoch)
	}
	return ActivationExitChurnLimit(activeBalance)
}

// ExitChurnLimitForVersion dispatches to the Gloas or pre-Gloas exit churn helper.
func ExitChurnLimitForVersion(v int, activeBalance primitives.Gwei, epoch primitives.Epoch) primitives.Gwei {
	if v >= version.Gloas {
		return exitChurnLimitGloas(activeBalance, epoch)
	}
	return ActivationExitChurnLimit(activeBalance)
}

// ConsolidationChurnLimitForVersion dispatches to the Gloas or pre-Gloas consolidation churn helper.
func ConsolidationChurnLimitForVersion(v int, activeBalance primitives.Gwei, epoch primitives.Epoch) primitives.Gwei {
	if v >= version.Gloas {
		return consolidationChurnLimitGloas(activeBalance, epoch)
	}
	return ConsolidationChurnLimit(activeBalance)
}
