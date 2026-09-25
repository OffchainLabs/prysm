package decoupled

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// Spec: get_committee_count_per_slot, spreading the active set across the round's slots.
func committeeCountPerSlot(ctx context.Context, st state.ReadOnlyBeaconState, epoch primitives.Epoch) (uint64, error) {
	cfg := params.BeaconConfig()
	active, err := helpers.ActiveValidatorCount(ctx, st, epoch)
	if err != nil {
		return 0, err
	}
	start, err := slots.EpochStart(epoch)
	if err != nil {
		return 0, err
	}
	count := active / slots.SlotsPerRoundAt(start) / cfg.TargetCommitteeSize
	return max(1, min(cfg.MaxCommitteesPerSlot, count)), nil
}

// Spec: get_beacon_committee. With the empty round schedule it equals phase0's, so the cached helper serves it.
func committeeAt(ctx context.Context, st state.ReadOnlyBeaconState, slot primitives.Slot, index primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error) {
	cfg := params.BeaconConfig()
	epoch := slots.ToEpoch(slot)
	start, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}
	perRound := slots.SlotsPerRoundAt(start)
	if perRound == uint64(cfg.SlotsPerEpoch) {
		return helpers.BeaconCommitteeFromState(ctx, st, slot, index)
	}
	count, err := committeeCountPerSlot(ctx, st, epoch)
	if err != nil {
		return nil, err
	}
	roundStart, err := slots.RoundStart(slots.ToRound(slot))
	if err != nil {
		return nil, err
	}
	slotInRound := uint64(slot - roundStart)
	seed, err := helpers.Seed(st, epoch, cfg.DomainBeaconAttester)
	if err != nil {
		return nil, err
	}
	active, err := helpers.ActiveValidatorIndices(ctx, st, epoch)
	if err != nil {
		return nil, err
	}
	return helpers.ComputeCommittee(active, seed, slotInRound*count+uint64(index), count*perRound)
}

// Spec: the committee walk in validate_attestation
func attestationCommittees(ctx context.Context, st state.ReadOnlyBeaconState, att *ethpb.AttestationDecoupled) ([][]primitives.ValidatorIndex, error) {
	epoch := slots.ToEpoch(att.Data.Slot)
	count, err := committeeCountPerSlot(ctx, st, epoch)
	if err != nil {
		return nil, err
	}
	indices := helpers.CommitteeIndices(att.CommitteeBits)
	committees := make([][]primitives.ValidatorIndex, 0, len(indices))
	for _, ci := range indices {
		if uint64(ci) >= count {
			return nil, fmt.Errorf("committee index %d exceeds committee count %d", ci, count)
		}
		c, err := committeeAt(ctx, st, att.Data.Slot, ci)
		if err != nil {
			return nil, err
		}
		committees = append(committees, c)
	}
	return committees, nil
}

