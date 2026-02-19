// Package precompute provides gathering of nicely-structured
// data important to feed into epoch processing, such as attesting
// records and balances, for faster computation.
package precompute

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/pkg/errors"
)

// New gets called at the beginning of process epoch cycle to return
// pre computed instances of validators attesting records and total
// balances attested in an epoch.
func New(ctx context.Context, s state.BeaconState) ([]Validator, *Balance, error) {
	_, span := trace.StartSpan(ctx, "precomputeEpoch.New")
	defer span.End()

	pValidators := make([]Validator, s.NumValidators())
	pBal := &Balance{}

	currentEpoch := time.CurrentEpoch(s)
	prevEpoch := time.PrevEpoch(s)
	farFutureSlot := params.BeaconConfig().FarFutureSlot

	if err := s.ForEachValidator(func(idx int, val *stateutil.CompactValidator) error {
		// Was validator withdrawable or slashed
		withdrawable := prevEpoch+1 >= val.WithdrawableEpoch
		pValidators[idx] = Validator{
			IsSlashed:                    val.Slashed,
			IsWithdrawableCurrentEpoch:   withdrawable,
			CurrentEpochEffectiveBalance: val.EffectiveBalance,
			InclusionSlot:                farFutureSlot,
			InclusionDistance:             farFutureSlot,
		}
		// Was validator active current epoch
		if helpers.IsActiveCompactValidator(val, currentEpoch) {
			pValidators[idx].IsActiveCurrentEpoch = true
			pBal.ActiveCurrentEpoch += val.EffectiveBalance
		}
		// Was validator active previous epoch
		if helpers.IsActiveCompactValidator(val, prevEpoch) {
			pValidators[idx].IsActivePrevEpoch = true
			pBal.ActivePrevEpoch += val.EffectiveBalance
		}
		return nil
	}); err != nil {
		return nil, nil, errors.Wrap(err, "failed to initialize precompute")
	}
	return pValidators, pBal, nil
}
