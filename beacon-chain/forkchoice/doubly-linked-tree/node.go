package doublylinkedtree

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

const processAttestationsBeforeSlotEnd = 2 * time.Second

// ProcessAttestationsThreshold is the time into the slot after which we process attestations for the current slot.
func ProcessAttestationsThreshold(slot primitives.Slot) time.Duration {
	d := params.BeaconConfig().SlotDurationAt(slot)
	return max(d-processAttestationsBeforeSlotEnd, d/2)
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

// arrivedEarly returns whether this node was inserted before the first
// threshold to orphan a block.
func (n *PayloadNode) arrivedEarly(genesis time.Time) (bool, error) {
	sss, err := slots.SinceSlotStart(n.node.slot, genesis, n.timestamp)
	votingWindow := params.BeaconConfig().SlotComponentDurationAt(params.AttestationDue, n.node.slot)
	return sss < votingWindow, err
}

// arrivedAfterOrphanCheck returns whether this block was inserted after the
// intermediate checkpoint to check for candidate of being orphaned.
func (n *PayloadNode) arrivedAfterOrphanCheck(genesis time.Time) (bool, error) {
	sss, err := slots.SinceSlotStart(n.node.slot, genesis, n.timestamp)
	return sss >= ProcessAttestationsThreshold(n.node.slot), err
}
