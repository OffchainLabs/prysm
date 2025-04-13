package doublylinkedtree

import (
	"bytes"
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	forkchoice2 "github.com/OffchainLabs/prysm/v6/consensus-types/forkchoice"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

// ProcessAttestationsThreshold is the amount of time after which we
// process attestations for the current slot
const ProcessAttestationsThreshold = 10 * time.Second

type updateDescendantArgs struct {
	justifiedEpoch        primitives.Epoch
	finalizedEpoch        primitives.Epoch
	currentSlot           primitives.Slot
	secondsSinceSlotStart time.Duration
	committeeWeight       uint64
	pbRoot                [32]byte
	pbValue               uint64
}

// applyWeightChanges recursively traverses a tree of nodes to update each node's total weight and
// weight without proposer boost by summing the balance of the node and its children.
// If the node matches a specific root (`pbRoot`), it subtracts a given boost value (`pbValue`) from the weight without boost,
// ensuring the balance is sufficient. It also handles context cancellation and errors during recursion.
func (n *Node) applyWeightChanges(ctx context.Context) error {
	// Recursively calling the children to sum their weights.
	childrenWeight := uint64(0)
	for _, child := range n.children {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := child.applyWeightChanges(ctx); err != nil {
			return err
		}
		childrenWeight += child.weight
	}
	if n.root == params.BeaconConfig().ZeroHash {
		return nil
	}
	n.weight = n.balance + childrenWeight
	return nil
}

// maxWeight computes the maximum possible voting weight for this node.
// This function computes the maximum weight a node can contribute from its start slot to the end slot,
// scaled by committee weight. If the range is within one epoch, it returns the number of slots times the committee weight.
// If the range spans at least one full epoch or starts at an epoch boundary and ends in the next epoch, it returns the full epoch weight.
// Otherwise, it prorates the weight based on the number of slots in the start and end epochs, accounting for partial epoch coverage.
func (n *Node) maxWeight(endSlot primitives.Slot, committeeWeight uint64) uint64 {
	startSlot := n.slot
	if n.parent != nil {
		startSlot = n.parent.slot + 1
	}
	if startSlot > endSlot {
		return 0
	}

	startEpoch := slots.ToEpoch(startSlot)
	endEpoch := slots.ToEpoch(endSlot)
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	slotSpan := uint64(endSlot - startSlot + 1)

	if startEpoch == endEpoch {
		return committeeWeight * slotSpan
	}

	if endEpoch > startEpoch+1 || (endEpoch == startEpoch+1 && uint64(startSlot)%slotsPerEpoch == 0) {
		return committeeWeight * slotsPerEpoch
	}

	slotsInStartEpoch := slotsPerEpoch - (uint64(startSlot) % slotsPerEpoch)
	slotsInEndEpoch := (uint64(endSlot) % slotsPerEpoch) + 1

	weightEnd := committeeWeight * slotsInEndEpoch
	weightStart := (committeeWeight * slotsInStartEpoch * (slotsPerEpoch - slotsInEndEpoch)) / slotsPerEpoch

	return weightEnd + weightStart
}

// updateBestDescendant updates the best descendant of this node and its
// children.
func (n *Node) updateBestDescendant(ctx context.Context, args *updateDescendantArgs) error {
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
	for _, child := range n.children {
		if child == nil {
			return errors.Wrap(ErrNilNode, "could not update best descendant")
		}
		if err := child.updateBestDescendant(ctx, args); err != nil {
			return err
		}
		currentEpoch := slots.ToEpoch(args.currentSlot)
		childLeadsToViableHead := child.leadsToViableHead(args.justifiedEpoch, currentEpoch)
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

		if uint64(args.secondsSinceSlotStart.Seconds()) < params.BeaconConfig().SecondsPerSlot/params.BeaconConfig().IntervalsPerSlot {
			prevSlot := primitives.Slot(0)
			if args.currentSlot > 1 {
				prevSlot = args.currentSlot - 1
			}

			if bestChild.confirmed(prevSlot, args.committeeWeight, args.pbRoot, args.pbValue) {
				n.bestConfirmedDescendant = bestChild.bestConfirmedDescendant
				if n.bestConfirmedDescendant == nil {
					n.bestConfirmedDescendant = bestChild
				}
			} else {
				n.bestConfirmedDescendant = nil
			}
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
func (n *Node) arrivedEarly(genesis time.Time) (bool, error) {
	sss, err := slots.SinceSlotStart(n.slot, genesis, n.timestamp.Truncate(time.Second)) // Truncate such that 3.9999 seconds will have a value of 3.
	votingWindow := time.Duration(params.BeaconConfig().SecondsPerSlot/params.BeaconConfig().IntervalsPerSlot) * time.Second
	return sss < votingWindow, err
}

// arrivedAfterOrphanCheck returns whether this block was inserted after the
// intermediate checkpoint to check for candidate of being orphaned.
// Note that genesisTime has seconds granularity, therefore we use an
// inequality >= here. For example a block that arrives 10.00001 seconds into the
// slot will have secs = 10 below.
func (n *Node) arrivedAfterOrphanCheck(genesis time.Time) (bool, error) {
	secs, err := slots.SinceSlotStart(n.slot, genesis, n.timestamp.Truncate(time.Second)) // Truncate such that 10.00001 seconds will have a value of 10.
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
	target := [32]byte{}
	if n.target != nil {
		target = n.target.root
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
		Target:                   target[:],
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

// confirmed returns true if the node satisfies the confirmation rule.
func (n *Node) confirmed(slot primitives.Slot, committeeWeight uint64, pbRoot [32]byte, pbValue uint64) bool {
	if n.slot > slot {
		return false
	}

	pbWeight := committeeWeight * params.BeaconConfig().ProposerScoreBoost / 100
	maxWeight := n.maxWeight(slot, committeeWeight)
	byzantineWeight := maxWeight * params.BeaconConfig().FastConfirmationByzantineThreshold / 100
	threshold := (maxWeight+pbWeight)/2 + byzantineWeight

	nodeWeight := n.weight

	if n.root == pbRoot || (n.bestDescendant != nil && n.bestDescendant.root == pbRoot) {
		if nodeWeight < pbValue {
			return false
		}
		nodeWeight -= pbValue
	}

	return nodeWeight > threshold
}
