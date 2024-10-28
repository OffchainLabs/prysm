package doublylinkedtree

import (
	"time"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
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

// InsertPayloadEnvelope adds a full node to forkchoice from the given payload
// envelope.
func (f *ForkChoice) InsertPayloadEnvelope(envelope interfaces.ROExecutionPayloadEnvelope) error {
	s := f.store
	b, ok := s.nodeByRoot[envelope.BeaconBlockRoot()]
	if !ok {
		return ErrNilNode
	}
	e, err := envelope.Execution()
	if err != nil {
		return err
	}
	hash := [32]byte(e.BlockHash())
	if _, ok = s.nodeByPayload[hash]; ok {
		// We ignore nodes with the give payload hash already included
		return nil
	}
	n := &Node{
		slot:                     b.slot,
		root:                     b.root,
		payloadHash:              hash,
		parent:                   b.parent,
		target:                   b.target,
		children:                 make([]*Node, 0),
		justifiedEpoch:           b.justifiedEpoch,
		unrealizedJustifiedEpoch: b.unrealizedJustifiedEpoch,
		finalizedEpoch:           b.finalizedEpoch,
		unrealizedFinalizedEpoch: b.unrealizedFinalizedEpoch,
		timestamp:                uint64(time.Now().Unix()),
		ptcVote:                  make([]primitives.PTCStatus, 0),
		withheld:                 envelope.PayloadWithheld(),
		optimistic:               true,
	}
	if n.parent != nil {
		n.parent.children = append(n.parent.children, n)
	}
	s.nodeByPayload[hash] = n
	processedPayloadCount.Inc()
	payloadCount.Set(float64(len(s.nodeByPayload)))

	if b.slot == s.highestReceivedNode.slot {
		s.highestReceivedNode = n
	}
	return nil
}
