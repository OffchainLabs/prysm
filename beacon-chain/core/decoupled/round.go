package decoupled

import (
	"bytes"
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Spec: get_current_round
func CurrentRound(st state.ReadOnlyBeaconState) primitives.Round {
	return slots.ToRound(st.Slot())
}

// Spec: get_previous_round
func PreviousRound(st state.ReadOnlyBeaconState) primitives.Round {
	current := CurrentRound(st)
	if current == params.BeaconConfig().GenesisRound {
		return current
	}
	return primitives.Round(uint64(current) - 1)
}

// Spec: the round boundary test in process_slots, the next slot starts a new round.
func IsRoundBoundary(slot primitives.Slot) bool {
	return slots.ToRound(slot+1) > slots.ToRound(slot)
}

// Spec: the fork epoch resolution shared by is_attestation_from_active_simplex_fork and is_round_from_active_simplex_fork.
func decoupledForkEpoch(st state.ReadOnlyBeaconState) primitives.Epoch {
	cfg := params.BeaconConfig()
	if bytes.Equal(st.Fork().CurrentVersion, cfg.DecoupledForkVersion) {
		return st.Fork().Epoch
	}
	return cfg.DecoupledForkEpoch
}

// Spec: is_attestation_from_active_simplex_fork
func isAttestationFromActiveDecoupledFork(st state.ReadOnlyBeaconState, data *ethpb.AttestationDataDecoupled) bool {
	return slots.ToEpoch(data.Slot) >= decoupledForkEpoch(st)
}

// Spec: is_round_from_active_simplex_fork
func isRoundFromActiveDecoupledFork(st state.ReadOnlyBeaconState, round primitives.Round) (bool, error) {
	epoch, err := slots.EpochAtRound(round)
	if err != nil {
		return false, err
	}
	return epoch >= decoupledForkEpoch(st), nil
}

// Spec: process_round from the old spec. The PDF has no per-round processing.
func ProcessRound(ctx context.Context, st state.BeaconState) error {
	if err := ProcessRewardsAndPenalties(ctx, st); err != nil {
		return errors.Wrap(err, "could not process round rewards and penalties")
	}
	// Spec: process_participation_flag_updates. The round arrays sit on the same native fields as the epoch arrays.
	_, err := altair.ProcessParticipationFlagUpdates(st)
	return err
}
