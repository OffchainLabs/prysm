package decoupled

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// Spec: get_base_reward_at_epoch, factored so one active-balance sum serves every validator of a settlement.
func baseRewardPerIncrementAtEpoch(ctx context.Context, st state.ReadOnlyBeaconState, epoch primitives.Epoch) (perIncrement, activeBalance uint64, err error) {
	active, err := helpers.ActiveValidatorIndices(ctx, st, epoch)
	if err != nil {
		return 0, 0, err
	}
	activeBalance = helpers.TotalBalance(st, active)
	perIncrement, err = altair.BaseRewardPerIncrement(activeBalance)
	return perIncrement, activeBalance, err
}

func baseRewardFromIncrement(effectiveBalance, perIncrement uint64) uint64 {
	return effectiveBalance / params.BeaconConfig().EffectiveBalanceIncrement * perIncrement
}

// Spec: get_attestation_proposer_reward_denominator
func attestationProposerRewardDenominator(slot primitives.Slot) uint64 {
	cfg := params.BeaconConfig()
	base := (cfg.WeightDenominator - cfg.ProposerWeight) * cfg.WeightDenominator / cfg.ProposerWeight
	return base * slots.RoundsPerEpochAt(slot)
}

// Spec: get_unslashed_participating_indices for the previous round, the only round rewards ever settle.
func unslashedParticipatingIndices(st state.ReadOnlyBeaconState, flagIndex uint8, epoch primitives.Epoch) (map[primitives.ValidatorIndex]struct{}, uint64, error) {
	participation, err := st.PreviousEpochParticipation()
	if err != nil {
		return nil, 0, err
	}
	out := make(map[primitives.ValidatorIndex]struct{})
	var balance uint64
	for idx, val := range st.ValidatorsReadOnlySeq() {
		if uint64(idx) >= uint64(len(participation)) || val.Slashed() || !helpers.IsActiveValidatorUsingTrie(val, epoch) {
			continue
		}
		has, err := altair.HasValidatorFlag(participation[idx], flagIndex)
		if err != nil {
			return nil, 0, err
		}
		if has {
			out[idx] = struct{}{}
			balance += val.EffectiveBalance()
		}
	}
	return out, max(balance, params.BeaconConfig().EffectiveBalanceIncrement), nil
}

// Spec: get_flag_index_deltas
func flagIndexDeltas(ctx context.Context, st state.ReadOnlyBeaconState, flagIndex uint8, weight uint64) (rewards, penalties []uint64, err error) {
	cfg := params.BeaconConfig()
	n := st.NumValidators()
	rewards = make([]uint64, n)
	penalties = make([]uint64, n)
	settlementRound := PreviousRound(st)
	settlementEpoch, err := slots.EpochAtRound(settlementRound)
	if err != nil {
		return nil, nil, err
	}
	participating, participatingBalance, err := unslashedParticipatingIndices(st, flagIndex, settlementEpoch)
	if err != nil {
		return nil, nil, err
	}
	perIncrement, activeBalance, err := baseRewardPerIncrementAtEpoch(ctx, st, settlementEpoch)
	if err != nil {
		return nil, nil, err
	}
	increment := cfg.EffectiveBalanceIncrement
	participatingIncrements := participatingBalance / increment
	activeIncrements := activeBalance / increment
	roundStart, err := slots.RoundStart(settlementRound)
	if err != nil {
		return nil, nil, err
	}
	roundsPerEpoch := slots.RoundsPerEpochAt(roundStart)
	leak, err := IsInInactivityLeak(st)
	if err != nil {
		return nil, nil, err
	}
	eligible, err := roundEligibleValidatorIndices(st, settlementRound)
	if err != nil {
		return nil, nil, err
	}
	for _, index := range eligible {
		val, err := st.ValidatorAtIndexReadOnly(index)
		if err != nil {
			return nil, nil, err
		}
		baseReward := baseRewardFromIncrement(val.EffectiveBalance(), perIncrement)
		if _, ok := participating[index]; ok {
			if !leak {
				rewards[index] += baseReward * weight * participatingIncrements / (activeIncrements * cfg.WeightDenominator * roundsPerEpoch)
			}
			continue
		}
		penalties[index] += baseReward * weight / (cfg.WeightDenominator * roundsPerEpoch)
	}
	return rewards, penalties, nil
}

// Spec: process_rewards_and_penalties. TIMELY_HEAD has no flag delta, it is paid per seat in process_available_attestation.
func ProcessRewardsAndPenalties(ctx context.Context, st state.BeaconState) error {
	cfg := params.BeaconConfig()
	if CurrentRound(st) == cfg.GenesisRound {
		return nil
	}
	active, err := isRoundFromActiveDecoupledFork(st, PreviousRound(st))
	if err != nil {
		return err
	}
	if !active {
		return nil
	}
	balances := st.Balances()
	for _, f := range []struct {
		index  uint8
		weight uint64
	}{
		{cfg.TimelyFinalityTargetFlagIndex, cfg.TimelySourceWeight},
		{cfg.TimelyTargetFlagIndex, cfg.TimelyTargetWeight},
	} {
		rewards, penalties, err := flagIndexDeltas(ctx, st, f.index, f.weight)
		if err != nil {
			return err
		}
		for i := range balances {
			balances[i], err = helpers.IncreaseBalanceWithVal(balances[i], rewards[i])
			if err != nil {
				return err
			}
			balances[i] = helpers.DecreaseBalanceWithVal(balances[i], penalties[i])
		}
	}
	return st.SetBalances(balances)
}
