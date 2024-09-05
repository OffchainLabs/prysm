package doublylinkedtree

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func (n *Node) isParentFull() bool {
	// Finalized checkpoint is considered full
	if n.parent == nil || n.parent.parent == nil {
		return true
	}
	return n.parent.payloadHash != [32]byte{}
}

func (f *ForkChoice) GetPTCVote() primitives.PTCStatus {
	highestNode := f.store.highestReceivedNode
	if highestNode == nil {
		return primitives.PAYLOAD_ABSENT
	}
	if slots.CurrentSlot(f.store.genesisTime) > highestNode.slot {
		return primitives.PAYLOAD_ABSENT
	}
	if highestNode.payloadHash == [32]byte{} {
		return primitives.PAYLOAD_ABSENT
	}
	if highestNode.withheld {
		return primitives.PAYLOAD_WITHHELD
	}
	return primitives.PAYLOAD_PRESENT
}
