package altair

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/math"
	"github.com/pkg/errors"
)

// BaseReward returns the base reward of the validator at the given index, priced at the slot duration in effect at the given epoch.
// https://github.com/ethereum/consensus-specs/blob/master/specs/heze/beacon-chain.md#modified-get_base_reward
func BaseReward(ctx context.Context, s state.ReadOnlyBeaconState, index primitives.ValidatorIndex, epoch primitives.Epoch) (uint64, error) {
	totalBalance, err := helpers.TotalActiveBalance(ctx, s)
	if err != nil {
		return 0, fmt.Errorf("total active balance: %w", err)
	}

	baseReward, err := BaseRewardWithTotalBalance(s, index, totalBalance, epoch)
	if err != nil {
		return 0, fmt.Errorf("base reward with total balance: %w", err)
	}

	return baseReward, nil
}

// BaseRewardWithTotalBalance is BaseReward with the total active balance provided by the caller.
func BaseRewardWithTotalBalance(s state.ReadOnlyBeaconState, index primitives.ValidatorIndex, totalBalance uint64, epoch primitives.Epoch) (uint64, error) {
	val, err := s.ValidatorAtIndexReadOnly(index)
	if err != nil {
		return 0, fmt.Errorf("validator at index read only: %w", err)
	}

	cfg := params.BeaconConfig()
	increments := val.EffectiveBalance() / cfg.EffectiveBalanceIncrement

	baseRewardPerInc, err := BaseRewardPerIncrement(totalBalance, epoch)
	if err != nil {
		return 0, fmt.Errorf("base reward per increment: %w", err)
	}

	return increments * baseRewardPerInc, nil
}

// BaseRewardPerIncrement returns the base reward per increment, priced at the slot duration in
// effect at “epoch“.
// https://github.com/ethereum/consensus-specs/blob/master/specs/heze/beacon-chain.md#modified-get_base_reward_per_increment
func BaseRewardPerIncrement(activeBalance uint64, epoch primitives.Epoch) (uint64, error) {
	if activeBalance == 0 {
		return 0, errors.New("active balance can't be 0")
	}

	cfg := params.BeaconConfig()
	scaled := cfg.EffectiveBalanceIncrement * cfg.BaseRewardFactor * cfg.SlotDurationMillisAtEpoch(epoch) / cfg.SlotDurationMillisAtEpoch(cfg.GenesisEpoch)
	return scaled / math.CachedSquareRoot(activeBalance), nil
}
