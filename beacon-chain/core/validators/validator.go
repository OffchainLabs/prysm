// Package validators contains libraries to shuffle validators
// and retrieve active validator indices from a given slot
// or an attestation. It also provides helper functions to locate
// validator based on pubic key.
package validators

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/math"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

// ExitInfo provides information about validator exits in the state.
type ExitInfo struct {
	HighestExitEpoch   primitives.Epoch
	Churn              uint64
	TotalActiveBalance uint64
}

// ErrValidatorAlreadyExited is an error raised when trying to process an exit of
// an already exited validator
var ErrValidatorAlreadyExited = errors.New("validator already exited")

// ExitInformation returns information about validator exits.
func ExitInformation(s state.BeaconState) *ExitInfo {
	exitInfo := &ExitInfo{}

	farFutureEpoch := params.BeaconConfig().FarFutureEpoch
	currentEpoch := slots.ToEpoch(s.Slot())
	totalActiveBalance := uint64(0)

	err := s.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		e := val.ExitEpoch()
		if e != farFutureEpoch {
			if e > exitInfo.HighestExitEpoch {
				exitInfo.HighestExitEpoch = e
				exitInfo.Churn = 1
			} else if e == exitInfo.HighestExitEpoch {
				exitInfo.Churn++
			}
		}

		// Calculate total active balance in the same loop
		if helpers.IsActiveValidatorUsingTrie(val, currentEpoch) {
			totalActiveBalance += val.EffectiveBalance()
		}

		return nil
	})
	_ = err

	// Apply minimum balance as per spec
	exitInfo.TotalActiveBalance = max(params.BeaconConfig().EffectiveBalanceIncrement, totalActiveBalance)
	return exitInfo
}

// InitiateValidatorExit takes in validator index and updates
// validator with correct voluntary exit parameters.
// Note: As of Electra, the exitQueueEpoch and churn parameters are unused.
//
// Spec pseudocode definition:
//
//	def initiate_validator_exit(state: BeaconState, index: ValidatorIndex) -> None:
//	    """
//	    Initiate the exit of the validator with index ``index``.
//	    """
//	    # Return if validator already initiated exit
//	    validator = state.validators[index]
//	    if validator.exit_epoch != FAR_FUTURE_EPOCH:
//	        return
//
//	    # Compute exit queue epoch [Modified in Electra:EIP7251]
//	    exit_queue_epoch = compute_exit_epoch_and_update_churn(state, validator.effective_balance)
//
//	    # Set validator exit epoch and withdrawable epoch
//	    validator.exit_epoch = exit_queue_epoch
//	    validator.withdrawable_epoch = Epoch(validator.exit_epoch + MIN_VALIDATOR_WITHDRAWABILITY_DELAY)
func InitiateValidatorExit(
	ctx context.Context,
	s state.BeaconState,
	idx primitives.ValidatorIndex,
	exitInfo *ExitInfo,
) (state.BeaconState, error) {
	validator, err := s.ValidatorAtIndex(idx)
	if err != nil {
		return nil, err
	}
	if validator.ExitEpoch != params.BeaconConfig().FarFutureEpoch {
		return s, ErrValidatorAlreadyExited
	}
	if exitInfo == nil {
		return nil, errors.New("exit info is required to process validator exit")
	}
	// Compute exit queue epoch.
	if s.Version() < version.Electra {
		if err = initiateValidatorExitPreElectra(ctx, s, exitInfo); err != nil {
			return nil, err
		}
	} else {
		// [Modified in Electra:EIP7251]
		// exit_queue_epoch = compute_exit_epoch_and_update_churn(state, validator.effective_balance)
		var err error
		exitInfo.HighestExitEpoch, err = s.ExitEpochAndUpdateChurn(primitives.Gwei(validator.EffectiveBalance))
		if err != nil {
			return nil, err
		}
	}
	validator.ExitEpoch = exitInfo.HighestExitEpoch
	validator.WithdrawableEpoch, err = exitInfo.HighestExitEpoch.SafeAddEpoch(params.BeaconConfig().MinValidatorWithdrawabilityDelay)
	if err != nil {
		return nil, err
	}
	if err := s.UpdateValidatorAtIndex(idx, validator); err != nil {
		return nil, err
	}
	return s, nil
}

