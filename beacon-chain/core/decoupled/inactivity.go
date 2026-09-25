package decoupled

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// Spec: get_finality_delay. Mid-epoch finalization can put the finalized epoch past the previous epoch, hence the guard.
func FinalityDelay(st state.ReadOnlyBeaconState) (primitives.Epoch, error) {
	fc, err := st.FinalizedCheckpointDecoupled()
	if err != nil {
		return 0, err
	}
	prev := time.PrevEpoch(st)
	finalized := slots.ToEpoch(fc.Slot)
	if finalized > prev {
		return 0, nil
	}
	return prev - finalized, nil
}

// Spec: is_in_inactivity_leak
func IsInInactivityLeak(st state.ReadOnlyBeaconState) (bool, error) {
	delay, err := FinalityDelay(st)
	if err != nil {
		return false, err
	}
	return delay > params.BeaconConfig().MinEpochsToInactivityPenalty, nil
}

// Spec: get_round_eligible_validator_indices
func roundEligibleValidatorIndices(st state.ReadOnlyBeaconState, round primitives.Round) ([]primitives.ValidatorIndex, error) {
	epoch, err := slots.EpochAtRound(round)
	if err != nil {
		return nil, err
	}
	var out []primitives.ValidatorIndex
	for idx, val := range st.ValidatorsReadOnlySeq() {
		if helpers.IsActiveValidatorUsingTrie(val, epoch) || (val.Slashed() && epoch+1 < val.WithdrawableEpoch()) {
			out = append(out, idx)
		}
	}
	return out, nil
}
