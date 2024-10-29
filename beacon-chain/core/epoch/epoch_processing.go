// Package epoch contains epoch processing libraries according to spec, able to
// process new balance for validators, justify and finalize new
// check points, and shuffle validators to different slots and
// shards.
package epoch

import (
	"context"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/validators"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stateutil"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/math"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// ProcessRegistryUpdates rotates validators in and out of active pool.
// the amount to rotate is determined churn limit.
//
// Spec pseudocode definition:
//
//	def process_registry_updates(state: BeaconState) -> None:
//	 # Process activation eligibility and ejections
//	 for index, validator in enumerate(state.validators):
//	     if is_eligible_for_activation_queue(validator):
//	         validator.activation_eligibility_epoch = get_current_epoch(state) + 1
//
//	     if is_active_validator(validator, get_current_epoch(state)) and validator.effective_balance <= EJECTION_BALANCE:
//	         initiate_validator_exit(state, ValidatorIndex(index))
//
//	 # Queue validators eligible for activation and not yet dequeued for activation
//	 activation_queue = sorted([
//	     index for index, validator in enumerate(state.validators)
//	     if is_eligible_for_activation(state, validator)
//	     # Order by the sequence of activation_eligibility_epoch setting and then index
//	 ], key=lambda index: (state.validators[index].activation_eligibility_epoch, index))
//	 # Dequeued validators for activation up to churn limit
//	 for index in activation_queue[:get_validator_churn_limit(state)]:
//	     validator = state.validators[index]
//	     validator.activation_epoch = compute_activation_exit_epoch(get_current_epoch(state))
func ProcessRegistryUpdates(ctx context.Context, st state.BeaconState) (state.BeaconState, error) {
	currentEpoch := time.CurrentEpoch(st)
	var err error
	ejectionBal := params.BeaconConfig().EjectionBalance

	// To avoid copying the state validator set via st.Validators(), we will perform a read only pass
	// over the validator set while collecting validator indices where the validator copy is actually
	// necessary, then we will process these operations.
	eligibleForActivationQ := make([]primitives.ValidatorIndex, 0)
	eligibleForActivation := make([]primitives.ValidatorIndex, 0)
	eligibleForEjection := make([]primitives.ValidatorIndex, 0)

	if err := st.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		// Collect validators eligible to enter the activation queue.
		if helpers.IsEligibleForActivationQueue(val, currentEpoch) {
			eligibleForActivationQ = append(eligibleForActivationQ, primitives.ValidatorIndex(idx))
		}

		// Collect validators to eject.
		isActive := helpers.IsActiveValidatorUsingTrie(val, currentEpoch)
		belowEjectionBalance := val.EffectiveBalance() <= ejectionBal
		if isActive && belowEjectionBalance {
			eligibleForEjection = append(eligibleForEjection, primitives.ValidatorIndex(idx))
		}

		// Collect validators eligible for activation and not yet dequeued for activation.
		if helpers.IsEligibleForActivationUsingROVal(st, val) {
			eligibleForActivation = append(eligibleForActivation, primitives.ValidatorIndex(idx))
		}

		return nil
	}); err != nil {
		return st, fmt.Errorf("failed to read validators: %w", err)
	}

	// Process validators for activation eligibility.
	activationEligibilityEpoch := time.CurrentEpoch(st) + 1
	for _, idx := range eligibleForActivationQ {
		v, err := st.ValidatorAtIndex(idx)
		if err != nil {
			return nil, err
		}
		v.ActivationEligibilityEpoch = activationEligibilityEpoch
		if err := st.UpdateValidatorAtIndex(idx, v); err != nil {
			return nil, err
		}
	}

	// Process validators eligible for ejection.
	for _, idx := range eligibleForEjection {
		// Here is fine to do a quadratic loop since this should
		// barely happen
		maxExitEpoch, churn := validators.MaxExitEpochAndChurn(st)
		st, _, err = validators.InitiateValidatorExit(ctx, st, idx, maxExitEpoch, churn)
		if err != nil && !errors.Is(err, validators.ErrValidatorAlreadyExited) {
			return nil, errors.Wrapf(err, "could not initiate exit for validator %d", idx)
		}
	}

	// Queue validators eligible for activation and not yet dequeued for activation.
	sort.Sort(sortableIndices{indices: eligibleForActivation, state: st})

	// Only activate just enough validators according to the activation churn limit.
	limit := uint64(len(eligibleForActivation))
	activeValidatorCount, err := helpers.ActiveValidatorCount(ctx, st, currentEpoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not get active validator count")
	}

	churnLimit := helpers.ValidatorActivationChurnLimit(activeValidatorCount)

	if st.Version() >= version.Deneb {
		churnLimit = helpers.ValidatorActivationChurnLimitDeneb(activeValidatorCount)
	}

	// Prevent churn limit cause index out of bound.
	if churnLimit < limit {
		limit = churnLimit
	}

	activationExitEpoch := helpers.ActivationExitEpoch(currentEpoch)
	for _, index := range eligibleForActivation[:limit] {
		validator, err := st.ValidatorAtIndex(index)
		if err != nil {
			return nil, err
		}
		validator.ActivationEpoch = activationExitEpoch
		if err := st.UpdateValidatorAtIndex(index, validator); err != nil {
			return nil, err
		}
	}
	return st, nil
}

