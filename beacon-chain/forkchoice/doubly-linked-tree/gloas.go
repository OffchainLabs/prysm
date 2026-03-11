package doublylinkedtree

import (
	"bytes"
	"context"
	"slices"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	forkchoice2 "github.com/OffchainLabs/prysm/v7/consensus-types/forkchoice"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

// CanonicalNodeAtSlot returns the full node that exists at the given slot in the canonical chain.
// The boolean indicates whether the payload was present for the returned blockroot.
// If the slot for the given blockroot is the current wall clock slot, it returns the pending node, that is, it sets the boolean to false.
// If the slot is in the past the boolean indicates between full or empty.
func (f *ForkChoice) CanonicalNodeAtSlot(slot primitives.Slot) ([32]byte, bool) {
	s := f.store
	n := s.headNode
	for n != nil && n.slot > slot {
		if n.parent == nil {
			n = nil
		} else {
			n = n.parent.node
		}
	}
	if n == nil {
		return [32]byte{}, false
	}
	if n.slot == s.currentSlot() {
		return n.root, false
	}
	pn := s.choosePayloadContent(n)
	return pn.node.root, pn.full
}

func (s *Store) resolveParentPayloadStatus(block interfaces.ReadOnlyBeaconBlock, parent **PayloadNode, blockHash *[32]byte) error {
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
	*blockHash = bid.BlockHash()
	parentRoot := block.ParentRoot()
	*parent = s.emptyNodeByRoot[parentRoot]
	if *parent == nil {
		// This is the tree root node.
		return nil
	}
	if bid.ParentBlockHash() == (*parent).node.blockHash {
		// block builds on full
		*parent = s.fullNodeByRoot[(*parent).node.root]
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
		return append(slices.Clone(en.children), fn.children...)
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
	return fullParent.node.blockHash
}

// latestHashForRoot returns the latest payload hash for the given block root.
func (s *Store) latestHashForRoot(root [32]byte) [32]byte {
	// try to get the full node first
	fn := s.fullNodeByRoot[root]
	if fn != nil {
		return fn.node.blockHash
	}
	en := s.emptyNodeByRoot[root]
	if en == nil {
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
	if err := s.updateBestDescendantPayloadNode(ctx, fn, justifiedEpoch, finalizedEpoch, currentEpoch); err != nil {
		return err
	}
	n.bestDescendant = s.choosePayloadContent(n).bestDescendant
	return nil
}

func (s *Store) currentSlot() primitives.Slot {
	return slots.CurrentSlot(s.genesisTime)
}

func (s *Store) shouldExtendPayload(fn *PayloadNode) bool {
	if fn == nil {
		return false
	}
	n := fn.node
	if n.payloadAvailabilityVote.Count() > fieldparams.PTCSize/2 && n.payloadDataAvailabilityVote.Count() > fieldparams.PTCSize/2 {
		return true
	}
	if s.proposerBoostRoot == [32]byte{} {
		return true
	}
	pn := s.emptyNodeByRoot[s.proposerBoostRoot]
	if pn == nil {
		return true
	}
	if pn.node.parent.node != fn.node {
		return true
	}
	return pn.node.parent.full
}

// choosePayloadContent chooses between empty or full for the passed consensus node.
func (s *Store) choosePayloadContent(n *Node) *PayloadNode {
	if n == nil {
		return nil
	}
	fn := s.fullNodeByRoot[n.root]
	en := s.emptyNodeByRoot[n.root]
	if fn == nil {
		return en
	}
	if fn.weight > en.weight {
		return fn
	} else if fn.weight < en.weight {
		return en
	}
	previousSlot := n.slot+1 == s.currentSlot()
	if !previousSlot || s.shouldExtendPayload(fn) {
		return fn
	}
	return en
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
	optimistic := false
	if n.parent != nil {
		optimistic = n.parent.optimistic
	}
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
		ExecutionBlockHash:       n.blockHash[:],
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

// InsertPayload inserts a full node into forkchoice after the Gloas fork.
func (f *ForkChoice) InsertPayload(pe interfaces.ROExecutionPayloadEnvelope) error {
	if pe.IsNil() {
		return errors.New("cannot insert nil payload")
	}
	s := f.store
	root := pe.BeaconBlockRoot()
	en := s.emptyNodeByRoot[root]
	if en == nil {
		return errors.Wrap(ErrNilNode, "cannot insert full node without an empty one")
	}
	if _, ok := s.fullNodeByRoot[root]; ok {
		// We don't import two payloads for the same root
		return nil
	}
	fn := &PayloadNode{
		node:       en.node,
		optimistic: true,
		timestamp:  time.Now(),
		full:       true,
		children:   make([]*Node, 0),
	}
	s.fullNodeByRoot[root] = fn
	f.updateNewFullNodeWeight(fn)
	return nil
}

func (f *ForkChoice) updateNewFullNodeWeight(fn *PayloadNode) {
	for index, vote := range f.votes {
		if vote.currentRoot == fn.node.root && vote.nextPayloadStatus && index < len(f.balances) {
			fn.balance += f.balances[index]
		}
	}
	fn.weight = fn.balance
}

// SetPTCVote sets the PTC vote bits on the consensus node identified by root.
func (f *ForkChoice) SetPTCVote(root [32]byte, ptcIdx uint64, payloadPresent, blobDataAvailable bool) {
	n := f.store.emptyNodeByRoot[root]
	if n == nil {
		return
	}
	if payloadPresent {
		n.node.setPayloadAvailabilityVote(ptcIdx)
	}
	if blobDataAvailable {
		n.node.setPayloadDataAvailabilityVote(ptcIdx)
	}
}

func (n *Node) setPayloadAvailabilityVote(idx uint64) {
	n.payloadAvailabilityVote.SetBitAt(idx, true)
}

func (n *Node) setPayloadDataAvailabilityVote(idx uint64) {
	n.payloadDataAvailabilityVote.SetBitAt(idx, true)
}

func (n *Node) payloadAvailabilityVoteCount() uint64 {
	return n.payloadAvailabilityVote.Count()
}

func (n *Node) payloadDataAvailabilityVoteCount() uint64 {
	return n.payloadDataAvailabilityVote.Count()
}

// resolveVoteNode returns the node that should receive the balance of a vote. It returns always a PayloadNode, but the boolean indicates
// whether the vote should be applied to the pending node (true) or not.
func (s *Store) resolveVoteNode(r [32]byte, slot primitives.Slot, payloadStatus bool) (*PayloadNode, bool) {
	en := s.emptyNodeByRoot[r]
	if en == nil {
		return nil, true
	}
	if payloadStatus {
		return s.fullNodeByRoot[r], false
	}
	return en, slot == en.node.slot
}

// HasFullNode returns true if a full (payload) node exists for the given beacon block root.
func (f *ForkChoice) HasFullNode(root [32]byte) bool {
	_, ok := f.store.fullNodeByRoot[root]
	return ok
}

// BlockHash returns the hash committed in the given block
func (f *ForkChoice) BlockHash(root [32]byte) ([32]byte, error) {
	s := f.store
	en := s.emptyNodeByRoot[root]
	if en == nil || en.node == nil {
		return [32]byte{}, errors.Wrap(ErrNilNode, "could not get block hash for root")
	}
	return en.node.blockHash, nil
}

func (s *Store) shouldApplyProposerBoost() bool {
	if s.proposerBoostRoot == [32]byte{} {
		return false
	}
	if slots.ToEpoch(s.currentSlot()) < params.BeaconConfig().GloasForkEpoch {
		return true
	}
	en := s.emptyNodeByRoot[s.proposerBoostRoot]
	if en == nil {
		return false
	}
	n := en.node
	p := n.parent
	if p == nil {
		return true
	}

	if p.node.slot+1 != n.slot {
		return true
	}
	return p.weight*100 >= s.committeeWeight*params.BeaconConfig().ReorgHeadWeightThreshold
}

// removeProposerBoostFromParent removes the proposer boost that must have been applied to the parent of the current proposer boost node
// in some circumstances.
func (s *Store) removeProposerBoostFromParent() {
	if s.proposerBoostRoot == [32]byte{} {
		return
	}
	pn := s.emptyNodeByRoot[s.proposerBoostRoot]
	if pn == nil {
		return
	}
	n := pn.node
	p := n.parent
	if p.node.slot+1 != s.currentSlot() {
		return
	}
	if p.weight < s.previousProposerBoostScore {
		p.weight = 0
	} else {
		p.weight -= s.previousProposerBoostScore
	}
	return
}

// FullHead returns the head root and the head blockhash of the chain. It also
// returns whether the head is full or not, that is if the blockhash is for the same
// slot as the beacon root or some previous slots.
func (f *ForkChoice) FullHead(ctx context.Context) ([32]byte, [32]byte, bool, error) {
	hr, err := f.Head(ctx)
	if err != nil {
		return [32]byte{}, [32]byte{}, false, err
	}
	n := f.store.headNode
	pn := f.store.choosePayloadContent(n)
	if pn.full {
		return hr, pn.node.blockHash, true, nil
	}
	fullAncestor := f.store.fullParent(pn)
	if fullAncestor == nil {
		return hr, [32]byte{}, false, nil
	}
	return hr, fullAncestor.node.blockHash, false, nil
}
