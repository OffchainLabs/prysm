package doublylinkedtree

import (
	"bytes"
	"context"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	forkchoice2 "github.com/OffchainLabs/prysm/v7/consensus-types/forkchoice"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/pkg/errors"
)

func (s *Store) getNodeInformation(block interfaces.ReadOnlyBeaconBlock, parent **PayloadNode, payloadHash *[32]byte) error {
	sb, err := block.Body().SignedExecutionPayloadBid()
	if err != nil {
		return err
	}
	wb, err := blocks.WrappedROSignedExecutionPayloadBid(sb)
	if err != nil {
		return errors.Wrap(err, "failed to wrap signed bid")
	}
	bid, err := wb.Bid()
	if err != nil {
		return errors.Wrap(err, "failed to get bid from wrapped bid")
	}
	*payloadHash = bid.BlockHash()
	parentRoot := block.ParentRoot()
	*parent = s.emptyNodeByRoot[parentRoot]
	if *parent == nil {
		// This is the tree root node.
		return nil
	}
	if bid.ParentBlockHash() == (*parent).node.payloadHash {
		//block builds on full
		*parent = s.fullNodeByRoot[(*parent).node.payloadHash]
	}
	return nil
}

// applyWeightChangesConsensusNode recomputes the weight of the node passed as an argument and all of its descendants,
// using the current balance stored in each node.
func (s *Store) applyWeightChangesConsensusNode(ctx context.Context, n *Node) error {
	// Recursively calling the children to sum their weights.
	en := s.emptyNodeByRoot[n.root]
	if err := s.applyWeightChangesPayloadNode(ctx, en); err != nil {
		return err
	}
	childrenWeight := en.weight
	fn := s.fullNodeByRoot[n.root]
	if fn != nil {
		if err := s.applyWeightChangesPayloadNode(ctx, fn); err != nil {
			return err
		}
		childrenWeight += fn.weight
	}
	if n.root == params.BeaconConfig().ZeroHash {
		return nil
	}
	n.weight = n.balance + childrenWeight
	return nil
}

// applyWeightChangesPayloadNode recomputes the weight of the node passed as an argument and all of its descendants,
// using the current balance stored in each node.
func (s *Store) applyWeightChangesPayloadNode(ctx context.Context, n *PayloadNode) error {
	// Recursively calling the children to sum their weights.
	childrenWeight := uint64(0)
	for _, child := range n.children {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.applyWeightChangesConsensusNode(ctx, child); err != nil {
			return err
		}
		childrenWeight += child.weight
	}
	n.weight = n.balance + childrenWeight
	return nil
}

// allConsensusChildren returns the list of all consensus blocks that build on the given node.
func (s *Store) allConsensusChildren(n *Node) []*Node {
	en := s.emptyNodeByRoot[n.root]
	fn, ok := s.fullNodeByRoot[n.root]
	if ok {
		return append(en.children, fn.children...)
	}
	return en.children
}

// setNodeAndParentValidated sets the current node and all the ancestors as validated (i.e. non-optimistic).
func (s *Store) setNodeAndParentValidated(ctx context.Context, pn *PayloadNode) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if !pn.optimistic {
		return nil
	}
	pn.optimistic = false
	if pn.full {
		// set the empty node also a as valid
		en := s.emptyNodeByRoot[pn.node.root]
		en.optimistic = false
	}
	if pn.node.parent == nil {
		return nil
	}
	return s.setNodeAndParentValidated(ctx, pn.node.parent)
}

// fullAncestor returns the highest ancestor with a full payload that a block with the
// given root has. If there is a payload for the past root, then it will return that full
// node. Otherwise it will use the full parent actually being an acestor of the given root
func (s *Store) fullAncestor(root [32]byte) *PayloadNode {
	fn, ok := s.fullNodeByRoot[root]
	if ok {
		return fn
	}
	en := s.emptyNodeByRoot[root]
	if en == nil {
		return nil
	}
	return s.fullParent(en)
}

// fullParent returns the latest full node that this block builds on.
func (s *Store) fullParent(pn *PayloadNode) *PayloadNode {
	parent := pn.node.parent
	for ; parent != nil && !parent.full; parent = parent.node.parent {
	}
	return parent
}

// parentHash return the payload hash of the latest full node that this block builds on.
func (s *Store) parentHash(pn *PayloadNode) [32]byte {
	fullParent := s.fullParent(pn)
	if fullParent == nil {
		return [32]byte{}
	}
	return fullParent.node.payloadHash
}

