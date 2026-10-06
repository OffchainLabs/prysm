package electra

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
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
//	        # [Modified in EIP8148]
//	        sweep_threshold = state.validator_sweep_thresholds[index]
//	        effective_sweep_threshold = get_effective_sweep_threshold(validator, sweep_threshold)
//
//	        if (
//	            balance + DOWNWARD_THRESHOLD < validator.effective_balance
//	            or validator.effective_balance + UPWARD_THRESHOLD < balance
//	        ):
//	            # [Modified in EIP8148]
//	            validator.effective_balance = min(
//	                balance - balance % EFFECTIVE_BALANCE_INCREMENT, effective_sweep_threshold
//	            )
func ProcessEffectiveBalanceUpdates(st state.BeaconState) error {
	effBalanceInc := params.BeaconConfig().EffectiveBalanceIncrement
	hysteresisInc := effBalanceInc / params.BeaconConfig().HysteresisQuotient
	downwardThreshold := hysteresisInc * params.BeaconConfig().HysteresisDownwardMultiplier
	upwardThreshold := hysteresisInc * params.BeaconConfig().HysteresisUpwardMultiplier

	bals := st.Balances()

	sweepThresholds := make([]uint64, len(bals))
	if st.Version() >= version.Gloas {
		var err error
		sweepThresholds, err = st.ValidatorSweepThresholds()
		if err != nil {
			return fmt.Errorf("validator sweep thresholds: %w", err)
		}
		if len(sweepThresholds) != len(bals) {
			return fmt.Errorf("sweep thresholds length does not match balances length %d != %d", len(sweepThresholds), len(bals))
		}
	}

	// Update effective balances with hysteresis.
	validatorFunc := func(idx int, val state.ReadOnlyValidator) (newVal *ethpb.Validator, err error) {
		if idx >= len(bals) {
			return nil, fmt.Errorf("validator index exceeds validator length in state %d >= %d", idx, len(st.Balances()))
		}
		balance := bals[idx]

		// [Modified in EIP8148]
		effectiveSweepThreshold := helpers.EffectiveSweepThreshold(val, sweepThresholds[idx])

		if balance+downwardThreshold < val.EffectiveBalance() || val.EffectiveBalance()+upwardThreshold < balance {
			effectiveBal := min(balance-balance%effBalanceInc, effectiveSweepThreshold)
			if effectiveBal != val.EffectiveBalance() {
				newVal = val.Copy()
				newVal.EffectiveBalance = effectiveBal
			}
		}
		return newVal, nil
	}

	return st.ApplyToEveryValidator(validatorFunc)
}
