package electra

import (
	"context"
	"fmt"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/altair"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
)

var (
	ProcessAttestationsNoVerifySignature = altair.ProcessAttestationsNoVerifySignature
)

// GetProposerRewardNumerator returns the numerator of the proposer reward for an attestation.
func GetProposerRewardNumerator(
	ctx context.Context,
	st state.ReadOnlyBeaconState,
	att ethpb.Att,
	totalBalance uint64,
) (uint64, error) {
	data := att.GetData()

	delay, err := st.Slot().SafeSubSlot(data.Slot)
	if err != nil {
		return 0, fmt.Errorf("attestation slot %d exceeds state slot %d", data.Slot, st.Slot())
	}

	flags, err := altair.AttestationParticipationFlagIndices(st, data, delay)
	if err != nil {
		return 0, err
	}

	committees, err := helpers.AttestationCommitteesFromState(ctx, st, att)
	if err != nil {
		return 0, err
	}

	indices, err := attestation.AttestingIndices(att, committees...)
	if err != nil {
		return 0, err
	}

	var participation []byte
	if data.Target.Epoch == time.CurrentEpoch(st) {
		participation, err = st.CurrentEpochParticipationNoCopy()
	} else {
		participation, err = st.PreviousEpochParticipationNoCopy()
	}
	if err != nil {
		return 0, err
	}

	cfg := params.BeaconConfig()
	var rewardNumerator uint64
	for _, index := range indices {
		if index >= uint64(len(participation)) {
			return 0, fmt.Errorf("index %d exceeds participation length %d", index, len(participation))
		}

		br, err := altair.BaseRewardWithTotalBalance(st, primitives.ValidatorIndex(index), totalBalance)
		if err != nil {
			return 0, err
		}

		for _, entry := range []struct {
			flagIndex uint8
			weight    uint64
		}{
			{cfg.TimelySourceFlagIndex, cfg.TimelySourceWeight},
			{cfg.TimelyTargetFlagIndex, cfg.TimelyTargetWeight},
			{cfg.TimelyHeadFlagIndex, cfg.TimelyHeadWeight},
		} {
			if flags[entry.flagIndex] {
				has, err := altair.HasValidatorFlag(participation[index], entry.flagIndex)
				if err != nil {
					return 0, err
				}
				if !has {
					rewardNumerator += br * entry.weight
				}
			}
		}
	}

	return rewardNumerator, nil
}
