package precompute

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
)

// ProcessSlashingsPrecompute processes the slashed validators during epoch processing.
// This is an optimized version by passing in precomputed total epoch balances.
func ProcessSlashingsPrecompute(s state.BeaconState, pBal *Balance) error {
	currentEpoch := time.CurrentEpoch(s)
	exitLength := params.BeaconConfig().EpochsPerSlashingsVector

	// Compute the sum of state slashings
	slashings := s.Slashings()
	totalSlashing := uint64(0)
	for _, slashing := range slashings {
		totalSlashing += slashing
	}

	minSlashing := min(totalSlashing*params.BeaconConfig().ProportionalSlashingMultiplier, pBal.ActiveCurrentEpoch)
	epochToWithdraw := currentEpoch + exitLength/2

	// Iterate through validator list in state, stop as soon as a validator satisfies the slashing condition of current epoch.
	hasSlashing := false
	for _, val := range s.ValidatorsReadOnlySeq() {
		correctEpoch := epochToWithdraw == val.WithdrawableEpoch()
		if val.Slashed() && correctEpoch {
			hasSlashing = true
			break
		}
	}

	// Exit early if there's no meaningful slashing to process.
	if !hasSlashing {
		return nil
	}

	increment := params.BeaconConfig().EffectiveBalanceIncrement
	bals := s.Balances()
	for idx, val := range s.ValidatorsReadOnlySeq() {
		correctEpoch := epochToWithdraw == val.WithdrawableEpoch()
		if val.Slashed() && correctEpoch {
			penaltyNumerator := val.EffectiveBalance() / increment * minSlashing
			penalty := penaltyNumerator / pBal.ActiveCurrentEpoch * increment
			bals[idx] = helpers.DecreaseBalanceWithVal(bals[idx], penalty)
		}
	}
	return s.SetBalances(bals)
}