// latestHashForRoot returns the latest payload hash for the given block root.
func (s *Store) latestHashForRoot(root [32]byte) [32]byte {
	// try to get the full node first
	fn, ok := s.fullNodeByRoot[root]
	if ok && fn != nil {
		return fn.node.payloadHash
	}
	en := s.emptyNodeByRoot[root]
	if !ok || en == nil {
		// This should not happen
		return [32]byte{}
	}
	return s.parentHash(en)
}

// updateBestDescendantPayloadNode updates the best descendant of this node and its
// children.
func (s *Store) updateBestDescendantPayloadNode(ctx context.Context, n *PayloadNode, justifiedEpoch, finalizedEpoch, currentEpoch primitives.Epoch) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	var bestChild *Node
	bestWeight := uint64(0)
	for _, child := range n.children {
		if child == nil {
			return errors.Wrap(ErrNilNode, "could not update best descendant")
		}
		if err := s.updateBestDescendantConsensusNode(ctx, child, justifiedEpoch, finalizedEpoch, currentEpoch); err != nil {
			return err
		}
		childLeadsToViableHead := child.leadsToViableHead(justifiedEpoch, currentEpoch)
		if childLeadsToViableHead && bestChild == nil {
			// The child leads to a viable head, but the current
			// parent's best child doesn't.
			bestWeight = child.weight
			bestChild = child
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
	if bestChild == nil {
		n.bestDescendant = nil
	} else {
		if bestChild.bestDescendant == nil {
			n.bestDescendant = bestChild
		} else {
			n.bestDescendant = bestChild.bestDescendant
		}
	}
	return nil
}

// updateBestDescendantConsensusNode updates the best descendant of this node and its
// children.
func (s *Store) updateBestDescendantConsensusNode(ctx context.Context, n *Node, justifiedEpoch, finalizedEpoch, currentEpoch primitives.Epoch) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(s.allConsensusChildren(n)) == 0 {
		n.bestDescendant = nil
		return nil
	}

	en := s.emptyNodeByRoot[n.root]
	if err := s.updateBestDescendantPayloadNode(ctx, en, justifiedEpoch, finalizedEpoch, currentEpoch); err != nil {
		return err
	}
	fn := s.fullNodeByRoot[n.root]
	if fn == nil {
		n.bestDescendant = en.bestDescendant
		return nil
	}
	// TODO GLOAS: pick between full or empty
	if err := s.updateBestDescendantPayloadNode(ctx, fn, justifiedEpoch, finalizedEpoch, currentEpoch); err != nil {
		return err
	}
	n.bestDescendant = fn.bestDescendant
	return nil
}

// choosePayloadContent chooses between empty or full for the passed consensus node. TODO Gloas: use PTC to choose.
func (s *Store) choosePayloadContent(n *Node) *PayloadNode {
	if n == nil {
		return nil
	}
	fn := s.fullNodeByRoot[n.root]
	if fn != nil {
		return fn
	}
	return s.emptyNodeByRoot[n.root]
}

// nodeTreeDump appends to the given list all the nodes descending from this one
func (s *Store) nodeTreeDump(ctx context.Context, n *Node, nodes []*forkchoice2.Node) ([]*forkchoice2.Node, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var parentRoot [32]byte
	if n.parent != nil {
		parentRoot = n.parent.node.root
	}
	target := [32]byte{}
	if n.target != nil {
		target = n.target.root
	}
	optimistic := n.parent.optimistic
	en := s.emptyNodeByRoot[n.root]
	timestamp := en.timestamp
	fn := s.fullNodeByRoot[n.root]
	if fn != nil {
		optimistic = fn.optimistic
		timestamp = fn.timestamp
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
		ExecutionOptimistic:      optimistic,
		ExecutionBlockHash:       n.payloadHash[:],
		Timestamp:                timestamp,
		Target:                   target[:],
	}
	if optimistic {
		thisNode.Validity = forkchoice2.Optimistic
	} else {
		thisNode.Validity = forkchoice2.Valid
	}

	nodes = append(nodes, thisNode)
	var err error
	children := s.allConsensusChildren(n)
	for _, child := range children {
		nodes, err = s.nodeTreeDump(ctx, child, nodes)
		if err != nil {
			return nil, err
		}
	}
	return nodes, nil
}
