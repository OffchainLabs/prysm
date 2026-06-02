package doublylinkedtree

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// ProcessAttestationsThreshold is the amount of time after which we
// process attestations for the current slot
const ProcessAttestationsThreshold = 10 * time.Second

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
// valid (ie. non optimistic) are met: the node is EL-validated, and if the ZKVM
// feature is enabled and the node's slot is at or after the Fulu fork, it also
// has enough execution proofs.
func (pn *PayloadNode) isNodeReady() (bool, error) {
	if !pn.elValidated {
		return false, nil
	}

	if !features.Get().IsZkvmEnabled() {
		return true, nil
	}

	fuluStart, err := slots.EpochStart(params.BeaconConfig().FuluForkEpoch)
	if err != nil {
		return false, fmt.Errorf("could not compute Fulu epoch start: %w", err)
	}

	if pn.node != nil && pn.node.slot >= fuluStart && !pn.hasEnoughProofs {
		return false, nil
	}

	return true, nil
}

// tryMarkValid transitions this payload node from optimistic to valid if it is
// locally ready and its parent is valid (ie. non optimistic), then recursively
// propagates the transition to all descendants by invoking itself on each child.
func (pn *PayloadNode) tryMarkValid(ctx context.Context, store *Store) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !pn.optimistic {
		return nil
	}

	ready, err := pn.isNodeReady()
	if err != nil {
		return fmt.Errorf("is node ready: %w", err)
	}

	if !ready {
		return nil
	}

	if pn.node != nil && pn.node.parent != nil && pn.node.parent.optimistic {
		return nil
	}

	pn.optimistic = false
	for _, child := range pn.children {
		childPn := store.fullNodeByRoot[child.root]
		if childPn == nil {
			childPn = store.emptyNodeByRoot[child.root]
		}

		if childPn == nil {
			continue
		}

		if err := childPn.tryMarkValid(ctx, store); err != nil {
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
func (n *PayloadNode) arrivedEarly(genesis time.Time) (bool, error) {
	sss, err := slots.SinceSlotStart(n.node.slot, genesis, n.timestamp.Truncate(time.Second)) // Truncate such that 3.9999 seconds will have a value of 3.
	votingWindow := params.BeaconConfig().SlotComponentDuration(params.BeaconConfig().AttestationDueBPS)
	return sss < votingWindow, err
}

// arrivedAfterOrphanCheck returns whether this block was inserted after the
// intermediate checkpoint to check for candidate of being orphaned.
// Note that genesisTime has seconds granularity, therefore we use an
// inequality >= here. For example a block that arrives 10.00001 seconds into the
// slot will have secs = 10 below.
func (n *PayloadNode) arrivedAfterOrphanCheck(genesis time.Time) (bool, error) {
	secs, err := slots.SinceSlotStart(n.node.slot, genesis, n.timestamp.Truncate(time.Second)) // Truncate such that 10.00001 seconds will have a value of 10.
	return secs >= ProcessAttestationsThreshold, err
}
