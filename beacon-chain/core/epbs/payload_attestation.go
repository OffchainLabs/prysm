package epbs

import (
	"bytes"
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/altair"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

func ProcessPayloadAttestations(state state.BeaconState, body interfaces.ReadOnlyBeaconBlockBody) error {
	atts, err := body.PayloadAttestations()
	if err != nil {
		return err
	}
	if len(atts) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	lbh := state.LatestBlockHeader()
	proposerIndex := lbh.ProposerIndex
	var participation []byte
	if state.Slot()%32 == 0 {
		participation, err = state.PreviousEpochParticipation()
	} else {
		participation, err = state.CurrentEpochParticipation()
	}
	if err != nil {
		return err
	}
	totalBalance, err := helpers.TotalActiveBalance(state)
	if err != nil {
		return err
	}
	baseReward, err := altair.BaseRewardWithTotalBalance(state, proposerIndex, totalBalance)
	if err != nil {
		return err
	}
	lfs, err := state.LatestFullSlot()
	if err != nil {
		return err
	}

	cfg := params.BeaconConfig()
	sourceFlagIndex := cfg.TimelySourceFlagIndex
	targetFlagIndex := cfg.TimelyTargetFlagIndex
	headFlagIndex := cfg.TimelyHeadFlagIndex
	penaltyNumerator := uint64(0)
	rewardNumerator := uint64(0)
	rewardDenominator := (cfg.WeightDenominator - cfg.ProposerWeight) * cfg.WeightDenominator / cfg.ProposerWeight

	for _, att := range atts {
		data := att.Data
		if !bytes.Equal(data.BeaconBlockRoot, lbh.ParentRoot) {
			return errors.New("invalid beacon block root in payload attestation data")
		}
		if data.Slot+1 != state.Slot() {
			return errors.New("invalid data slot")
		}
		indexed, err := helpers.GetIndexedPayloadAttestation(ctx, state, data.Slot, att)
		if err != nil {
			return err
		}
		valid, err := helpers.IsValidIndexedPayloadAttestation(state, indexed)
		if err != nil {
			return err
		}
		if !valid {
			return errors.New("invalid payload attestation")
		}
		payloadWasPreset := data.Slot == lfs
		votedPresent := data.PayloadStatus == primitives.PAYLOAD_PRESENT
		if votedPresent != payloadWasPreset {
			for _, idx := range indexed.GetAttestingIndices() {
				flags := participation[idx]
				has, err := altair.HasValidatorFlag(flags, targetFlagIndex)
				if err != nil {
					return err
				}
				if has {
					penaltyNumerator += baseReward * cfg.TimelyTargetWeight
				}
				has, err = altair.HasValidatorFlag(flags, sourceFlagIndex)
				if err != nil {
					return err
				}
				if has {
					penaltyNumerator += baseReward * cfg.TimelySourceWeight
				}
				has, err = altair.HasValidatorFlag(flags, headFlagIndex)
				if err != nil {
					return err
				}
				if has {
					penaltyNumerator += baseReward * cfg.TimelyHeadWeight
				}
				participation[idx] = 0
			}
		} else {
			for _, idx := range indexed.GetAttestingIndices() {
				participation[idx] = (1 << headFlagIndex) | (1 << sourceFlagIndex) | (1 << targetFlagIndex)
				rewardNumerator += baseReward * (cfg.TimelyHeadWeight + cfg.TimelySourceWeight + cfg.TimelyTargetWeight)
			}
		}
	}
	if penaltyNumerator > 0 {
		if err := helpers.DecreaseBalance(state, proposerIndex, penaltyNumerator/rewardDenominator); err != nil {
			return err
		}
	}
	if rewardNumerator > 0 {
		if err := helpers.IncreaseBalance(state, proposerIndex, penaltyNumerator/rewardDenominator); err != nil {
			return err
		}
	}
	return nil
}
