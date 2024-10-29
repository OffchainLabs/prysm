package doublylinkedtree

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func (f *ForkChoice) GetPTCVote() primitives.PTCStatus {
	highestNode := f.store.highestReceivedNode
	if highestNode == nil {
		return primitives.PAYLOAD_ABSENT
	}
	if slots.CurrentSlot(f.store.genesisTime) > highestNode.block.slot {
		return primitives.PAYLOAD_ABSENT
	}
	if highestNode.full {
		return primitives.PAYLOAD_PRESENT
	}
	return primitives.PAYLOAD_ABSENT
}

func (f *ForkChoice) insertExecutionPayload(b *BlockNode, e interfaces.ExecutionData) error {
	s := f.store
	hash := [32]byte(e.BlockHash())
	if _, ok := s.fullNodeByPayload[hash]; ok {
		// We ignore nodes with the give payload hash already included
		return nil
	}
	n := &Node{
		block:      b,
		children:   make([]*Node, 0),
		full:       true,
		optimistic: true,
	}
	if n.block.parent != nil {
		n.block.parent.children = append(n.block.parent.children, n)
	} else {
		// make this the tree node
		f.store.treeRootNode = n
	}
	s.fullNodeByPayload[hash] = n
	s.updateWithPayload(n)
	processedPayloadCount.Inc()
	payloadCount.Set(float64(len(s.fullNodeByPayload)))

	// make this node head if the empty node was
	if s.headNode.block == n.block {
		s.headNode = n
	}
	if b.slot == s.highestReceivedNode.block.slot {
		s.highestReceivedNode = n
	}
	return nil
}

// InsertPayloadEnvelope adds a full node to forkchoice from the given payload
// envelope.
func (f *ForkChoice) InsertPayloadEnvelope(envelope interfaces.ROExecutionPayloadEnvelope) error {
	s := f.store
	b, ok := s.emptyNodeByRoot[envelope.BeaconBlockRoot()]
	if !ok {
		return ErrNilNode
	}
	e, err := envelope.Execution()
	if err != nil {
		return err
	}
	return f.insertExecutionPayload(b.block, e)
}

func (s *Store) updateWithPayload(n *Node) {
	for _, node := range s.emptyNodeByRoot {
		if node.bestDescendant != nil && node.bestDescendant.block == n.block {
			node.bestDescendant = n
		}
	}
	for _, node := range s.fullNodeByPayload {
		if node.bestDescendant != nil && node.bestDescendant.block == n.block {
			node.bestDescendant = n
		}
	}
}