// InitiateValidatorExitForTotalBal has the same functionality as InitiateValidatorExit,
// the only difference being how total active balance is obtained. In InitiateValidatorExit
// it is calculated inside the function and in InitiateValidatorExitForTotalBal it's a
// function argument.
func InitiateValidatorExitForTotalBal(
	ctx context.Context,
	s state.BeaconState,
	idx primitives.ValidatorIndex,
	exitInfo *ExitInfo,
	totalActiveBalance primitives.Gwei,
) (state.BeaconState, error) {
	validator, err := s.ValidatorAtIndex(idx)
	if err != nil {
		return nil, err
	}
	if validator.ExitEpoch != params.BeaconConfig().FarFutureEpoch {
		return s, ErrValidatorAlreadyExited
	}

	// Compute exit queue epoch.
	if s.Version() < version.Electra {
		if err = initiateValidatorExitPreElectra(ctx, s, exitInfo); err != nil {
			return nil, err
		}
	} else {
		// [Modified in Electra:EIP7251]
		// exit_queue_epoch = compute_exit_epoch_and_update_churn(state, validator.effective_balance)
		var err error
		exitInfo.HighestExitEpoch, err = s.ExitEpochAndUpdateChurnForTotalBal(totalActiveBalance, primitives.Gwei(validator.EffectiveBalance))
		if err != nil {
			return nil, err
		}
	}
	validator.ExitEpoch = exitInfo.HighestExitEpoch
	validator.WithdrawableEpoch, err = exitInfo.HighestExitEpoch.SafeAddEpoch(params.BeaconConfig().MinValidatorWithdrawabilityDelay)
	if err != nil {
		return nil, err
	}
	if err := s.UpdateValidatorAtIndex(idx, validator); err != nil {
		return nil, err
	}
	return s, nil
}

func initiateValidatorExitPreElectra(ctx context.Context, s state.BeaconState, exitInfo *ExitInfo) error {
	// Relevant spec code from phase0:
	//
	//	exit_epochs = [v.exit_epoch for v in state.validators if v.exit_epoch != FAR_FUTURE_EPOCH]
	//	exit_queue_epoch = max(exit_epochs + [compute_activation_exit_epoch(get_current_epoch(state))])
	//	exit_queue_churn = len([v for v in state.validators if v.exit_epoch == exit_queue_epoch])
	//	if exit_queue_churn >= get_validator_churn_limit(state):
	//	    exit_queue_epoch += Epoch(1)
	exitableEpoch := helpers.ActivationExitEpoch(time.CurrentEpoch(s))
	if exitInfo == nil {
		return errors.New("exit info is required to process validator exit")
	}
	if exitableEpoch > exitInfo.HighestExitEpoch {
		exitInfo.HighestExitEpoch = exitableEpoch
		exitInfo.Churn = 0
	}
	activeValidatorCount, err := helpers.ActiveValidatorCount(ctx, s, time.CurrentEpoch(s))
	if err != nil {
		return errors.Wrap(err, "could not get active validator count")
	}
	currentChurn := helpers.ValidatorExitChurnLimit(activeValidatorCount)
	if exitInfo.Churn >= currentChurn {
		exitInfo.HighestExitEpoch, err = exitInfo.HighestExitEpoch.SafeAdd(1)
		if err != nil {
			return err
		}
		exitInfo.Churn = 1
	} else {
		exitInfo.Churn = exitInfo.Churn + 1
	}
	return nil
}

