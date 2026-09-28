package confirmation

import (
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// committeeWeightEstimationAdjustmentFactor is the per-mille value added to the
// committee weight estimate for cross-epoch slot ranges to ensure safety.
//
// Spec constant: COMMITTEE_WEIGHT_ESTIMATION_ADJUSTMENT_FACTOR = uint64(5)
const committeeWeightEstimationAdjustmentFactor = uint64(5)

// IsFullValidatorSetCovered returns true if the slot range [startSlot, endSlot]
// includes an entire epoch of committee assignments.
//
//	<spec fn="is_full_validator_set_covered" fork="phase0">
//
//	start_full_epoch = compute_epoch_at_slot(start_slot + (SLOTS_PER_EPOCH - 1))
//	end_full_epoch = compute_epoch_at_slot(Slot(end_slot + 1))
//	return start_full_epoch < end_full_epoch
//	</spec>
func IsFullValidatorSetCovered(startSlot, endSlot primitives.Slot) bool {
	spe := params.BeaconConfig().SlotsPerEpoch
	startFullEpoch := slots.ToEpoch(startSlot + primitives.Slot(spe-1))
	endFullEpoch := slots.ToEpoch(endSlot + 1)
	return startFullEpoch < endFullEpoch
}

// AdjustCommitteeWeightEstimate adjusts the committee weight estimate for
// cross-epoch slot ranges by adding a per-mille safety margin.
//
//	<spec fn="adjust_committee_weight_estimate_to_ensure_safety" fork="phase0">
//
//	ceil = (estimate + 999) // 1000
//	return Gwei(ceil * (1000 + COMMITTEE_WEIGHT_ESTIMATION_ADJUSTMENT_FACTOR))
//	</spec>
func AdjustCommitteeWeightEstimate(estimate uint64) uint64 {
	ceil := (estimate + 999) / 1000
	return ceil * (1000 + committeeWeightEstimationAdjustmentFactor)
}

// EstimateCommitteeWeightBetweenSlots estimates the total committee weight in
// [startSlot, endSlot] using total active balance and slot counts.
//
//	<spec fn="estimate_committee_weight_between_slots" fork="phase0">
//
//	if start_slot > end_slot:
//	    return Gwei(0)
//	if is_full_validator_set_covered(start_slot, end_slot):
//	    return total_active_balance
//	start_epoch = compute_epoch_at_slot(start_slot)
//	end_epoch = compute_epoch_at_slot(end_slot)
//	committee_weight = total_active_balance // SLOTS_PER_EPOCH
//	if start_epoch == end_epoch:
//	    return committee_weight * (end_slot - start_slot + 1)
//	else:
//	    num_slots_in_end_epoch = compute_slots_since_epoch_start(end_slot) + 1
//	    remaining_slots_in_end_epoch = SLOTS_PER_EPOCH - num_slots_in_end_epoch
//	    num_slots_in_start_epoch = SLOTS_PER_EPOCH - compute_slots_since_epoch_start(start_slot)
//	    start_epoch_weight = committee_weight * num_slots_in_start_epoch
//	    end_epoch_weight = committee_weight * num_slots_in_end_epoch
//	    start_epoch_weight_pro_rated = start_epoch_weight // SLOTS_PER_EPOCH * remaining_slots_in_end_epoch
//	    return adjust_committee_weight_estimate_to_ensure_safety(
//	        start_epoch_weight_pro_rated + end_epoch_weight)
//	</spec>
func EstimateCommitteeWeightBetweenSlots(totalActiveBalance uint64, startSlot, endSlot primitives.Slot) uint64 {
	if startSlot > endSlot {
		return 0
	}
	if IsFullValidatorSetCovered(startSlot, endSlot) {
		return totalActiveBalance
	}

	spe := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	startEpoch := slots.ToEpoch(startSlot)
	endEpoch := slots.ToEpoch(endSlot)
	committeeWeight := totalActiveBalance / uint64(spe)

	if startEpoch == endEpoch {
		return committeeWeight * uint64(endSlot-startSlot+1)
	}

	// Cross-epoch case
	numSlotsInEndEpoch := uint64(slots.SinceEpochStarts(endSlot)) + 1
	remainingSlotsInEndEpoch := uint64(spe) - numSlotsInEndEpoch
	numSlotsInStartEpoch := uint64(spe) - uint64(slots.SinceEpochStarts(startSlot))

	startEpochWeight := committeeWeight * numSlotsInStartEpoch
	endEpochWeight := committeeWeight * numSlotsInEndEpoch
	startEpochWeightProRated := startEpochWeight / uint64(spe) * remainingSlotsInEndEpoch

	return AdjustCommitteeWeightEstimate(startEpochWeightProRated + endEpochWeight)
}

// ComputeAdversarialWeight returns the maximum possible adversarial weight in
// committees for [startSlot, endSlot], discounting known equivocators.
//
//	<spec fn="compute_adversarial_weight" fork="phase0">
//
//	maximum_weight = estimate_committee_weight_between_slots(total_active_balance, start_slot, end_slot)
//	max_adversarial_weight = maximum_weight // 100 * CONFIRMATION_BYZANTINE_THRESHOLD
//	equivocation_score = get_equivocation_score(store, balance_source, start_slot, end_slot)
//	if max_adversarial_weight > equivocation_score:
//	    return max_adversarial_weight - equivocation_score
//	else:
//	    return Gwei(0)
//	</spec>
func ComputeAdversarialWeight(totalActiveBalance, equivocationScore uint64, startSlot, endSlot primitives.Slot) uint64 {
	maximumWeight := EstimateCommitteeWeightBetweenSlots(totalActiveBalance, startSlot, endSlot)
	maxAdversarialWeight := maximumWeight / 100 * params.BeaconConfig().ConfirmationByzantineThreshold
	if maxAdversarialWeight > equivocationScore {
		return maxAdversarialWeight - equivocationScore
	}
	return 0
}

// ComputeProposerScore returns the proposer boost score based on total active balance.
//
//	<spec fn="compute_proposer_score" fork="phase0">
//
//	committee_weight = get_total_active_balance(state) // SLOTS_PER_EPOCH
//	return (committee_weight * PROPOSER_SCORE_BOOST) // 100
//	</spec>
func ComputeProposerScore(totalActiveBalance uint64) uint64 {
	committeeWeight := totalActiveBalance / uint64(params.BeaconConfig().SlotsPerEpoch)
	return committeeWeight * params.BeaconConfig().ProposerScoreBoost / 100
}
