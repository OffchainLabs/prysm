package doublylinkedtree

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	forkchoice2 "github.com/OffchainLabs/prysm/v7/consensus-types/forkchoice"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// ProcessAttestationsThreshold is the amount of time after which we
// process attestations for the current slot
const ProcessAttestationsThreshold = 10 * time.Second

// applyWeightChanges recomputes the weight of the node passed as an argument and all of its descendants,
// using the current balance stored in each node.
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

// updateBestDescendant updates the best descendant of this node and its
// children.
func (n *Node) updateBestDescendant(ctx context.Context, justifiedEpoch, finalizedEpoch, currentEpoch primitives.Epoch) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(n.children) == 0 {
		n.bestDescendant = nil
		return nil
	}

	var bestChild *Node
	bestWeight := uint64(0)
	hasViableDescendant := false
	for _, child := range n.children {
		if child == nil {
			return errors.Wrap(ErrNilNode, "could not update best descendant")
		}
		if err := child.updateBestDescendant(ctx, justifiedEpoch, finalizedEpoch, currentEpoch); err != nil {
			return err
		}
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
		if bestChild.bestDescendant == nil {
			n.bestDescendant = bestChild
		} else {
			n.bestDescendant = bestChild.bestDescendant
		}
	} else {
		n.bestDescendant = nil
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

// isNodeReady returns true if this node's local conditions for being
// non-optimistic are met: the node is EL-validated, and if the ZKVM feature
// is enabled and the node's slot is at or after the Fulu fork, it also has
// enough execution proofs.
func (n *Node) isNodeReady() (bool, error) {
	if !n.elValidated {
		return false, nil
	}
	if !features.Get().EnableZkvm {
		return true, nil
	}

	fuluStart, err := slots.EpochStart(params.BeaconConfig().FuluForkEpoch)
	if err != nil {
		return false, fmt.Errorf("could not compute Fulu epoch start: %w", err)
	}

	if n.slot >= fuluStart && !n.hasEnoughProofs {
		return false, nil
	}

	return true, nil
}

// tryMarkValid transitions this node from optimistic to valid if it is locally
// ready and its parent is non-optimistic, then recursively propagates the
// transition to all descendants by invoking itself on each child.
func (n *Node) tryMarkValid(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !n.optimistic {
		return nil
	}

	ready, err := n.isNodeReady()
	if err != nil {
		return fmt.Errorf("is node ready: %w", err)
	}

	if !ready {
		return nil
	}

	if n.parent != nil && n.parent.optimistic {
		return nil
	}

	n.optimistic = false
	for _, child := range n.children {
		if err := child.tryMarkValid(ctx); err != nil {
			return fmt.Errorf("try mark valid child: %w", err)
		}
	}

	return nil
}

// arrivedEarly returns whether this node was inserted before the first
// threshold to orphan a block.
// Note that genesisTime has seconds granularity, therefore we use a strict
// inequality < here. For example a block that arrives 3.9999 seconds into the
// slot will have secs = 3 below.
func (n *Node) arrivedEarly(genesis time.Time) (bool, error) {
	sss, err := slots.SinceSlotStart(n.slot, genesis, n.timestamp.Truncate(time.Second)) // Truncate such that 3.9999 seconds will have a value of 3.
	votingWindow := params.BeaconConfig().SlotComponentDuration(params.BeaconConfig().AttestationDueBPS)
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
