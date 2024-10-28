package doublylinkedtree

import (
	"context"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/config/params"
)

func (s *Store) removeNodeIfSynced(ctx context.Context, node *Node) ([][32]byte, error) {
	invalidRoots := make([][32]byte, 0)
	if node == nil {
		return invalidRoots, nil
	}
	fullNode := s.fullNodeByPayload[node.block.payloadHash]
	if fullNode == nil {
		return invalidRoots, nil
	}
	return s.removeNode(ctx, fullNode)
}

func (s *Store) setOptimisticToInvalid(ctx context.Context, root, parentRoot, lastValidHash [32]byte) ([][32]byte, error) {
	node := s.emptyNodeByRoot[root]
	// if the last valid hash is not known or null, prune only the incoming
	// block.
	lastValid, ok := s.fullNodeByPayload[lastValidHash]
	if !ok || lastValidHash == [32]byte{} {
		return s.removeNodeIfSynced(ctx, node)
	}
	// We have a valid hash, find if it's in the same fork as the last valid
	// root.
	invalidRoots := make([][32]byte, 0)
	ancestor, err := s.ancestorRoot(ctx, parentRoot, lastValid.block.slot)
	if err != nil {
		return invalidRoots, errors.Wrap(err, "could not set block as invalid")
	}
	if ancestor != lastValid.block.root {
		return s.removeNodeIfSynced(ctx, node)
	}
	// we go up we find a child of the last valid that is full. We find
	// first the starting node for the loop
	if node == nil {
		node = s.emptyNodeByRoot[parentRoot]
		fullParent, ok := s.fullNodeByPayload[node.block.payloadHash]
		if ok {
			// return early if the parent is the LVH
			if fullParent.block.payloadHash == lastValidHash {
				return invalidRoots, nil
			}
			node = fullParent
		}
	} else {
		fullNode, ok := s.fullNodeByPayload[node.block.payloadHash]
		if ok {
			node = fullNode
		}
	}
	var lastFullNode *Node
	for ; node.block.fullParent != nil; node = node.block.fullParent {
		if node.full {
			lastFullNode = node
		}
		if node.block.fullParent.block.payloadHash == lastValidHash {
			break
		}
	}
	if lastFullNode == nil {
		return invalidRoots, nil
	}
	return s.removeNode(ctx, lastFullNode)
}

// removeNode removes the node with the given root and all of its children
// from the Fork Choice Store.
func (s *Store) removeNode(ctx context.Context, node *Node) ([][32]byte, error) {
	invalidRoots := make([][32]byte, 0)

	if node == nil {
		return invalidRoots, errors.Wrap(ErrNilNode, "could not remove node")
	}
	if !node.optimistic || node.block.parent == nil {
		return invalidRoots, errInvalidOptimisticStatus
	}

	children := node.block.parent.children
	if len(children) == 1 {
		node.block.parent.children = []*Node{}
	} else {
		for i, n := range children {
			if n == node {
				if i != len(children)-1 {
					children[i] = children[len(children)-1]
				}
				node.block.parent.children = children[:len(children)-1]
				break
			}
		}
	}
	return s.removeNodeAndChildren(ctx, node, invalidRoots)
}

// removeNodeAndChildren removes `node` and all of its descendant from the Store
func (s *Store) removeNodeAndChildren(ctx context.Context, node *Node, invalidRoots [][32]byte) ([][32]byte, error) {
	var err error
	for _, child := range node.children {
		if ctx.Err() != nil {
			return invalidRoots, ctx.Err()
		}
		if invalidRoots, err = s.removeNodeAndChildren(ctx, child, invalidRoots); err != nil {
			return invalidRoots, err
		}
	}
	invalidRoots = append(invalidRoots, node.block.root)
	if node.block.root == s.proposerBoostRoot {
		s.proposerBoostRoot = [32]byte{}
	}
	if node.block.root == s.previousProposerBoostRoot {
		s.previousProposerBoostRoot = params.BeaconConfig().ZeroHash
		s.previousProposerBoostScore = 0
	}
	if node.block.root == s.payloadWithholdBoostRoot {
		s.payloadWithholdBoostRoot = [32]byte{}
	}
	if node.block.root == s.payloadRevealBoostRoot {
		s.payloadRevealBoostRoot = [32]byte{}
	}
	delete(s.emptyNodeByRoot, node.block.root)
	delete(s.fullNodeByPayload, node.block.payloadHash)
	return invalidRoots, nil
}
