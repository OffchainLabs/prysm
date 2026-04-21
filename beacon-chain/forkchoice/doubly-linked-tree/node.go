package doublylinkedtree

import (
	"time"

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

// arrivedEarly returns whether this node was inserted before the first
// threshold to orphan a block.
// Note that genesisTime has seconds granularity, therefore we use a strict
// inequality < here. For example a block that arrives 3.9999 seconds into the
// slot will have secs = 3 below.
func (n *PayloadNode) arrivedEarly(genesis time.Time) (bool, error) {
	// The tree root node (genesis or the finalized checkpoint on checkpoint-sync)
	// carries an insertion timestamp sourced from time.Now() at process startup,
	// not actual network arrival time. It can never be a reorg target anyway, so
	// treat it as always having arrived early.
	if n.node.parent == nil {
		return true, nil
	}
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
	// See arrivedEarly: the tree root's timestamp is not meaningful here.
	if n.node.parent == nil {
		return false, nil
	}
	secs, err := slots.SinceSlotStart(n.node.slot, genesis, n.timestamp.Truncate(time.Second)) // Truncate such that 10.00001 seconds will have a value of 10.
	return secs >= ProcessAttestationsThreshold, err
}
