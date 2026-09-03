package doublylinkedtree

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// orphanLateBlockProposingEarly determines the maximum threshold that we
// consider the node is proposing early and sure to receive proposer boost
const orphanLateBlockProposingEarly = 2

// ShouldOverrideFCU returns whether the current forkchoice head is weak
// and thus may be reorged when proposing the next block.
// This function should only be called if the following two conditions are
// satisfied:
// 1-   It is immediately after receiving a block that may be subject to a reorg
//
//	or
//
//	It is right after processAttestationsThreshold and we have processed the
//	current slots attestations.
//
// 2- The caller has already called Forkchoice.Head() so that forkchoice has
// been updated.
// 3- The beacon node is serving a validator that will propose during the next
// slot.
//
// This function only applies a heuristic to decide if the beacon will update
// the engine's view of head with the parent block or the incoming block. It
// does not guarantee an attempted reorg. This will only be decided later at
// proposal time by calling GetProposerHead.
func (f *ForkChoice) ShouldOverrideFCU() (override bool) {
	override = false

	// We only need to override FCU if our current consensusHead is from the current
	// slot. This differs from the spec implementation in that we assume
	// that we will call this function in the previous slot to proposing.
	consensusHead := f.store.headNode
	if consensusHead == nil {
		return
	}

	if consensusHead.slot != slots.CurrentSlot(f.store.genesisTime) {
		return
	}

	// is_shuffling_stable, removed in Fulu by EIP-7917
	proposalSlot := consensusHead.slot + 1
	if slots.ToEpoch(proposalSlot) < params.BeaconConfig().FuluForkEpoch && slots.IsEpochStart(proposalSlot) {
		return
	}

	head := f.store.choosePayloadContent(consensusHead)
	// Only reorg blocks that arrive late
	early, err := head.arrivedEarly(f.store.genesisTime)
	if err != nil {
		log.WithError(err).Error("Could not check if block arrived early")
		return
	}
	if early {
		return
	}
	// Only reorg if we have been finalizing
	finalizedEpoch := f.store.finalizedCheckpoint.Epoch
	if slots.ToEpoch(consensusHead.slot+1) > finalizedEpoch+params.BeaconConfig().ReorgMaxEpochsSinceFinalization {
		return
	}
	// Only orphan a single block
	parent := consensusHead.parent
	if parent == nil {
		return
	}
	if consensusHead.slot > parent.node.slot+1 {
		return
	}
	// Do not orphan a block that has higher justification than the parent
	// if head.unrealizedJustifiedEpoch > parent.unrealizedJustifiedEpoch {
	//		return
	// }

	// Only orphan a block if the head LMD vote is weak
	if !f.store.isHeadWeak(consensusHead) {
		return
	}

	// Return early if we are checking before 10 seconds into the slot
	sss, err := slots.SinceSlotStart(consensusHead.slot, f.store.genesisTime, time.Now())
	if err != nil {
		log.WithError(err).Error("could not check current slot")
		return true
	}
	if sss < ProcessAttestationsThreshold {
		return true
	}
	// Only orphan a block if the parent LMD vote is strong
	if parent.node.weight*100 < f.store.committeeWeight*params.BeaconConfig().ReorgParentWeightThreshold {
		return
	}
	return true
}

