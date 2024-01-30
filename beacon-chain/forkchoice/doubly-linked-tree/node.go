package doublylinkedtree

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	forkchoice2 "github.com/prysmaticlabs/prysm/v5/consensus-types/forkchoice"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

// ProcessAttestationsThreshold  is the number of seconds after which we
// process attestations for the current slot
const ProcessAttestationsThreshold = 10

// applyWeightChanges recomputes the weight of the node passed as an argument and all of its descendants,
// using the current balance stored in each node.
func (n *Node) applyWeightChanges(ctx context.Context, proposerBoostRoot [32]byte, proposerBootScore uint64) error {
	// Recursively calling the children to sum their weights.
	childrenWeight := uint64(0)
	childrenVoteOnlyWeight := uint64(0)
	for _, child := range n.children {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := child.applyWeightChanges(ctx, proposerBoostRoot, proposerBootScore); err != nil {
			return err
		}
		childrenWeight += child.weight
		childrenVoteOnlyWeight += child.voteOnlyWeight
	}
	if n.root == params.BeaconConfig().ZeroHash {
		return nil
	}
	n.weight = n.balance + childrenWeight
	n.voteOnlyWeight = n.balance + childrenVoteOnlyWeight
	if n.root == proposerBoostRoot {
		if n.balance < proposerBootScore {
			return errors.New(fmt.Sprintf("invalid node weight %d is lesser than proposer boost score %d for root %#x", n.balance, proposerBoostRoot, n.root))
		}
		n.voteOnlyWeight -= proposerBootScore
	}
	return nil
}

func (n *Node) getMaxPossibleSupport(currentSlot primitives.Slot, committeeWeight uint64) uint64 {
	startSlot := n.slot
	if n.parent != nil {
		startSlot = n.parent.slot + 1
	}
	startEpoch := slots.ToEpoch(startSlot)
	currentEpoch := slots.ToEpoch(currentSlot)
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)

	// If the span of slots does not cover an epoch boundary, simply return the number of slots times committee weight.
	if startEpoch == currentEpoch {
		return committeeWeight * uint64(currentSlot-startSlot+1)
	}

	// If the entire validator set is covered between startSlot and currentSlot,
	// return the 32 * committeeWeight
	if currentEpoch > startEpoch+1 ||
		(currentEpoch == startEpoch+1 && uint64(startSlot)%slotsPerEpoch == 0) {
		return committeeWeight * slotsPerEpoch
	}

	// The span of slots goes across an epoch boundary, but does not cover any full epoch.
	// Do a pro-rata calculation of how many committees are contained.
	slotsInStartEpoch := slotsPerEpoch - (uint64(startSlot) % slotsPerEpoch)
	slotsInCurrentEpoch := (uint64(currentSlot) % slotsPerEpoch) + 1
	slotsRemainingInCurrentEpoch := slotsPerEpoch - slotsInCurrentEpoch
	weightFromCurrentEpoch := committeeWeight * slotsInCurrentEpoch
	weightFromStartEpoch := committeeWeight * slotsInStartEpoch * slotsRemainingInCurrentEpoch / slotsPerEpoch
	return weightFromCurrentEpoch + weightFromStartEpoch
}

func (n *Node) isOneConfirmed(currentSlot primitives.Slot, committeeWeight uint64) bool {
	proposerBoostWeight := (committeeWeight * params.BeaconConfig().ProposerScoreBoost) / 100
	maxPossibleSupport := n.getMaxPossibleSupport(currentSlot, committeeWeight)
	if maxPossibleSupport == 0 {
		return true
	}
	safeThreshold := (maxPossibleSupport + proposerBoostWeight) / 2
	return n.voteOnlyWeight > safeThreshold
}

// updateBestDescendant updates the best descendant of this node and its
// children.
func (n *Node) updateBestDescendant(ctx context.Context, justifiedEpoch primitives.Epoch, finalizedEpoch primitives.Epoch, genesisTime uint64, committeeWeight uint64) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(n.children) == 0 {
		n.bestDescendant = nil
		n.bestConfirmedDescendant = nil
		return nil
	}

	var bestChild *Node
	bestWeight := uint64(0)
	hasViableDescendant := false
	currentSlot := slots.CurrentSlot(genesisTime)
	for _, child := range n.children {
		if child == nil {
			return errors.Wrap(ErrNilNode, "could not update best descendant")
		}
		if err := child.updateBestDescendant(ctx, justifiedEpoch, finalizedEpoch, genesisTime, committeeWeight); err != nil {
			return err
		}
		currentEpoch := slots.ToEpoch(currentSlot)
		childLeadsToViableHead := child.leadsToViableHead(justifiedEpoch, currentEpoch)
		if childLeadsToViableHead && !hasViableDescendant {
			// The child leads to a viable head, but the current
			// parent's best child doesn't.
			bestWeight = child.weight
			bestChild = child
			hasViableDescendant = true
		} else if childLeadsToViableHead {
			// If both are viable, compare their weights.
			if child.weight == bestWeight {
				// Tie-breaker of equal weights by root.
				if bytes.Compare(child.root[:], bestChild.root[:]) > 0 {
					bestChild = child
				}
			} else if child.weight > bestWeight {
				bestChild = child
				bestWeight = child.weight
			}
		}
	}
	if hasViableDescendant {
		// This node has a viable descendant.
		if bestChild.bestDescendant == nil {
			// The best descendant is the best child.
			n.bestDescendant = bestChild
		} else {
			// The best descendant is more than 1 hop away.
			n.bestDescendant = bestChild.bestDescendant
		}

		// For safe head computation, consider the current slot as the latest slot for which we have received most of the attestations.
		safeHeadLatestSlot := currentSlot
		secsIntoSlot, err := slots.SecondsSinceSlotStart(currentSlot, genesisTime, uint64(time.Now().Unix()))
		if err != nil {
			return err
		}
		// If we are more than 10 seconds into the slot, assume that we have received most attestations from that slot.
		if secsIntoSlot < 10 {
			// If we are less than 10 seconds into the slot, set safeHeadLatestSlot to the previous slot.
			safeHeadLatestSlot = max(0, currentSlot-1)
		}
		if bestChild.slot < safeHeadLatestSlot-2 && bestChild.isOneConfirmed(safeHeadLatestSlot, committeeWeight) {
			// The best child is confirmed.
			if bestChild.bestConfirmedDescendant == nil {
				// The best child does not have confirmed descendants.
				n.bestConfirmedDescendant = bestChild
			} else {
				// The best child has confirmed descendants.
				n.bestConfirmedDescendant = bestChild.bestConfirmedDescendant
			}
		} else {
			// The best child is not confirmed. There is no confirmed descendant.
			n.bestConfirmedDescendant = nil
		}
	} else {
		n.bestDescendant = nil
		n.bestConfirmedDescendant = nil
	}
	return nil
}