// SlashValidator slashes the malicious validator's balance and awards
// the whistleblower's balance. Note: This implementation does not handle an
// optional whistleblower index. The whistleblower index is always the proposer index.
//
// Spec pseudocode definition:
//
//	def slash_validator(state: BeaconState,
//	                  slashed_index: ValidatorIndex,
//	                  whistleblower_index: ValidatorIndex=None) -> None:
//	  """
//	  Slash the validator with index ``slashed_index``.
//	  """
//	  epoch = get_current_epoch(state)
//	  initiate_validator_exit(state, slashed_index)
//	  validator = state.validators[slashed_index]
//	  validator.slashed = True
//	  validator.withdrawable_epoch = max(validator.withdrawable_epoch, Epoch(epoch + EPOCHS_PER_SLASHINGS_VECTOR))
//	  state.slashings[epoch % EPOCHS_PER_SLASHINGS_VECTOR] += validator.effective_balance
//	  slashing_penalty = validator.effective_balance // MIN_SLASHING_PENALTY_QUOTIENT_EIP7251  # [Modified in EIP7251]
//	  decrease_balance(state, slashed_index, slashing_penalty)
//
//	  # Apply proposer and whistleblower rewards
//	  proposer_index = get_beacon_proposer_index(state)
//	  if whistleblower_index is None:
//	      whistleblower_index = proposer_index
//	  whistleblower_reward = Gwei(
//	       validator.effective_balance // WHISTLEBLOWER_REWARD_QUOTIENT_ELECTRA)  # [Modified in EIP7251]
//	  proposer_reward = Gwei(whistleblower_reward * PROPOSER_WEIGHT // WEIGHT_DENOMINATOR)
//	  increase_balance(state, proposer_index, proposer_reward)
//	  increase_balance(state, whistleblower_index, Gwei(whistleblower_reward - proposer_reward))
func SlashValidator(
	ctx context.Context,
	s state.BeaconState,
	slashedIdx primitives.ValidatorIndex,
	exitInfo *ExitInfo,
) (state.BeaconState, error) {
	var err error
	if exitInfo == nil {
		return nil, errors.New("exit info is required to slash validator")
	}
	s, err = InitiateValidatorExitForTotalBal(ctx, s, slashedIdx, exitInfo, primitives.Gwei(exitInfo.TotalActiveBalance))
	if err != nil && !errors.Is(err, ErrValidatorAlreadyExited) {
		return nil, errors.Wrapf(err, "could not initiate validator %d exit", slashedIdx)
	}
	currentEpoch := slots.ToEpoch(s.Slot())
	validator, err := s.ValidatorAtIndex(slashedIdx)
	if err != nil {
		return nil, err
	}
	validator.Slashed = true
	maxWithdrawableEpoch := primitives.MaxEpoch(validator.WithdrawableEpoch, currentEpoch+params.BeaconConfig().EpochsPerSlashingsVector)
	validator.WithdrawableEpoch = maxWithdrawableEpoch

	if err := s.UpdateValidatorAtIndex(slashedIdx, validator); err != nil {
		return nil, err
	}

	// The slashing amount is represented by epochs per slashing vector. The validator's effective balance is then applied to that amount.
	slashings := s.Slashings()
	currentSlashing := slashings[currentEpoch%params.BeaconConfig().EpochsPerSlashingsVector]
	if err := s.UpdateSlashingsAtIndex(
		uint64(currentEpoch%params.BeaconConfig().EpochsPerSlashingsVector),
		currentSlashing+validator.EffectiveBalance,
	); err != nil {
		return nil, err
	}

	slashingQuotient, proposerRewardQuotient, whistleblowerRewardQuotient, err := SlashingParamsPerVersion(s.Version())
	if err != nil {
		return nil, errors.Wrap(err, "could not get slashing parameters per version")
	}

	slashingPenalty, err := math.Div64(validator.EffectiveBalance, slashingQuotient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute slashing slashingPenalty")
	}
	if err := helpers.DecreaseBalance(s, slashedIdx, slashingPenalty); err != nil {
		return nil, err
	}

	proposerIdx, err := helpers.BeaconProposerIndex(ctx, s)
	if err != nil {
		return nil, errors.Wrap(err, "could not get proposer idx")
	}
	whistleBlowerIdx := proposerIdx
	whistleblowerReward, err := math.Div64(validator.EffectiveBalance, whistleblowerRewardQuotient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute whistleblowerReward")
	}
	proposerReward, err := math.Div64(whistleblowerReward, proposerRewardQuotient)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute proposer reward")
	}
	if err := helpers.IncreaseBalance(s, proposerIdx, proposerReward); err != nil {
		return nil, err
	}
	if err := helpers.IncreaseBalance(s, whistleBlowerIdx, whistleblowerReward-proposerReward); err != nil {
		return nil, err
	}
	return s, nil
}

