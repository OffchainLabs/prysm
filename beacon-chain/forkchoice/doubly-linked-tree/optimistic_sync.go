package doublylinkedtree

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/pkg/errors"
)

// setOptimisticToInvalid removes invalid nodes from forkchoice. It does NOT remove the empty node for the passed root.
func (s *Store) setOptimisticToInvalid(ctx context.Context, root, parentRoot, parentHash, lastValidHash [32]byte) ([][32]byte, error) {
	invalidRoots := make([][32]byte, 0)
	n := s.fullNodeByRoot[root]
	if n == nil {
		// The offending node with its payload is not in forkchoice. Try with the parent
		n = s.emptyNodeByRoot[parentRoot]
		if n == nil {
			return invalidRoots, errors.Wrap(ErrNilNode, "could not set node to invalid, could not find consensus parent")
		}
		if n.node.payloadHash == lastValidHash {
			// The parent node must have been full and with a valid payload
			return invalidRoots, nil
		}
		if n.node.payloadHash == parentHash {
			// The parent was full and invalid
			n = s.fullNodeByRoot[parentRoot]
			if n == nil {
				return invalidRoots, errors.Wrap(ErrNilNode, "could not set node to invalid, could not find full parent")
			}
		} else {
			// The parent is empty and we don't yet know if it's valid or not
			for n = n.node.parent; n != nil; n = n.node.parent {
				if ctx.Err() != nil {
					return invalidRoots, ctx.Err()
				}
				if n.node.payloadHash == lastValidHash {
					// The node built on empty and the whole chain was valid
					return invalidRoots, nil
				}
				if n.node.payloadHash == parentHash {
					// The parent was full and invalid
					break
				}
			}
			if n == nil {
				return nil, errors.Wrap(ErrNilNode, "could not set node to invalid, could not find full parent in ancestry")
			}
		}
	} else {
		// check consistency with the parent information
		if n.node.parent == nil {
			return nil, ErrNilNode
		}
		if n.node.parent.node.root != parentRoot {
			return nil, errInvalidParentRoot
		}
	}
	// n points to a full node that has an invalid payload in forkchoice. We need to find the fist node in the chain that is actually invalid.
	startNode := n
	fp := s.fullParent(n)
	for ; fp != nil && fp.node.payloadHash != lastValidHash; fp = s.fullParent(fp) {
		if ctx.Err() != nil {
			return invalidRoots, ctx.Err()
		}
		n = fp
	}
	// Deal with the case that the last valid payload is in a different fork
	// This means we are dealing with an EE that does not follow the spec
	if fp == nil {
		// return early if the invalid node was not imported
		if startNode.node.root != root {
			return invalidRoots, nil
		}
		// Remove just the imported invalid root
		n = startNode
	}
	return s.removeNode(ctx, n)
}

// removeNode removes the node with the given root and all of its children
// from the Fork Choice Store.
func (s *Store) removeNode(ctx context.Context, pn *PayloadNode) ([][32]byte, error) {
	invalidRoots := make([][32]byte, 0)

	if pn == nil {
		return invalidRoots, errors.Wrap(ErrNilNode, "could not remove node")
	}
	if !pn.optimistic || pn.node.parent == nil {
		return invalidRoots, errInvalidOptimisticStatus
	}
	children := pn.node.parent.children
	if len(children) == 1 {
		pn.node.parent.children = []*Node{}
	} else {
		for i, n := range children {
			if n == pn.node {
				if i != len(children)-1 {
					children[i] = children[len(children)-1]
				}
				pn.node.parent.children = children[:len(children)-1]
				break
			}
		}
	}
	return s.removeNodeAndChildren(ctx, pn, invalidRoots)
}

// removeNodeAndChildren removes `node` and all of its descendant from the Store
func (s *Store) removeNodeAndChildren(ctx context.Context, pn *PayloadNode, invalidRoots [][32]byte) ([][32]byte, error) {
	var err error
	// If we are removing an empty node, then remove the full node as well if it exists.
	if !pn.full {
		fn, ok := s.fullNodeByRoot[pn.node.root]
		if ok {
			invalidRoots, err = s.removeNodeAndChildren(ctx, fn, invalidRoots)
			if err != nil {
				return invalidRoots, err
			}
		}
	}
	// Now we remove the full node's children.
	for _, child := range pn.children {
		if ctx.Err() != nil {
			return invalidRoots, ctx.Err()
		}
		// We need to remove only the empty node here since the recursion will take care of the full one.
		en := s.emptyNodeByRoot[child.root]
		if invalidRoots, err = s.removeNodeAndChildren(ctx, en, invalidRoots); err != nil {
			return invalidRoots, err
		}
	}
	// Only append the root for the empty nodes.
	if pn.full {
		delete(s.fullNodeByRoot, pn.node.root)
	} else {
		invalidRoots = append(invalidRoots, pn.node.root)
		if pn.node.root == s.proposerBoostRoot {
			s.proposerBoostRoot = [32]byte{}
		}
		if pn.node.root == s.previousProposerBoostRoot {
			s.previousProposerBoostRoot = params.BeaconConfig().ZeroHash
			s.previousProposerBoostScore = 0
		}
		delete(s.emptyNodeByRoot, pn.node.root)
	}
	return invalidRoots, nil
}