// ProcessSlashings processes the slashed validators during epoch processing,
//
//	def process_slashings(state: BeaconState) -> None:
//	  epoch = get_current_epoch(state)
//	  total_balance = get_total_active_balance(state)
//	  adjusted_total_slashing_balance = min(sum(state.slashings) * PROPORTIONAL_SLASHING_MULTIPLIER, total_balance)
//	  if state.version == electra:
//		 increment = EFFECTIVE_BALANCE_INCREMENT  # Factored out from total balance to avoid uint64 overflow
//	  penalty_per_effective_balance_increment = adjusted_total_slashing_balance // (total_balance // increment)
//	  for index, validator in enumerate(state.validators):
//	      if validator.slashed and epoch + EPOCHS_PER_SLASHINGS_VECTOR // 2 == validator.withdrawable_epoch:
//	          increment = EFFECTIVE_BALANCE_INCREMENT  # Factored out from penalty numerator to avoid uint64 overflow
//	          penalty_numerator = validator.effective_balance // increment * adjusted_total_slashing_balance
//	          penalty = penalty_numerator // total_balance * increment
//	          if state.version == electra:
//	            effective_balance_increments = validator.effective_balance // increment
//	            penalty = penalty_per_effective_balance_increment * effective_balance_increments
//	          decrease_balance(state, ValidatorIndex(index), penalty)
func ProcessSlashings(st state.BeaconState, slashingMultiplier uint64) (state.BeaconState, error) {
	currentEpoch := time.CurrentEpoch(st)
	totalBalance, err := helpers.TotalActiveBalance(st)
	if err != nil {
		return nil, errors.Wrap(err, "could not get total active balance")
	}

	// Compute slashed balances in the current epoch
	exitLength := params.BeaconConfig().EpochsPerSlashingsVector

	// Compute the sum of state slashings
	slashings := st.Slashings()
	totalSlashing := uint64(0)
	for _, slashing := range slashings {
		totalSlashing, err = math.Add64(totalSlashing, slashing)
		if err != nil {
			return nil, err
		}
	}

	// a callback is used here to apply the following actions to all validators
	// below equally.
	increment := params.BeaconConfig().EffectiveBalanceIncrement
	minSlashing := math.Min(totalSlashing*slashingMultiplier, totalBalance)

	// Modified in Electra:EIP7251
	var penaltyPerEffectiveBalanceIncrement uint64
	if st.Version() >= version.Electra {
		penaltyPerEffectiveBalanceIncrement = minSlashing / (totalBalance / increment)
	}

	bals := st.Balances()
	changed := false
	err = st.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		correctEpoch := (currentEpoch + exitLength/2) == val.WithdrawableEpoch()
		if val.Slashed() && correctEpoch {
			var penalty uint64
			if st.Version() >= version.Electra {
				effectiveBalanceIncrements := val.EffectiveBalance() / increment
				penalty = penaltyPerEffectiveBalanceIncrement * effectiveBalanceIncrements
			} else {
				penaltyNumerator := val.EffectiveBalance() / increment * minSlashing
				penalty = penaltyNumerator / totalBalance * increment
			}
			bals[idx] = helpers.DecreaseBalanceWithVal(bals[idx], penalty)
			changed = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if changed {
		if err := st.SetBalances(bals); err != nil {
			return nil, err
		}
	}
	return st, nil
}

// ProcessEth1DataReset processes updates to ETH1 data votes during epoch processing.
//
// Spec pseudocode definition:
//
//	def process_eth1_data_reset(state: BeaconState) -> None:
//	  next_epoch = Epoch(get_current_epoch(state) + 1)
//	  # Reset eth1 data votes
//	  if next_epoch % EPOCHS_PER_ETH1_VOTING_PERIOD == 0:
//	      state.eth1_data_votes = []
func ProcessEth1DataReset(state state.BeaconState) (state.BeaconState, error) {
	currentEpoch := time.CurrentEpoch(state)
	nextEpoch := currentEpoch + 1

	// Reset ETH1 data votes.
	if nextEpoch%params.BeaconConfig().EpochsPerEth1VotingPeriod == 0 {
		if err := state.SetEth1DataVotes([]*ethpb.Eth1Data{}); err != nil {
			return nil, err
		}
	}

	return state, nil
}

// ProcessEffectiveBalanceUpdates processes effective balance updates during epoch processing.
//
// Spec pseudocode definition:
//
//	def process_effective_balance_updates(state: BeaconState) -> None:
//	  # Update effective balances with hysteresis
//	  for index, validator in enumerate(state.validators):
//	      balance = state.balances[index]
//	      HYSTERESIS_INCREMENT = uint64(EFFECTIVE_BALANCE_INCREMENT // HYSTERESIS_QUOTIENT)
//	      DOWNWARD_THRESHOLD = HYSTERESIS_INCREMENT * HYSTERESIS_DOWNWARD_MULTIPLIER
//	      UPWARD_THRESHOLD = HYSTERESIS_INCREMENT * HYSTERESIS_UPWARD_MULTIPLIER
//	      if (
//	          balance + DOWNWARD_THRESHOLD < validator.effective_balance
//	          or validator.effective_balance + UPWARD_THRESHOLD < balance
//	      ):
//	          validator.effective_balance = min(balance - balance % EFFECTIVE_BALANCE_INCREMENT, MAX_EFFECTIVE_BALANCE)
func ProcessEffectiveBalanceUpdates(st state.BeaconState) (state.BeaconState, error) {
	effBalanceInc := params.BeaconConfig().EffectiveBalanceIncrement
	maxEffBalance := params.BeaconConfig().MaxEffectiveBalance
	hysteresisInc := effBalanceInc / params.BeaconConfig().HysteresisQuotient
	downwardThreshold := hysteresisInc * params.BeaconConfig().HysteresisDownwardMultiplier
	upwardThreshold := hysteresisInc * params.BeaconConfig().HysteresisUpwardMultiplier

	bals := st.Balances()

	// Update effective balances with hysteresis.
	validatorFunc := func(idx int, val state.ReadOnlyValidator) (newVal *ethpb.Validator, err error) {
		if val == nil {
			return nil, fmt.Errorf("validator %d is nil in state", idx)
		}
		if idx >= len(bals) {
			return nil, fmt.Errorf("validator index exceeds validator length in state %d >= %d", idx, len(st.Balances()))
		}
		balance := bals[idx]

		if balance+downwardThreshold < val.EffectiveBalance() || val.EffectiveBalance()+upwardThreshold < balance {
			effectiveBal := maxEffBalance
			if effectiveBal > balance-balance%effBalanceInc {
				effectiveBal = balance - balance%effBalanceInc
			}
			if effectiveBal != val.EffectiveBalance() {
				newVal = val.Copy()
				newVal.EffectiveBalance = effectiveBal
			}
		}
		return
	}

	if err := st.ApplyToEveryValidator(validatorFunc); err != nil {
		return nil, err
	}

	return st, nil
}

// ProcessSlashingsReset processes the total slashing balances updates during epoch processing.
//
// Spec pseudocode definition:
//
//	def process_slashings_reset(state: BeaconState) -> None:
//	  next_epoch = Epoch(get_current_epoch(state) + 1)
//	  # Reset slashings
//	  state.slashings[next_epoch % EPOCHS_PER_SLASHINGS_VECTOR] = Gwei(0)
func ProcessSlashingsReset(state state.BeaconState) (state.BeaconState, error) {
	currentEpoch := time.CurrentEpoch(state)
	nextEpoch := currentEpoch + 1

	// Set total slashed balances.
	slashedExitLength := params.BeaconConfig().EpochsPerSlashingsVector
	slashedEpoch := nextEpoch % slashedExitLength
	slashings := state.Slashings()
	if uint64(len(slashings)) != uint64(slashedExitLength) {
		return nil, fmt.Errorf(
			"state slashing length %d different than EpochsPerHistoricalVector %d",
			len(slashings),
			slashedExitLength,
		)
	}
	if err := state.UpdateSlashingsAtIndex(uint64(slashedEpoch) /* index */, 0 /* value */); err != nil {
		return nil, err
	}

	return state, nil
}

// ProcessRandaoMixesReset processes the final updates to RANDAO mix during epoch processing.
//
// Spec pseudocode definition:
//
//	def process_randao_mixes_reset(state: BeaconState) -> None:
//	  current_epoch = get_current_epoch(state)
//	  next_epoch = Epoch(current_epoch + 1)
//	  # Set randao mix
//	  state.randao_mixes[next_epoch % EPOCHS_PER_HISTORICAL_VECTOR] = get_randao_mix(state, current_epoch)
func ProcessRandaoMixesReset(state state.BeaconState) (state.BeaconState, error) {
	currentEpoch := time.CurrentEpoch(state)
	nextEpoch := currentEpoch + 1

	// Set RANDAO mix.
	randaoMixLength := params.BeaconConfig().EpochsPerHistoricalVector
	if uint64(state.RandaoMixesLength()) != uint64(randaoMixLength) {
		return nil, fmt.Errorf(
			"state randao length %d different than EpochsPerHistoricalVector %d",
			state.RandaoMixesLength(),
			randaoMixLength,
		)
	}
	mix, err := helpers.RandaoMix(state, currentEpoch)
	if err != nil {
		return nil, err
	}
	if err := state.UpdateRandaoMixesAtIndex(uint64(nextEpoch%randaoMixLength), [32]byte(mix)); err != nil {
		return nil, err
	}

	return state, nil
}

// ProcessHistoricalDataUpdate processes the updates to historical data during epoch processing.
// From Capella onward, per spec,state's historical summaries are updated instead of historical roots.
func ProcessHistoricalDataUpdate(state state.BeaconState) (state.BeaconState, error) {
	currentEpoch := time.CurrentEpoch(state)
	nextEpoch := currentEpoch + 1

	// Set historical root accumulator.
	epochsPerHistoricalRoot := params.BeaconConfig().SlotsPerHistoricalRoot.DivSlot(params.BeaconConfig().SlotsPerEpoch)
	if nextEpoch.Mod(uint64(epochsPerHistoricalRoot)) == 0 {
		if state.Version() >= version.Capella {
			br, err := stateutil.ArraysRoot(state.BlockRoots(), fieldparams.BlockRootsLength)
			if err != nil {
				return nil, err
			}
			sr, err := stateutil.ArraysRoot(state.StateRoots(), fieldparams.StateRootsLength)
			if err != nil {
				return nil, err
			}
			if err := state.AppendHistoricalSummaries(&ethpb.HistoricalSummary{BlockSummaryRoot: br[:], StateSummaryRoot: sr[:]}); err != nil {
				return nil, err
			}
		} else {
			historicalBatch := &ethpb.HistoricalBatch{
				BlockRoots: state.BlockRoots(),
				StateRoots: state.StateRoots(),
			}
			batchRoot, err := historicalBatch.HashTreeRoot()
			if err != nil {
				return nil, errors.Wrap(err, "could not hash historical batch")
			}
			if err := state.AppendHistoricalRoots(batchRoot); err != nil {
				return nil, err
			}
		}
	}

	return state, nil
}

// ProcessParticipationRecordUpdates rotates current/previous epoch attestations during epoch processing.
//
// nolint:dupword
// Spec pseudocode definition:
//
//	def process_participation_record_updates(state: BeaconState) -> None:
//	  # Rotate current/previous epoch attestations
//	  state.previous_epoch_attestations = state.current_epoch_attestations
//	  state.current_epoch_attestations = []
func ProcessParticipationRecordUpdates(state state.BeaconState) (state.BeaconState, error) {
	if err := state.RotateAttestations(); err != nil {
		return nil, err
	}
	return state, nil
}

// ProcessFinalUpdates processes the final updates during epoch processing.
func ProcessFinalUpdates(state state.BeaconState) (state.BeaconState, error) {
	var err error

	// Reset ETH1 data votes.
	state, err = ProcessEth1DataReset(state)
	if err != nil {
		return nil, err
	}

	// Update effective balances with hysteresis.
	state, err = ProcessEffectiveBalanceUpdates(state)
	if err != nil {
		return nil, err
	}

	// Set total slashed balances.
	state, err = ProcessSlashingsReset(state)
	if err != nil {
		return nil, err
	}

	// Set RANDAO mix.
	state, err = ProcessRandaoMixesReset(state)
	if err != nil {
		return nil, err
	}

	// Set historical root accumulator.
	state, err = ProcessHistoricalDataUpdate(state)
	if err != nil {
		return nil, err
	}

	// Rotate current and previous epoch attestations.
	state, err = ProcessParticipationRecordUpdates(state)
	if err != nil {
		return nil, err
	}

	return state, nil
}
