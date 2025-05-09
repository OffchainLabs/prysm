package electra

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
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
	effBalanceInc := params.BeaconConfig().EffectiveBalanceIncrement
	hysteresisInc := effBalanceInc / params.BeaconConfig().HysteresisQuotient
	downwardThreshold := hysteresisInc * params.BeaconConfig().HysteresisDownwardMultiplier
	upwardThreshold := hysteresisInc * params.BeaconConfig().HysteresisUpwardMultiplier

	bals := st.Balances()

	// Update effective balances with hysteresis.
	validatorFunc := func(idx int, val state.ReadOnlyValidator) (newVal *ethpb.Validator, err error) {
		if val.IsNil() {
			return nil, fmt.Errorf("validator %d is nil in state", idx)
		}
		if idx >= len(bals) {
			return nil, fmt.Errorf("validator index exceeds validator length in state %d >= %d", idx, len(st.Balances()))
		}
		balance := bals[idx]

		effectiveBalanceLimit := params.BeaconConfig().MinActivationBalance
		if val.HasCompoundingWithdrawalCredentials() {
			effectiveBalanceLimit = params.BeaconConfig().MaxEffectiveBalanceElectra
		}

		if balance+downwardThreshold < val.EffectiveBalance() || val.EffectiveBalance()+upwardThreshold < balance {
			effectiveBal := min(balance-balance%effBalanceInc, effectiveBalanceLimit)
			newVal = val.Copy()
			newVal.EffectiveBalance = effectiveBal
		}
		return newVal, nil
	}

	return st.ApplyToEveryValidator(validatorFunc)
}
