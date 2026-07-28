// Package issuance implements the tapered issuance burn draft EIP.
package issuance

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	coreTime "github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/math"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Elevated during the transition to cushion the yield reduction, today's factor otherwise.
func BaseRewardFactorAtEpoch(epoch primitives.Epoch) uint64 {
	cfg := params.BeaconConfig()
	if cfg.TransitionStartEpoch == cfg.FarFutureEpoch || epoch < cfg.TransitionStartEpoch {
		return cfg.BaseRewardFactor
	}
	elapsed := epoch - cfg.TransitionStartEpoch
	if elapsed >= cfg.TransitionDurationEpochs {
		return cfg.BaseRewardFactor
	}
	remaining := uint64(cfg.TransitionDurationEpochs - elapsed)
	boost := cfg.TransitionBaseRewardFactor - cfg.BaseRewardFactor
	return cfg.BaseRewardFactor + boost*remaining/uint64(cfg.TransitionDurationEpochs)
}

// ProcessBurn must run after rewards and penalties and before effective balance updates.
func ProcessBurn(ctx context.Context, st state.BeaconState) error {
	cfg := params.BeaconConfig()
	epoch := coreTime.CurrentEpoch(st)
	if cfg.TransitionStartEpoch == cfg.FarFutureEpoch || epoch < cfg.TransitionStartEpoch {
		return nil
	}
	if cfg.SaturationBalance == 0 {
		return errors.New("saturation balance is 0")
	}
	active, err := helpers.TotalActiveBalance(ctx, st)
	if err != nil {
		return err
	}
	sqrtSat := math.IntegerSquareRoot(cfg.SaturationBalance)
	// Clamping to sqrtSat keeps the burn fraction at or below 1.
	sqrtActive := min(math.IntegerSquareRoot(active), sqrtSat)
	increment := cfg.EffectiveBalanceIncrement
	base := increment * BaseRewardFactorAtEpoch(epoch) / math.CachedSquareRoot(active)
	bals := st.Balances()

	attWeight := cfg.TimelySourceWeight + cfg.TimelyTargetWeight + cfg.TimelyHeadWeight
	for idx, val := range st.ValidatorsReadOnlySeq() {
		if !helpers.IsActiveValidatorUsingTrie(val, epoch) {
			continue
		}
		reward := val.EffectiveBalance() / increment * base * attWeight / cfg.WeightDenominator
		bals[idx] = helpers.DecreaseBalanceWithVal(bals[idx], burn(reward, sqrtActive, sqrtSat))
	}

	idealProposerReward := active / increment * base * cfg.ProposerWeight / cfg.WeightDenominator / uint64(cfg.SlotsPerEpoch)
	proposerBurn := burn(idealProposerReward, sqrtActive, sqrtSat)
	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return err
	}
	for slot := startSlot; slot < startSlot+cfg.SlotsPerEpoch; slot++ {
		proposer, err := helpers.BeaconProposerIndexAtSlot(ctx, st, slot)
		if err != nil {
			return err
		}
		bals[proposer] = helpers.DecreaseBalanceWithVal(bals[proposer], proposerBurn)
	}

	committee, err := st.CurrentSyncCommittee()
	if err != nil {
		return err
	}
	totalBaseRewards := base * (active / increment)
	participantReward := totalBaseRewards * cfg.SyncRewardWeight / cfg.WeightDenominator / uint64(cfg.SlotsPerEpoch) / cfg.SyncCommitteeSize
	// Uniform per member unlike the draft's balance-scaled formula, so the aggregate burn matches the taper.
	syncBurn := burn(participantReward*uint64(cfg.SlotsPerEpoch), sqrtActive, sqrtSat)
	for _, pubkey := range committee.Pubkeys {
		idx, ok := st.ValidatorIndexByPubkey(bytesutil.ToBytes48(pubkey))
		if !ok {
			return errors.New("sync committee pubkey not found in state")
		}
		bals[idx] = helpers.DecreaseBalanceWithVal(bals[idx], syncBurn)
	}

	return st.SetBalances(bals)
}

// Three successive multiply-divides keep every intermediate value within uint64.
func burn(reward, sqrtActive, sqrtSat uint64) uint64 {
	b := reward
	for range 3 {
		b = b * sqrtActive / sqrtSat
	}
	return b
}
