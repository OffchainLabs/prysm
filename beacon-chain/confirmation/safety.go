package confirmation

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// GetAdversarialWeight returns the maximum adversarial weight that can support
// a competing block against blockRoot. When the block crosses an epoch boundary
// from its parent, the adversarial range starts at the block's epoch start
// instead of the block's slot.
//
//	<spec fn="get_adversarial_weight" fork="phase0">
//
//	current_slot = get_current_slot(store)
//	block = store.blocks[block_root]
//	if get_block_epoch(store, block_root) > get_block_epoch(store, block.parent_root):
//	    start_slot = compute_start_slot_at_epoch(get_block_epoch(store, block_root))
//	    return compute_adversarial_weight(store, balance_source, start_slot, current_slot - 1)
//	else:
//	    return compute_adversarial_weight(store, balance_source, block.slot, current_slot - 1)
//	</spec>
func GetAdversarialWeight(
	fc ForkchoiceReader,
	totalActiveBalance uint64,
	blockRoot [32]byte,
	currentSlot primitives.Slot,
	equivScorer EquivocationScorer,
) (uint64, error) {
	blockSlot, err := fc.Slot(blockRoot)
	if err != nil {
		return 0, err
	}
	parentRoot, err := fc.ParentRoot(blockRoot)
	if err != nil {
		return 0, err
	}
	parentSlot, err := fc.Slot(parentRoot)
	if err != nil {
		return 0, err
	}

	if currentSlot == 0 {
		return 0, nil
	}

	var startSlot primitives.Slot
	blockEpoch := slots.ToEpoch(blockSlot)
	parentEpoch := slots.ToEpoch(parentSlot)
	if blockEpoch > parentEpoch {
		startSlot = slots.UnsafeEpochStart(blockEpoch)
	} else {
		startSlot = blockSlot
	}

	equivScore := equivScorer(startSlot, currentSlot-1)
	return ComputeAdversarialWeight(totalActiveBalance, equivScore, startSlot, currentSlot-1), nil
}

// ComputeEmptySlotSupportDiscount returns the weight that can be discounted from
// the safety threshold when there are empty (skipped) slots between the block's
// parent and the block itself. Validators in those empty slots who already voted
// for the parent can't be used by an attacker.
//
//	<spec fn="compute_empty_slot_support_discount" fork="phase0">
//
//	block = store.blocks[block_root]
//	parent_block = store.blocks[block.parent_root]
//	if parent_block.slot + 1 == block.slot:
//	    return Gwei(0)
//	parent_support_in_empty_slots = get_block_support_between_slots(
//	    store, balance_source, block.parent_root,
//	    parent_block.slot + 1, block.slot - 1)
//	adversarial_weight = compute_adversarial_weight(
//	    store, balance_source, parent_block.slot + 1, block.slot - 1)
//	if parent_support_in_empty_slots > adversarial_weight:
//	    return parent_support_in_empty_slots - adversarial_weight
//	else:
//	    return Gwei(0)
//	</spec>
func ComputeEmptySlotSupportDiscount(
	fc ForkchoiceReader,
	support *SupportMap,
	totalActiveBalance uint64,
	blockRoot [32]byte,
	equivScorer EquivocationScorer,
) uint64 {
	// A zero discount on error raises the threshold, so these error returns fail closed.
	blockSlot, err := fc.Slot(blockRoot)
	if err != nil {
		return 0
	}
	parentRoot, err := fc.ParentRoot(blockRoot)
	if err != nil {
		return 0
	}
	parentSlot, err := fc.Slot(parentRoot)
	if err != nil {
		return 0
	}

	// No empty slot between parent and block.
	if parentSlot+1 == blockSlot {
		return 0
	}

	emptyStart := parentSlot + 1
	emptyEnd := blockSlot - 1

	parentSupport := support.BlockSupportBetweenSlots(parentRoot, emptyStart, emptyEnd)
	equivScore := equivScorer(emptyStart, emptyEnd)
	adversarial := ComputeAdversarialWeight(totalActiveBalance, equivScore, emptyStart, emptyEnd)

	if parentSupport > adversarial {
		return parentSupport - adversarial
	}
	return 0
}

// ComputeSafetyThreshold computes the LMD-GHOST safety threshold for a block.
// A block is confirmed when its attestation score exceeds this threshold.
//
//	<spec fn="compute_safety_threshold" fork="phase0">
//
//	total_active_balance = get_total_active_balance(balance_source)
//	proposer_score = compute_proposer_score(balance_source)
//	maximum_support = estimate_committee_weight_between_slots(
//	    total_active_balance, parent_block.slot + 1, current_slot - 1)
//	support_discount = get_support_discount(store, balance_source, block_root)
//	adversarial_weight = get_adversarial_weight(store, balance_source, block_root)
//	if support_discount < maximum_support + proposer_score + 2 * adversarial_weight:
//	    return (maximum_support + proposer_score + 2 * adversarial_weight - support_discount) // 2
//	else:
//	    return Gwei(0)
//	</spec>
func ComputeSafetyThreshold(
	fc ForkchoiceReader,
	support *SupportMap,
	totalActiveBalance uint64,
	blockRoot [32]byte,
	currentSlot primitives.Slot,
	equivScorer EquivocationScorer,
) (uint64, error) {
	if currentSlot == 0 {
		return 0, nil
	}
	parentRoot, err := fc.ParentRoot(blockRoot)
	if err != nil {
		return 0, err
	}
	parentSlot, err := fc.Slot(parentRoot)
	if err != nil {
		return 0, err
	}

	proposerScore := ComputeProposerScore(totalActiveBalance)
	maximumSupport := EstimateCommitteeWeightBetweenSlots(totalActiveBalance, parentSlot+1, currentSlot-1)
	supportDiscount := ComputeEmptySlotSupportDiscount(fc, support, totalActiveBalance, blockRoot, equivScorer)
	adversarialWeight, err := GetAdversarialWeight(fc, totalActiveBalance, blockRoot, currentSlot, equivScorer)
	if err != nil {
		return 0, err
	}

	numerator := maximumSupport + proposerScore + 2*adversarialWeight
	if supportDiscount < numerator {
		return (numerator - supportDiscount) / 2, nil
	}
	return 0, nil
}

// IsOneConfirmed returns true if the block is LMD-GHOST safe: its attestation
// score exceeds the safety threshold. Optimistic blocks are never confirmed.
//
//	<spec fn="is_one_confirmed" fork="phase0">
//
//	support = get_attestation_score(store, block_root, balance_source)
//	safety_threshold = compute_safety_threshold(store, block_root, balance_source)
//	return support > safety_threshold
//	</spec>
func IsOneConfirmed(
	fc ForkchoiceReader,
	support *SupportMap,
	totalActiveBalance uint64,
	blockRoot [32]byte,
	currentSlot primitives.Slot,
	equivScorer EquivocationScorer,
) bool {
	// Don't confirm optimistic blocks (per PR discussion, not in spec).
	optimistic, err := fc.IsOptimistic(blockRoot)
	if err != nil || optimistic {
		return false
	}

	score := support.AttestationScore(blockRoot)
	threshold, err := ComputeSafetyThreshold(fc, support, totalActiveBalance, blockRoot, currentSlot, equivScorer)
	if err != nil {
		return false
	}
	return score > threshold
}