// GetProposerHead returns the block root that has to be used as ParentRoot by a
// proposer. It may not be the actual head of the canonical chain, in certain
// cases it may be its parent, when the last head block has arrived early and is
// considered safe to be orphaned.
//
// This function needs to be called only when proposing a block and all
// attestation processing has already happened.
func (f *ForkChoice) GetProposerHead() [32]byte {
	consensusHead := f.store.headNode
	if consensusHead == nil {
		return [32]byte{}
	}
	// Only reorg blocks from the previous slot.
	currentSlot := slots.CurrentSlot(f.store.genesisTime)
	if consensusHead.slot+1 != currentSlot {
		return consensusHead.root
	}
	// is_shuffling_stable, removed in Fulu by EIP-7917
	if slots.ToEpoch(currentSlot) < params.BeaconConfig().FuluForkEpoch && slots.IsEpochStart(currentSlot) {
		return consensusHead.root
	}
	// Only reorg blocks that arrive late
	head := f.store.choosePayloadContent(consensusHead)
	if slots.ToEpoch(consensusHead.slot) >= params.BeaconConfig().GloasForkEpoch {
		head = f.store.emptyNodeByRoot[consensusHead.root]
	}
	if head == nil {
		return consensusHead.root
	}
	early, err := head.arrivedEarly(f.store.genesisTime)
	if err != nil {
		log.WithError(err).Error("could not check if block arrived early")
		return consensusHead.root
	}
	if early {
		return consensusHead.root
	}
	// Only reorg if we have been finalizing
	finalizedEpoch := f.store.finalizedCheckpoint.Epoch
	if slots.ToEpoch(consensusHead.slot+1) > finalizedEpoch+params.BeaconConfig().ReorgMaxEpochsSinceFinalization {
		return consensusHead.root
	}
	// Only orphan a single block
	parent := consensusHead.parent
	if parent == nil {
		return consensusHead.root
	}
	if consensusHead.slot > parent.node.slot+1 {
		return consensusHead.root
	}

	// Only orphan a block if the head LMD vote is weak
	if !f.store.isHeadWeak(consensusHead) {
		return consensusHead.root
	}

	// Only orphan a block if the parent LMD vote is strong
	if parent.node.weight*100 < f.store.committeeWeight*params.BeaconConfig().ReorgParentWeightThreshold {
		return consensusHead.root
	}

	// Only reorg if we are proposing early
	sss, err := slots.SinceSlotStart(currentSlot, f.store.genesisTime, time.Now())
	if err != nil {
		log.WithError(err).Error("could not check if proposing early")
		return consensusHead.root
	}
	if sss >= orphanLateBlockProposingEarly*time.Second {
		return consensusHead.root
	}
	return parent.node.root
}

// cacheWeakHeadCommittees only loads committees for the slots used by weak-head checks.
func (f *ForkChoice) cacheWeakHeadCommittees(ctx context.Context) error {
	currentSlot := f.store.currentSlot()
	for _, pn := range f.store.emptyNodeByRoot {
		n := pn.node
		if n.slot <= currentSlot && currentSlot-n.slot > 1 {
			n.slotCommittee = nil
			continue
		}
		if len(f.store.slashedIndices) == 0 || n.slotCommittee != nil || n.slot > currentSlot {
			continue
		}
		indices, err := f.weakHeadCommittee(ctx, n.root, n.slot, nil)
		if err != nil {
			return err
		}
		n.slotCommittee = indices
	}
	return nil
}

func (f *ForkChoice) weakHeadCommittee(ctx context.Context, root [32]byte, slot primitives.Slot, st state.ReadOnlyBeaconState) ([]primitives.ValidatorIndex, error) {
	if f.committeesByRoot == nil {
		return nil, fmt.Errorf("missing committee provider for root %#x", root)
	}
	committees, err := f.committeesByRoot(ctx, root, slot, st)
	if err != nil {
		return nil, err
	}
	// Preserve unavailable state as nil, distinct from a loaded empty committee.
	if committees == nil {
		return nil, nil
	}
	indices := make([]primitives.ValidatorIndex, 0)
	for _, committee := range committees {
		indices = append(indices, committee...)
	}
	return indices, nil
}

// isHeadWeak restores equivocators' committee weight only for weak-head checks.
func (s *Store) isHeadWeak(n *Node) bool {
	if len(s.slashedIndices) != 0 && n.slotCommittee == nil {
		return false
	}
	weight := s.attestationScore(n)
	for _, index := range n.slotCommittee {
		if s.slashedIndices[index] && uint64(index) < uint64(len(s.justifiedEffectiveBalances)) {
			weight += s.justifiedEffectiveBalances[index]
		}
	}
	threshold := s.weakHeadCommitteeWeight * params.BeaconConfig().ReorgHeadWeightThreshold / 100
	return weight < threshold
}

// attestationScore reads current balances, excluding proposer boost and stale cached weights.
func (s *Store) attestationScore(n *Node) uint64 {
	weight := n.balance
	if n.root == s.previousProposerBoostRoot {
		weight -= s.previousProposerBoostScore
	}
	for _, pn := range []*PayloadNode{s.emptyNodeByRoot[n.root], s.fullNodeByRoot[n.root]} {
		if pn == nil {
			continue
		}
		weight += pn.balance
		for _, child := range pn.children {
			weight += s.attestationScore(child)
		}
	}
	return weight
}