// ActivatedValidatorIndices determines the indices activated during the given epoch.
func ActivatedValidatorIndices(epoch primitives.Epoch, validators []*ethpb.Validator) []primitives.ValidatorIndex {
	activations := make([]primitives.ValidatorIndex, 0)
	for i := 0; i < len(validators); i++ {
		val := validators[i]
		if val.ActivationEpoch <= epoch && epoch < val.ExitEpoch {
			activations = append(activations, primitives.ValidatorIndex(i))
		}
	}
	return activations
}

// SlashedValidatorIndices determines the indices slashed during the given epoch.
func SlashedValidatorIndices(epoch primitives.Epoch, validators []*ethpb.Validator) []primitives.ValidatorIndex {
	slashed := make([]primitives.ValidatorIndex, 0)
	for i := 0; i < len(validators); i++ {
		val := validators[i]
		maxWithdrawableEpoch := primitives.MaxEpoch(val.WithdrawableEpoch, epoch+params.BeaconConfig().EpochsPerSlashingsVector)
		if val.WithdrawableEpoch == maxWithdrawableEpoch && val.Slashed {
			slashed = append(slashed, primitives.ValidatorIndex(i))
		}
	}
	return slashed
}

// ExitedValidatorIndices returns the indices of validators who exited during the specified epoch.
//
// A validator is considered to have exited during an epoch if their ExitEpoch equals the epoch and
// excludes validators that have been ejected.
// This function simplifies the exit determination by directly checking the validator's ExitEpoch,
// avoiding the complexities and potential inaccuracies of calculating withdrawable epochs.
func ExitedValidatorIndices(epoch primitives.Epoch, validators []*ethpb.Validator) ([]primitives.ValidatorIndex, error) {
	exited := make([]primitives.ValidatorIndex, 0)
	for i, val := range validators {
		if val.ExitEpoch == epoch && val.EffectiveBalance > params.BeaconConfig().EjectionBalance {
			exited = append(exited, primitives.ValidatorIndex(i))
		}
	}
	return exited, nil
}

// EjectedValidatorIndices returns the indices of validators who were ejected during the specified epoch.
//
// A validator is considered ejected during an epoch if:
// - Their ExitEpoch equals the epoch.
// - Their EffectiveBalance is less than or equal to the EjectionBalance threshold.
//
// This function simplifies the ejection determination by directly checking the validator's ExitEpoch
// and EffectiveBalance, avoiding the complexities and potential inaccuracies of calculating
// withdrawable epochs.
func EjectedValidatorIndices(epoch primitives.Epoch, validators []*ethpb.Validator) ([]primitives.ValidatorIndex, error) {
	ejected := make([]primitives.ValidatorIndex, 0)
	for i, val := range validators {
		if val.ExitEpoch == epoch && val.EffectiveBalance <= params.BeaconConfig().EjectionBalance {
			ejected = append(ejected, primitives.ValidatorIndex(i))
		}
	}
	return ejected, nil
}
