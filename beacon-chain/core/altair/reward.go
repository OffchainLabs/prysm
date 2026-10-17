package altair

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/math"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// BaseReward takes state and validator index and calculate
// individual validator's base reward.
//
// Spec code:
//
//	def get_base_reward(state: BeaconState, index: ValidatorIndex) -> Gwei:
//	  """
//	  Return the base reward for the validator defined by ``index`` with respect to the current ``state``.
//
//	  Note: An optimally performing validator can earn one base reward per epoch over a long time horizon.
//	  This takes into account both per-epoch (e.g. attestation) and intermittent duties (e.g. block proposal
//	  and sync committees).
//	  """
//	  increments = state.validators[index].effective_balance // EFFECTIVE_BALANCE_INCREMENT
//	  return Gwei(increments * get_base_reward_per_increment(state))
func BaseReward(ctx context.Context, s state.ReadOnlyBeaconState, index primitives.ValidatorIndex) (uint64, error) {
	totalBalance, err := helpers.TotalActiveBalance(ctx, s)
	if err != nil {
		return 0, errors.Wrap(err, "could not calculate active balance")
	}
	return BaseRewardWithTotalBalance(s, index, totalBalance, slots.ToEpoch(s.Slot()))
}

// BaseRewardWithTotalBalance calculates the base reward with the provided total balance, priced at the slot duration of the given epoch.
//
// Spec code:
//
//	def get_base_reward_at_epoch(state: BeaconState, index: ValidatorIndex, epoch: Epoch) -> Gwei:
//	    increments = state.validators[index].effective_balance // EFFECTIVE_BALANCE_INCREMENT
//	    return increments * get_base_reward_per_increment_at_epoch(state, epoch)
func BaseRewardWithTotalBalance(s state.ReadOnlyBeaconState, index primitives.ValidatorIndex, totalBalance uint64, epoch primitives.Epoch) (uint64, error) {
	val, err := s.ValidatorAtIndexReadOnly(index)
	if err != nil {
		return 0, err
	}
	cfg := params.BeaconConfig()
	increments := val.EffectiveBalance() / cfg.EffectiveBalanceIncrement
	baseRewardPerInc, err := BaseRewardPerIncrement(totalBalance, epoch)
	if err != nil {
		return 0, err
	}
	return increments * baseRewardPerInc, nil
}

// BaseRewardPerIncrement of the beacon state, priced at the slot duration of the given epoch.
//
// Spec code:
//
//	def get_base_reward_per_increment_at_epoch(state: BeaconState, epoch: Epoch) -> Gwei:
//	    return Gwei(
//	        EFFECTIVE_BALANCE_INCREMENT
//	        * BASE_REWARD_FACTOR
//	        * get_slot_duration_ms(epoch)
//	        // get_slot_duration_ms(GENESIS_EPOCH)
//	        // integer_squareroot(get_total_active_balance(state))
//	    )
func BaseRewardPerIncrement(activeBalance uint64, epoch primitives.Epoch) (uint64, error) {
	if activeBalance == 0 {
		return 0, errors.New("active balance can't be 0")
	}
	cfg := params.BeaconConfig()
	scaled := cfg.EffectiveBalanceIncrement * cfg.BaseRewardFactor * cfg.SlotDurationMillisAtEpoch(epoch) / cfg.SlotDurationMillis()
	return scaled / math.CachedSquareRoot(activeBalance), nil
}