// Spec: get_attesting_indices (electra), written for the Decoupled type.
func attestingIndices(aggBits bitfield.Bitlist, committees [][]primitives.ValidatorIndex) ([]uint64, error) {
	total := 0
	for _, c := range committees {
		total += len(c)
	}
	if aggBits.Len() != uint64(total) {
		return nil, fmt.Errorf("aggregation bits length %d does not match committee length %d", aggBits.Len(), total)
	}
	attesters := make([]uint64, 0, aggBits.Count())
	offset := 0
	for ci, c := range committees {
		found := false
		for i, vi := range c {
			if aggBits.BitAt(uint64(offset + i)) {
				attesters = append(attesters, uint64(vi))
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("no attesting indices for committee %d", ci)
		}
		offset += len(c)
	}
	slices.Sort(attesters)
	return slices.Compact(attesters), nil
}

// Spec: the PDF only rejects attestations from a future round (on_block, PDF §7). The inclusion delay, the η_SG window and the committee checks are Prysm's.
func ValidateAttestationNoVerifySignature(ctx context.Context, st state.ReadOnlyBeaconState, att *ethpb.AttestationDecoupled) ([]uint64, error) {
	cfg := params.BeaconConfig()
	data := att.Data
	if data == nil {
		return nil, errors.New("nil attestation data")
	}
	if !isAttestationFromActiveDecoupledFork(st, data) {
		return nil, errors.New("attestation claims a duty before the Decoupled fork")
	}
	if data.Slot+cfg.MinAttestationInclusionDelay > st.Slot() {
		return nil, fmt.Errorf("attestation slot %d too recent for state slot %d", data.Slot, st.Slot())
	}
	round := slots.ToRound(data.Slot)
	if uint64(round)+cfg.SGWindowRounds < uint64(CurrentRound(st)) {
		return nil, fmt.Errorf("attestation round %d outside the window of round %d", round, CurrentRound(st))
	}
	committees, err := attestationCommittees(ctx, st, att)
	if err != nil {
		return nil, err
	}
	indices, err := attestingIndices(att.AggregationBits, committees)
	if err != nil {
		return nil, err
	}
	return indices, validIndices(st, indices)
}

// Spec: process_attestation over B.attestations, PDF §4.
func ProcessAttestationsNoVerifySignature(ctx context.Context, st state.BeaconState, atts []*ethpb.AttestationDecoupled) (state.BeaconState, error) {
	for _, att := range atts {
		if err := ProcessAttestationNoVerifySignature(ctx, st, att); err != nil {
			return nil, err
		}
	}
	return st, nil
}

// Spec: a's height pair is (σ.h, σ.T_h, τ), PDF §4. A timeout counts only when it names the chain's own entry.
func heightPairCounts(st state.ReadOnlyBeaconState, data *ethpb.AttestationDataDecoupled) (bool, error) {
	height, err := st.CurrentHeight()
	if err != nil || data.Height != height {
		return false, err
	}
	entry, err := st.CurrentHeightTarget()
	if err != nil {
		return false, err
	}
	return bytes.Equal(data.Target.GetRoot(), entry.Root), nil
}

// Spec: a's finality pair = (σ.h_j, σ.J), PDF §4.
func finalityPairMatches(st state.ReadOnlyBeaconState, data *ethpb.AttestationDataDecoupled) (bool, error) {
	justifiedHeight, err := st.JustifiedHeight()
	if err != nil || isEmptyCheckpoint(data.FinalityTarget) || data.FinalityHeight != justifiedHeight {
		return false, err
	}
	justified, err := st.JustifiedCheckpointDecoupled()
	if err != nil {
		return false, err
	}
	return bytes.Equal(data.FinalityTarget.GetRoot(), justified.Root), nil
}

// State copies share these bitlists, so writes go to a copy, grown to cover validators activated since the last reset.
func writableBitlist(b bitfield.Bitlist, n uint64) bitfield.Bitlist {
	if b.Len() >= n {
		return bitfield.Bitlist(bytesutil.SafeCopyBytes(b))
	}
	out := bitfield.NewBitlist(n)
	for _, i := range b.BitIndices() {
		out.SetBitAt(uint64(i), true)
	}
	return out
}

// Spec: process_attestation(σ, a), PDF §4, plus the old spec's participation flags and proposer reward.
func ProcessAttestationNoVerifySignature(ctx context.Context, st state.BeaconState, att *ethpb.AttestationDecoupled) error {
	cfg := params.BeaconConfig()
	indices, err := ValidateAttestationNoVerifySignature(ctx, st, att)
	if err != nil {
		return err
	}
	data := att.Data
	heightCounts, err := heightPairCounts(st, data)
	if err != nil {
		return err
	}
	finalityMatches, err := finalityPairMatches(st, data)
	if err != nil {
		return err
	}
	justifiedHeight, err := st.JustifiedHeight()
	if err != nil {
		return err
	}
	finalizedHeight, err := st.FinalizedHeight()
	if err != nil {
		return err
	}
	finalityCounts := finalityMatches && justifiedHeight > finalizedHeight

	var modify func(func([]byte) ([]byte, error)) error
	switch slots.ToRound(data.Slot) {
	case CurrentRound(st):
		modify = st.ModifyCurrentParticipationBits
	case PreviousRound(st):
		modify = st.ModifyPreviousParticipationBits
	}

	n := uint64(st.NumValidators())
	progress, err := st.Progress()
	if err != nil {
		return err
	}
	progress = writableBitlist(progress, n)
	targets, err := st.TargetParticipation()
	if err != nil {
		return err
	}
	targets = writableBitlist(targets, n)
	finalize, err := st.FinalityParticipation()
	if err != nil {
		return err
	}
	finalize = writableBitlist(finalize, n)
	attestationEpoch := slots.ToEpoch(data.Slot)
	perIncrement, _, err := baseRewardPerIncrementAtEpoch(ctx, st, attestationEpoch)
	if err != nil {
		return err
	}
	var proposerRewardNumerator uint64
	apply := func(participation []byte) ([]byte, error) {
		for _, idx := range indices {
			index := primitives.ValidatorIndex(idx)
			val, err := st.ValidatorAtIndexReadOnly(index)
			if err != nil {
				return nil, err
			}
			// Judged against the duty epoch so a final vote at an exit boundary still counts.
			if !helpers.IsActiveValidatorUsingTrie(val, attestationEpoch) {
				continue
			}
			if finalityCounts {
				finalize.SetBitAt(idx, true)
			}
			if heightCounts {
				progress.SetBitAt(idx, true)
				if !data.Timeout {
					targets.SetBitAt(idx, true)
				}
			}
			if participation == nil {
				continue
			}
			baseReward := baseRewardFromIncrement(val.EffectiveBalance(), perIncrement)
			if finalityMatches {
				set, err := setFlag(participation, index, cfg.TimelyFinalityTargetFlagIndex)
				if err != nil {
					return nil, err
				}
				if set {
					proposerRewardNumerator += baseReward * cfg.TimelySourceWeight
				}
			}
			if heightCounts {
				set, err := setFlag(participation, index, cfg.TimelyTargetFlagIndex)
				if err != nil {
					return nil, err
				}
				if set {
					proposerRewardNumerator += baseReward * cfg.TimelyTargetWeight
				}
			}
		}
		return participation, nil
	}
	if modify != nil {
		if err := modify(apply); err != nil {
			return err
		}
	} else if _, err := apply(nil); err != nil {
		return err
	}
	if err := st.SetProgress(progress); err != nil {
		return err
	}
	if err := st.SetTargetParticipation(targets); err != nil {
		return err
	}
	if err := st.SetFinalityParticipation(finalize); err != nil {
		return err
	}
	if proposerRewardNumerator == 0 {
		return nil
	}
	proposer, err := helpers.BeaconProposerIndex(ctx, st)
	if err != nil {
		return err
	}
	return helpers.IncreaseBalance(st, proposer, proposerRewardNumerator/attestationProposerRewardDenominator(data.Slot))
}

// Reports whether the flag was newly set, the condition the proposer reward keys on.
func setFlag(participation []byte, index primitives.ValidatorIndex, flag uint8) (bool, error) {
	if uint64(index) >= uint64(len(participation)) {
		return false, fmt.Errorf("validator %d beyond participation length %d", index, len(participation))
	}
	has, err := altair.HasValidatorFlag(participation[index], flag)
	if err != nil || has {
		return false, err
	}
	participation[index], err = altair.AddValidatorFlag(participation[index], flag)
	return err == nil, err
}
