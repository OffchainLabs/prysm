package electra

import (
	"context"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	customtypes "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/custom-types"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1/attestation"
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

	var participation customtypes.ReadOnlyParticipation
	if data.Target.Epoch == time.CurrentEpoch(st) {
		participation, err = st.CurrentEpochParticipationReadOnly()
	} else {
		participation, err = st.PreviousEpochParticipationReadOnly()
	}
	if err != nil {
		return 0, err
	}

	cfg := params.BeaconConfig()
	var rewardNumerator uint64
	for _, index := range indices {
		if index >= uint64(participation.Len()) {
			return 0, fmt.Errorf("index %d exceeds participation length %d", index, participation.Len())
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
			if flags[entry.flagIndex] { // If set, the validator voted correctly for the attestation given flag index.
				hasVoted, err := altair.HasValidatorFlag(participation.At(index), entry.flagIndex)
				if err != nil {
					return 0, err
				}
				if !hasVoted { // If set, the validator has already voted in the beacon state so we don't double count.
					rewardNumerator += br * entry.weight
				}
			}
		}
	}

	return rewardNumerator, nil
}
