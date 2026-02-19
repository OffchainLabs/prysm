package electra

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
)

// ProcessEffectiveBalanceUpdates processes effective balance updates during epoch processing.
//
// Spec pseudocode definition:
//
//	def process_effective_balance_updates(state: BeaconState) -> None:
//	    # Update effective balances with hysteresis
//	    for index, validator in enumerate(state.validators):
//	        balance = state.balances[index]
//	        HYSTERESIS_INCREMENT = uint64(EFFECTIVE_BALANCE_INCREMENT // HYSTERESIS_QUOTIENT)
//	        DOWNWARD_THRESHOLD = HYSTERESIS_INCREMENT * HYSTERESIS_DOWNWARD_MULTIPLIER
//	        UPWARD_THRESHOLD = HYSTERESIS_INCREMENT * HYSTERESIS_UPWARD_MULTIPLIER
//	        EFFECTIVE_BALANCE_LIMIT = (
//	            MAX_EFFECTIVE_BALANCE_EIP7251 if has_compounding_withdrawal_credential(validator)
//	            else MIN_ACTIVATION_BALANCE
//	        )
//
//	        if (
//	            balance + DOWNWARD_THRESHOLD < validator.effective_balance
//	            or validator.effective_balance + UPWARD_THRESHOLD < balance
//	        ):
//	            validator.effective_balance = min(balance - balance % EFFECTIVE_BALANCE_INCREMENT, EFFECTIVE_BALANCE_LIMIT)
func ProcessEffectiveBalanceUpdates(st state.BeaconState) error {
	cfg := params.BeaconConfig()
	effBalanceInc := cfg.EffectiveBalanceIncrement
	hysteresisInc := effBalanceInc / cfg.HysteresisQuotient
	downwardThreshold := hysteresisInc * cfg.HysteresisDownwardMultiplier
	upwardThreshold := hysteresisInc * cfg.HysteresisUpwardMultiplier
	minActivationBalance := cfg.MinActivationBalance
	maxEffBalanceElectra := cfg.MaxEffectiveBalanceElectra
	compoundingPrefix := cfg.CompoundingWithdrawalPrefixByte

	bals := st.Balances()

	return st.ApplyToEveryCompactValidator(func(idx int, val *stateutil.CompactValidator) (stateutil.CompactValidator, bool, error) {
		if idx >= len(bals) {
			return stateutil.CompactValidator{}, false, fmt.Errorf("validator index exceeds validator length in state %d >= %d", idx, len(bals))
		}
		balance := bals[idx]

		effectiveBalanceLimit := minActivationBalance
		if val.WithdrawalCredentials[0] == compoundingPrefix {
			effectiveBalanceLimit = maxEffBalanceElectra
		}

		if balance+downwardThreshold < val.EffectiveBalance || val.EffectiveBalance+upwardThreshold < balance {
			effectiveBal := min(balance-balance%effBalanceInc, effectiveBalanceLimit)
			updated := *val
			updated.EffectiveBalance = effectiveBal
			return updated, true, nil
		}
		return stateutil.CompactValidator{}, false, nil
	})
}