// viableForHead returns true if the node is viable to head.
// Any node with different finalized or justified epoch than
// the ones in fork choice store should not be viable to head.
func (n *Node) viableForHead(justifiedEpoch, currentEpoch primitives.Epoch) bool {
	if justifiedEpoch == 0 {
		return true
	}
	// We use n.justifiedEpoch as the voting source because:
	//   1. if this node is from current epoch, n.justifiedEpoch is the realized justification epoch.
	//   2. if this node is from a previous epoch, n.justifiedEpoch has already been updated to the unrealized justification epoch.
	return n.justifiedEpoch == justifiedEpoch || n.justifiedEpoch+2 >= currentEpoch
}

func (n *Node) leadsToViableHead(justifiedEpoch, currentEpoch primitives.Epoch) bool {
	if n.bestDescendant == nil {
		return n.viableForHead(justifiedEpoch, currentEpoch)
	}
	return n.bestDescendant.viableForHead(justifiedEpoch, currentEpoch)
}

// setNodeAndParentValidated sets the current node and all the ancestors as validated (i.e. non-optimistic).
func (n *Node) setNodeAndParentValidated(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !n.optimistic {
		return nil
	}
	n.optimistic = false

	if n.parent == nil {
		return nil
	}
	return n.parent.setNodeAndParentValidated(ctx)
}

// arrivedEarly returns whether this node was inserted before the first
// threshold to orphan a block.
// Note that genesisTime has seconds granularity, therefore we use a strict
// inequality < here. For example a block that arrives 3.9999 seconds into the
// slot will have secs = 3 below.
func (n *Node) arrivedEarly(genesisTime uint64) (bool, error) {
	secs, err := slots.SecondsSinceSlotStart(n.slot, genesisTime, n.timestamp)
	votingWindow := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	return secs < votingWindow, err
}

// arrivedAfterOrphanCheck returns whether this block was inserted after the
// intermediate checkpoint to check for candidate of being orphaned.
// Note that genesisTime has seconds granularity, therefore we use an
// inequality >= here. For example a block that arrives 10.00001 seconds into the
// slot will have secs = 10 below.
func (n *Node) arrivedAfterOrphanCheck(genesisTime uint64) (bool, error) {
	secs, err := slots.SecondsSinceSlotStart(n.slot, genesisTime, n.timestamp)
	return secs >= ProcessAttestationsThreshold, err
}

// nodeTreeDump appends to the given list all the nodes descending from this one
func (n *Node) nodeTreeDump(ctx context.Context, nodes []*forkchoice2.Node) ([]*forkchoice2.Node, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var parentRoot [32]byte
	if n.parent != nil {
		parentRoot = n.parent.root
	}
	thisNode := &forkchoice2.Node{
		Slot:                     n.slot,
		BlockRoot:                n.root[:],
		ParentRoot:               parentRoot[:],
		JustifiedEpoch:           n.justifiedEpoch,
		FinalizedEpoch:           n.finalizedEpoch,
		UnrealizedJustifiedEpoch: n.unrealizedJustifiedEpoch,
		UnrealizedFinalizedEpoch: n.unrealizedFinalizedEpoch,
		Balance:                  n.balance,
		Weight:                   n.weight,
		ExecutionOptimistic:      n.optimistic,
		ExecutionBlockHash:       n.payloadHash[:],
		Timestamp:                n.timestamp,
	}
	if n.optimistic {
		thisNode.Validity = forkchoice2.Optimistic
	} else {
		thisNode.Validity = forkchoice2.Valid
	}

	nodes = append(nodes, thisNode)
	var err error
	for _, child := range n.children {
		nodes, err = child.nodeTreeDump(ctx, nodes)
		if err != nil {
			return nil, err
		}
	}
	return nodes, nil
}
