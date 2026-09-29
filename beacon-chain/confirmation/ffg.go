package confirmation

import (
	"context"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// HonestFFGSupport lazily yields compute_honest_ffg_support_for_current_target and the total active balance.
type HonestFFGSupport func() (support uint64, totalActiveBalance uint64)

// GetCurrentTargetScore returns the estimated FFG support for the current epoch
// target checkpoint by examining LMD-GHOST votes.
//
//	<spec fn="get_current_target_score" fork="phase0">
//
//	target = get_current_target(store)
//	state = get_pulled_up_head_state(store)
//	unslashed_and_active_indices = [
//	    i for i in get_active_validator_indices(state, get_current_epoch(state))
//	    if not state.validators[i].slashed
//	]
//	return sum(
//	    state.validators[i].effective_balance
//	    for i in unslashed_and_active_indices
//	    if (i in store.latest_messages
//	        and i not in store.equivocating_indices
//	        and target == get_checkpoint_for_block(
//	            store, store.latest_messages[i].root,
//	            get_latest_message_epoch(store.latest_messages[i]))))
//	</spec>
func GetCurrentTargetScore(
	ctx context.Context,
	fc ForkchoiceReader,
	ffg *FFGStateInfo,
	votes []forkchoicetypes.VoteData,
	equivocating map[primitives.ValidatorIndex]bool,
	currentTarget forkchoicetypes.Checkpoint,
) uint64 {
	epochStart, err := slots.EpochStart(currentTarget.Epoch)
	if err != nil {
		return 0
	}
	score := uint64(0)

	cpByRoot := make(map[[32]byte][32]byte)
	for i, vote := range votes {
		if vote.Root == ([32]byte{}) {
			continue
		}
		// Only same-epoch votes can imply the current target, filter before the ancestor walk.
		if slots.ToEpoch(vote.Slot) != currentTarget.Epoch {
			continue
		}
		if i >= len(ffg.Balances) || ffg.Balances[i] == 0 {
			continue
		}
		if equivocating[primitives.ValidatorIndex(i)] {
			continue
		}

		cpRoot, ok := cpByRoot[vote.Root]
		if !ok {
			// A failed derivation caches the zero root, a real checkpoint root is never zero.
			cpRoot, _ = fc.AncestorRoot(ctx, vote.Root, epochStart)
			cpByRoot[vote.Root] = cpRoot
		}
		if cpRoot != ([32]byte{}) && cpRoot == currentTarget.Root {
			score += ffg.Balances[i]
		}
	}
	return score
}

// ComputeHonestFFGSupport computes the estimated honest FFG support for the
// current epoch target, accounting for already-received votes, remaining honest
// weight, and adversarial budget.
//
//	<spec fn="compute_honest_ffg_support_for_current_target" fork="phase0">
//
//	ffg_support_for_checkpoint = get_current_target_score(store)
//	ffg_weight_till_now = estimate_committee_weight_between_slots(
//	    total_active_balance, compute_start_slot_at_epoch(current_epoch), current_slot - 1)
//	remaining_ffg_weight = total_active_balance - ffg_weight_till_now
//	remaining_honest_ffg_weight = remaining_ffg_weight // 100 * (100 - CONFIRMATION_BYZANTINE_THRESHOLD)
//	adversarial_weight = compute_adversarial_weight(
//	    store, balance_source, compute_start_slot_at_epoch(current_epoch), current_slot - 1)
//	min_honest_ffg_support = ffg_support - min(adversarial_weight, ffg_support)
//	return min_honest_ffg_support + remaining_honest_ffg_weight
//	</spec>
func ComputeHonestFFGSupport(
	ctx context.Context,
	fc ForkchoiceReader,
	ffg *FFGStateInfo,
	votes []forkchoicetypes.VoteData,
	equivocating map[primitives.ValidatorIndex]bool,
	currentTarget forkchoicetypes.Checkpoint,
	currentSlot primitives.Slot,
	equivScorer EquivocationScorer,
) uint64 {
	if currentSlot == 0 {
		return 0
	}
	currentEpoch := slots.ToEpoch(currentSlot)
	epochStart, err := slots.EpochStart(currentEpoch)
	if err != nil {
		return 0
	}
	totalActiveBalance := ffg.TotalActiveBalance

	ffgSupport := GetCurrentTargetScore(ctx, fc, ffg, votes, equivocating, currentTarget)

	ffgWeightTillNow := EstimateCommitteeWeightBetweenSlots(totalActiveBalance, epochStart, currentSlot-1)

	remainingFFGWeight := uint64(0)
	if totalActiveBalance > ffgWeightTillNow {
		remainingFFGWeight = totalActiveBalance - ffgWeightTillNow
	}
	remainingHonestFFGWeight := remainingFFGWeight / 100 * (100 - params.BeaconConfig().ConfirmationByzantineThreshold)

	equivScore := equivScorer(epochStart, currentSlot-1)
	adversarialWeight := ComputeAdversarialWeight(totalActiveBalance, equivScore, epochStart, currentSlot-1)

	minHonestFFGSupport := uint64(0)
	if ffgSupport > adversarialWeight {
		minHonestFFGSupport = ffgSupport - adversarialWeight
	}

	return minHonestFFGSupport + remainingHonestFFGWeight
}

// WillNoConflictingCheckpointBeJustified returns true if no checkpoint conflicting
// with the current target can ever be justified.
//
//	<spec fn="will_no_conflicting_checkpoint_be_justified" fork="phase0">
//
//	if get_current_target(store) == store.unrealized_justified_checkpoint:
//	    return True
//	total_active_balance = get_total_active_balance(state)
//	honest_ffg_support = compute_honest_ffg_support_for_current_target(store)
//	return 3 * honest_ffg_support > 1 * total_active_balance
//	</spec>
func WillNoConflictingCheckpointBeJustified(honest HonestFFGSupport, currentTarget, unrealizedJustified forkchoicetypes.Checkpoint) bool {
	if currentTarget == unrealizedJustified {
		return true
	}
	support, total := honest()
	return 3*support > total
}

// WillCurrentTargetBeJustified returns true if the current target will eventually
// be justified, assuming honest validators vote for it from now on.
//
//	<spec fn="will_current_target_be_justified" fork="phase0">
//
//	total_active_balance = get_total_active_balance(state)
//	honest_ffg_support = compute_honest_ffg_support_for_current_target(store)
//	return 3 * honest_ffg_support >= 2 * total_active_balance
//	</spec>
func WillCurrentTargetBeJustified(honest HonestFFGSupport) bool {
	support, total := honest()
	return 3*support >= 2*total
}
