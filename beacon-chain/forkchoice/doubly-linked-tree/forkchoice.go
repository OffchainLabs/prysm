package doublylinkedtree

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensus_blocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	forkchoice2 "github.com/OffchainLabs/prysm/v7/consensus-types/forkchoice"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// New initializes a new fork choice store.
func New() *ForkChoice {
	s := &Store{
		justifiedCheckpoint:           &forkchoicetypes.Checkpoint{},
		unrealizedJustifiedCheckpoint: &forkchoicetypes.Checkpoint{},
		unrealizedFinalizedCheckpoint: &forkchoicetypes.Checkpoint{},
		prevJustifiedCheckpoint:       &forkchoicetypes.Checkpoint{},
		finalizedCheckpoint:           &forkchoicetypes.Checkpoint{},
		proposerBoostRoot:             [32]byte{},
		emptyNodeByRoot:               make(map[[fieldparams.RootLength]byte]*PayloadNode),
		fullNodeByRoot:                make(map[[fieldparams.RootLength]byte]*PayloadNode),
		slashedIndices:                make(map[primitives.ValidatorIndex]bool),
		receivedBlocksLastEpoch:       [fieldparams.SlotsPerEpoch]primitives.Slot{},
	}

	b := make([]uint64, 0)
	v := make([]Vote, 0)
	return &ForkChoice{store: s, balances: b, votes: v}
}

// NodeCount returns the current number of nodes in the Store.
func (f *ForkChoice) NodeCount() int {
	return len(f.store.emptyNodeByRoot)
}

// Head returns the head root from fork choice store.
// It firsts computes validator's balance changes then recalculates block tree from leaves to root.
func (f *ForkChoice) Head(
	ctx context.Context,
) ([32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.Head")
	defer span.End()

	calledHeadCount.Inc()

	if err := f.updateBalances(); err != nil {
		return [32]byte{}, errors.Wrap(err, "could not update balances")
	}

	if err := f.applyProposerBoostScore(); err != nil {
		return [32]byte{}, errors.Wrap(err, "could not apply proposer boost score")
	}

	if err := f.store.applyWeightChangesConsensusNode(ctx, f.store.treeRootNode); err != nil {
		return [32]byte{}, errors.Wrap(err, "could not apply weight changes")
	}
	f.store.removeProposerBoostFromParent()

	jc := f.JustifiedCheckpoint()
	fc := f.FinalizedCheckpoint()
	currentEpoch := slots.EpochsSinceGenesis(f.store.genesisTime)
	if err := f.store.updateBestDescendantConsensusNode(ctx, f.store.treeRootNode, jc.Epoch, fc.Epoch, currentEpoch); err != nil {
		return [32]byte{}, errors.Wrap(err, "could not update best descendant")
	}
	return f.store.head(ctx)
}

// ProcessAttestation processes attestation for vote accounting, it iterates around validator indices
// and update their votes accordingly.
func (f *ForkChoice) ProcessAttestation(ctx context.Context, validatorIndices []uint64, blockRoot [32]byte, slot primitives.Slot, payloadStatus bool) {
	_, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.ProcessAttestation")
	defer span.End()

	for _, index := range validatorIndices {
		// Validator indices will grow the vote cache.
		newVote := false
		for index >= uint64(len(f.votes)) {
			f.votes = append(f.votes, Vote{currentRoot: params.BeaconConfig().ZeroHash, nextRoot: params.BeaconConfig().ZeroHash})
			newVote = true
		}

		// Vote gets updated if it's newly allocated or high target epoch.
		targetEpoch := slots.ToEpoch(slot)
		nextEpoch := slots.ToEpoch(f.votes[index].nextSlot)
		if newVote || targetEpoch > nextEpoch {
			f.votes[index].nextSlot = slot
			f.votes[index].nextRoot = blockRoot
			f.votes[index].nextPayloadStatus = payloadStatus
		}
	}

	processedAttestationCount.Inc()
}

// InsertNode processes a new block by inserting it to the fork choice store.
func (f *ForkChoice) InsertNode(ctx context.Context, state state.BeaconState, roblock consensus_blocks.ROBlock) error {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.InsertNode")
	defer span.End()

	jc := state.CurrentJustifiedCheckpoint()
	if jc == nil {
		return errInvalidNilCheckpoint
	}
	justifiedEpoch := jc.Epoch
	fc := state.FinalizedCheckpoint()
	if fc == nil {
		return errInvalidNilCheckpoint
	}
	finalizedEpoch := fc.Epoch
	pn, err := f.store.insert(ctx, roblock, justifiedEpoch, finalizedEpoch)
	if err != nil {
		return err
	}

	jc, fc = f.store.pullTips(state, pn.node, jc, fc)
	if err := f.updateCheckpoints(ctx, jc, fc); err != nil {
		_, remErr := f.store.removeNode(ctx, pn)
		if remErr != nil {
			log.WithError(remErr).Error("Could not remove node")
		}
		return errors.Wrap(err, "could not update checkpoints")
	}
	return nil
}

// updateCheckpoints update the checkpoints when inserting a new node.
func (f *ForkChoice) updateCheckpoints(ctx context.Context, jc, fc *ethpb.Checkpoint) error {
	if jc.Epoch > f.store.justifiedCheckpoint.Epoch {
		f.store.prevJustifiedCheckpoint = f.store.justifiedCheckpoint
		jcRoot := bytesutil.ToBytes32(jc.Root)
		f.store.justifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: jc.Epoch, Root: jcRoot}
		if err := f.updateJustifiedBalances(ctx, jcRoot); err != nil {
			return errors.Wrap(err, "could not update justified balances")
		}
	}
	// Update finalization
	if fc.Epoch <= f.store.finalizedCheckpoint.Epoch {
		return nil
	}
	f.store.finalizedCheckpoint = &forkchoicetypes.Checkpoint{
		Epoch: fc.Epoch,
		Root:  bytesutil.ToBytes32(fc.Root),
	}
	return f.store.prune(ctx)
}

// HasNode returns true if the node exists in fork choice store,
// false else wise.
func (f *ForkChoice) HasNode(root [32]byte) bool {
	_, ok := f.store.emptyNodeByRoot[root]
	return ok
}

// IsCanonical returns true if the given root is part of the canonical chain.
func (f *ForkChoice) IsCanonical(root [32]byte) bool {
	// It is fine to pick empty node here since we only check if the beacon block is canonical.
	pn, ok := f.store.emptyNodeByRoot[root]
	if !ok || pn == nil {
		return false
	}

	if pn.node.bestDescendant == nil {
		// The node doesn't have any children
		if f.store.headNode.bestDescendant == nil {
			// headNode is itself head.
			return pn.node == f.store.headNode
		}
		// headNode is not actualized and there are some descendants
		return pn.node == f.store.headNode.bestDescendant
	}
	// The node has children
	if f.store.headNode.bestDescendant == nil {
		return pn.node.bestDescendant == f.store.headNode
	}
	return pn.node.bestDescendant == f.store.headNode.bestDescendant
}

// IsOptimistic returns true if the given root has been optimistically synced.
// TODO: Gloas, the current implementation uses the result of the full block for
// the given root. In gloas this would be incorrect and we should specify the
// payload content, thus we should expose a full/empty version of this call.
func (f *ForkChoice) IsOptimistic(root [32]byte) (bool, error) {
	if f.store.allTipsAreInvalid {
		return true, nil
	}

	en, ok := f.store.emptyNodeByRoot[root]
	if !ok || en == nil {
		return true, ErrNilNode
	}
	fn := f.store.fullNodeByRoot[root]
	if fn != nil {
		return fn.optimistic, nil
	}

	return en.optimistic, nil
}

// AncestorRoot returns the ancestor root of input block root at a given slot.
func (f *ForkChoice) AncestorRoot(ctx context.Context, root [32]byte, slot primitives.Slot) ([32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.AncestorRoot")
	defer span.End()

	pn, ok := f.store.emptyNodeByRoot[root]
	if !ok || pn == nil {
		return [32]byte{}, errors.Wrap(ErrNilNode, "could not determine ancestor root")
	}

	n := pn.node
	for n.slot > slot {
		if ctx.Err() != nil {
			return [32]byte{}, ctx.Err()
		}
		if n.parent == nil {
			n = nil
			break
		}
		n = n.parent.node
	}

	if n == nil {
		return [32]byte{}, errors.Wrap(ErrNilNode, "could not determine ancestor root")
	}

	return n.root, nil
}

// IsViableForCheckpoint returns whether the root passed is a checkpoint root for any
// known chain in forkchoice.
func (f *ForkChoice) IsViableForCheckpoint(cp *forkchoicetypes.Checkpoint) (bool, error) {
	pn, ok := f.store.emptyNodeByRoot[cp.Root]
	if !ok || pn == nil {
		return false, nil
	}
	node := pn.node
	epochStart, err := slots.EpochStart(cp.Epoch)
	if err != nil {
		return false, err
	}
	if node.slot > epochStart {
		return false, nil
	}

	// If it's the start of the epoch, it is a checkpoint
	if node.slot == epochStart {
		return true, nil
	}
	// If there are no descendants of this beacon block, it is is viable as a checkpoint
	children := f.store.allConsensusChildren(node)
	if len(children) == 0 {
		return true, nil
	}
	if !features.Get().IgnoreUnviableAttestations {
		// Allow any node from the checkpoint epoch - 1 to be viable.
		nodeEpoch := slots.ToEpoch(node.slot)
		if nodeEpoch+1 == cp.Epoch {
			return true, nil
		}
	}
	// If some child is after the start of the epoch, the checkpoint is viable.
	for _, child := range children {
		if child.slot > epochStart {
			return true, nil
		}
	}
	return false, nil
}

// updateBalances updates the balances that directly voted for each block taking into account the
// validators' latest votes.
func (f *ForkChoice) updateBalances() error {
	newBalances := f.justifiedBalances
	zHash := params.BeaconConfig().ZeroHash

	for index := 0; index < len(f.votes); index++ {
		// Skip if validator has been slashed
		if f.store.slashedIndices[primitives.ValidatorIndex(index)] {
			continue
		}
		vote := &f.votes[index]
		// Skip if validator has never voted for current root and next root (i.e. if the
		// votes are zero hash aka genesis block), there's nothing to compute.
		if vote.currentRoot == zHash && vote.nextRoot == zHash {
			continue
		}

		oldBalance := uint64(0)
		newBalance := uint64(0)
		// If the validator index did not exist in `f.balances` or
		// `newBalances` list above, the balance is just 0.
		if index < len(f.balances) {
			oldBalance = f.balances[index]
		}
		if index < len(newBalances) {
			newBalance = newBalances[index]
		}

		if oldBalance != newBalance || vote.currentSlot != vote.nextSlot {
			// Add new balance to the next vote target if the root is known.
			pn, pending := f.store.resolveVoteNode(vote.nextRoot, vote.nextSlot, vote.nextPayloadStatus)
			if pn != nil && vote.nextRoot != zHash {
				if pending {
					pn.node.balance += newBalance
				} else {
					pn.balance += newBalance
				}
			}

			// Subtract old balance from the current vote target if the root is known.
			pn, pending = f.store.resolveVoteNode(vote.currentRoot, vote.currentSlot, vote.currentPayloadStatus)
			if pn != nil && vote.currentRoot != zHash {
				if pending {
					if pn.node.balance < oldBalance {
						if pn.node.slot == 0 {
							log.WithField("nodeRoot", fmt.Sprintf("%#x", bytesutil.Trunc(vote.currentRoot[:]))).
								Debug("Genesis node pending balance underflow, clamping to zero")
						} else {
							log.WithFields(logrus.Fields{
								"nodeRoot":                   fmt.Sprintf("%#x", bytesutil.Trunc(vote.currentRoot[:])),
								"oldBalance":                 oldBalance,
								"nodeBalance":                pn.node.balance,
								"nodeWeight":                 pn.node.weight,
								"proposerBoostRoot":          fmt.Sprintf("%#x", bytesutil.Trunc(f.store.proposerBoostRoot[:])),
								"previousProposerBoostRoot":  fmt.Sprintf("%#x", bytesutil.Trunc(f.store.previousProposerBoostRoot[:])),
								"previousProposerBoostScore": f.store.previousProposerBoostScore,
							}).Warning("node with invalid balance, setting it to zero")
						}
						pn.node.balance = 0
					} else {
						pn.node.balance -= oldBalance
					}
				} else {
					if pn.balance < oldBalance {
						log.WithFields(logrus.Fields{
							"nodeRoot":                   fmt.Sprintf("%#x", bytesutil.Trunc(vote.currentRoot[:])),
							"oldBalance":                 oldBalance,
							"nodeBalance":                pn.balance,
							"nodeWeight":                 pn.weight,
							"proposerBoostRoot":          fmt.Sprintf("%#x", bytesutil.Trunc(f.store.proposerBoostRoot[:])),
							"previousProposerBoostRoot":  fmt.Sprintf("%#x", bytesutil.Trunc(f.store.previousProposerBoostRoot[:])),
							"previousProposerBoostScore": f.store.previousProposerBoostScore,
						}).Warning("node with invalid balance, setting it to zero")
						pn.balance = 0
					} else {
						pn.balance -= oldBalance
					}
				}
			}
		}
		// Rotate the validator vote.
		f.votes[index].currentRoot = vote.nextRoot
		f.votes[index].currentSlot = vote.nextSlot
		f.votes[index].currentPayloadStatus = vote.nextPayloadStatus
	}
	f.balances = newBalances
	return nil
}

// Tips returns a list of possible heads from fork choice store, it returns the
// roots and the slots of the leaf nodes.
func (f *ForkChoice) Tips() ([][32]byte, []primitives.Slot) {
	return f.store.tips()
}

// ProposerBoost returns the proposerBoost of the store
func (f *ForkChoice) ProposerBoost() [fieldparams.RootLength]byte {
	return f.store.proposerBoost()
}

// SetOptimisticToValid sets the node with the given root as a fully validated node. The payload for this root MUST have been processed.
func (f *ForkChoice) SetOptimisticToValid(ctx context.Context, root [fieldparams.RootLength]byte) error {
	fn := f.store.fullNodeByRoot[root]
	if fn == nil {
		fn = f.store.emptyNodeByRoot[root]
		if fn == nil {
			return errors.Wrapf(ErrNilNode, "could not set node to valid, no node found for root: %#x", bytesutil.Trunc(root[:]))
		}
	}
	return f.store.setNodeAndParentValidated(ctx, fn)
}

// PreviousJustifiedCheckpoint of fork choice store.
func (f *ForkChoice) PreviousJustifiedCheckpoint() *forkchoicetypes.Checkpoint {
	return f.store.prevJustifiedCheckpoint
}

// JustifiedCheckpoint of fork choice store.
func (f *ForkChoice) JustifiedCheckpoint() *forkchoicetypes.Checkpoint {
	return f.store.justifiedCheckpoint
}

// FinalizedCheckpoint of fork choice store.
func (f *ForkChoice) FinalizedCheckpoint() *forkchoicetypes.Checkpoint {
	return f.store.finalizedCheckpoint
}

// SetOptimisticToInvalid removes a block with an invalid execution payload from fork choice store
func (f *ForkChoice) SetOptimisticToInvalid(ctx context.Context, root, parentRoot, parentHash, payloadHash [fieldparams.RootLength]byte) ([][32]byte, error) {
	return f.store.setOptimisticToInvalid(ctx, root, parentRoot, parentHash, payloadHash)
}

// InsertSlashedIndex adds the given slashed validator index to the
// store-tracked list. Votes from these validators are not accounted for
// in forkchoice.
func (f *ForkChoice) InsertSlashedIndex(_ context.Context, index primitives.ValidatorIndex) {
	// return early if the index was already included:
	if f.store.slashedIndices[index] {
		return
	}
	f.store.slashedIndices[index] = true

	// Subtract last vote from this equivocating validator

	if index >= primitives.ValidatorIndex(len(f.balances)) {
		return
	}

	if index >= primitives.ValidatorIndex(len(f.votes)) {
		return
	}

	v := f.votes[index]
	pn, pending := f.store.resolveVoteNode(v.currentRoot, v.currentSlot, v.currentPayloadStatus)
	if pn == nil {
		return
	}
	if pending {
		if pn.node.balance < f.balances[index] {
			pn.node.balance = 0
		} else {
			pn.node.balance -= f.balances[index]
		}
		return
	}
	if pn.balance < f.balances[index] {
		pn.balance = 0
	} else {
		pn.balance -= f.balances[index]
	}
}

// UpdateJustifiedCheckpoint sets the justified checkpoint to the given one
func (f *ForkChoice) UpdateJustifiedCheckpoint(ctx context.Context, jc *forkchoicetypes.Checkpoint) error {
	if jc == nil {
		return errInvalidNilCheckpoint
	}
	f.store.prevJustifiedCheckpoint = f.store.justifiedCheckpoint
	f.store.justifiedCheckpoint = jc
	if err := f.updateJustifiedBalances(ctx, jc.Root); err != nil {
		return errors.Wrap(err, "could not update justified balances")
	}
	return nil
}

// UpdateFinalizedCheckpoint sets the finalized checkpoint to the given one
func (f *ForkChoice) UpdateFinalizedCheckpoint(fc *forkchoicetypes.Checkpoint) error {
	if fc == nil {
		return errInvalidNilCheckpoint
	}
	f.store.finalizedCheckpoint = fc
	return nil
}

// CommonAncestor returns the common ancestor root and slot between the two block roots r1 and r2.
// This is payload aware. Consider the following situation
// [A,full] <--- [B, full] <---[C,pending]
//
//	\---------[B, empty] <--[D, pending]
//
// Then even though C and D both descend from the beacon block B, their common ancestor is A.
// Notice that also this function **requires** that the two roots are actually contending blocks! otherwise the
// behavior is not defined.
func (f *ForkChoice) CommonAncestor(ctx context.Context, r1 [32]byte, r2 [32]byte) ([32]byte, primitives.Slot, error) {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.CommonAncestorRoot")
	defer span.End()

	en1, ok := f.store.emptyNodeByRoot[r1]
	if !ok || en1 == nil {
		return [32]byte{}, 0, forkchoice.ErrUnknownCommonAncestor
	}

	// Do nothing if the input roots are the same.
	if r1 == r2 {
		return r1, en1.node.slot, nil
	}

	en2, ok := f.store.emptyNodeByRoot[r2]
	if !ok || en2 == nil {
		return [32]byte{}, 0, forkchoice.ErrUnknownCommonAncestor
	}

	for {
		if ctx.Err() != nil {
			return [32]byte{}, 0, ctx.Err()
		}
		if en1.node.slot > en2.node.slot {
			en1 = en1.node.parent
			// Reaches the end of the tree and unable to find common ancestor.
			// This should not happen at runtime as the finalized
			// node has to be a common ancestor
			if en1 == nil {
				return [32]byte{}, 0, forkchoice.ErrUnknownCommonAncestor
			}
		} else {
			en2 = en2.node.parent
			// Reaches the end of the tree and unable to find common ancestor.
			if en2 == nil {
				return [32]byte{}, 0, forkchoice.ErrUnknownCommonAncestor
			}
		}
		if en1 == en2 {
			return en1.node.root, en1.node.slot, nil
		}
	}
}

// InsertChain inserts all nodes corresponding to blocks in the slice
// `blocks`. This slice must be ordered in increasing slot order and
// each consecutive entry must be a child of the previous one.
// The parent of the first block in this list must already be present in forkchoice.
func (f *ForkChoice) InsertChain(ctx context.Context, chain []*forkchoicetypes.BlockAndCheckpoints) error {
	if len(chain) == 0 {
		return nil
	}
	for _, bcp := range chain {
		if _, err := f.store.insert(ctx,
			bcp.Block,
			bcp.JustifiedCheckpoint.Epoch, bcp.FinalizedCheckpoint.Epoch); err != nil {
			return err
		}
		if bcp.HasPayload {
			root := bcp.Block.Root()
			en := f.store.emptyNodeByRoot[root]
			if en != nil && f.store.fullNodeByRoot[root] == nil {
				f.store.fullNodeByRoot[root] = &PayloadNode{
					node:       en.node,
					optimistic: true,
					timestamp:  time.Now(),
					full:       true,
					children:   make([]*Node, 0),
				}
			}
		}
		if err := f.updateCheckpoints(ctx, bcp.JustifiedCheckpoint, bcp.FinalizedCheckpoint); err != nil {
			return err
		}
	}
	return nil
}

// SetGenesisTime sets the genesisTime tracked by forkchoice
func (f *ForkChoice) SetGenesisTime(genesis time.Time) {
	f.store.genesisTime = genesis.Truncate(time.Second) // Genesis time has a precision of 1 second.
}

// SetOriginRoot sets the genesis block root
func (f *ForkChoice) SetOriginRoot(root [32]byte) {
	f.store.originRoot = root
}

// CachedHeadRoot returns the last cached head root
func (f *ForkChoice) CachedHeadRoot() [32]byte {
	node := f.store.headNode
	if node == nil {
		return [32]byte{}
	}
	return f.store.headNode.root
}

// FinalizedPayloadBlockHash returns the hash of the payload at the finalized checkpoint
func (f *ForkChoice) FinalizedPayloadBlockHash() [32]byte {
	return f.store.checkpointPayloadHashForRoot(f.FinalizedCheckpoint().Root)
}

// JustifiedPayloadBlockHash returns the hash of the payload at the justified checkpoint
func (f *ForkChoice) JustifiedPayloadBlockHash() [32]byte {
	return f.store.checkpointPayloadHashForRoot(f.JustifiedCheckpoint().Root)
}

// UnrealizedJustifiedPayloadBlockHash returns the hash of the payload at the unrealized justified checkpoint
func (f *ForkChoice) UnrealizedJustifiedPayloadBlockHash() [32]byte {
	return f.store.checkpointPayloadHashForRoot(f.store.unrealizedJustifiedCheckpoint.Root)
}

// ForkChoiceDump returns a full dump of forkchoice.
func (f *ForkChoice) ForkChoiceDump(ctx context.Context) (*forkchoice2.Dump, error) {
	jc := &ethpb.Checkpoint{
		Epoch: f.store.justifiedCheckpoint.Epoch,
		Root:  f.store.justifiedCheckpoint.Root[:],
	}
	ujc := &ethpb.Checkpoint{
		Epoch: f.store.unrealizedJustifiedCheckpoint.Epoch,
		Root:  f.store.unrealizedJustifiedCheckpoint.Root[:],
	}
	fc := &ethpb.Checkpoint{
		Epoch: f.store.finalizedCheckpoint.Epoch,
		Root:  f.store.finalizedCheckpoint.Root[:],
	}
	ufc := &ethpb.Checkpoint{
		Epoch: f.store.unrealizedFinalizedCheckpoint.Epoch,
		Root:  f.store.unrealizedFinalizedCheckpoint.Root[:],
	}
	nodes := make([]*forkchoice2.Node, 0, f.NodeCount())
	var err error
	if f.store.treeRootNode != nil {
		nodes, err = f.store.nodeTreeDump(ctx, f.store.treeRootNode, nodes)
		if err != nil {
			return nil, err
		}
	}
	var headRoot [32]byte
	if f.store.headNode != nil {
		headRoot = f.store.headNode.root
	}
	resp := &forkchoice2.Dump{
		JustifiedCheckpoint:           jc,
		UnrealizedJustifiedCheckpoint: ujc,
		FinalizedCheckpoint:           fc,
		UnrealizedFinalizedCheckpoint: ufc,
		ProposerBoostRoot:             f.store.proposerBoostRoot[:],
		PreviousProposerBoostRoot:     f.store.previousProposerBoostRoot[:],
		HeadRoot:                      headRoot[:],
		ForkChoiceNodes:               nodes,
	}
	return resp, nil
}

// SetBalancesByRooter sets the balanceByRoot handler in forkchoice
func (f *ForkChoice) SetBalancesByRooter(handler forkchoice.BalancesByRooter) {
	f.balancesByRoot = handler
}

// Weight returns the payload-node weight of the given root if found on the store.
// For Gloas, this is the node weight used for forkchoice on the payload tree.
func (f *ForkChoice) Weight(root [32]byte) (uint64, error) {
	n, ok := f.store.emptyNodeByRoot[root]
	if !ok || n == nil {
		return 0, ErrNilNode
	}
	return n.weight, nil
}

// ConsensusNodeWeight returns the consensus-node weight for the given root if found on the store.
// For Gloas blocks, this includes both empty and full payload node weights.
func (f *ForkChoice) ConsensusNodeWeight(root [32]byte) (uint64, error) {
	n, ok := f.store.emptyNodeByRoot[root]
	if !ok || n == nil {
		return 0, ErrNilNode
	}
	return n.node.weight, nil
}

// PayloadWeights returns the empty and full payload node weights for the given root.
func (f *ForkChoice) PayloadWeights(root [32]byte) (emptyWeight, fullWeight uint64, err error) {
	en, ok := f.store.emptyNodeByRoot[root]
	if !ok || en == nil {
		return 0, 0, ErrNilNode
	}
	emptyWeight = en.weight
	fn := f.store.fullNodeByRoot[root]
	if fn != nil {
		fullWeight = fn.weight
	}
	return emptyWeight, fullWeight, nil
}

// updateJustifiedBalances updates the validators balances on the justified checkpoint pointed by root.
func (f *ForkChoice) updateJustifiedBalances(ctx context.Context, root [32]byte) error {
	balances, err := f.balancesByRoot(ctx, root)
	if err != nil {
		return errors.Wrap(err, "could not get justified balances")
	}
	f.justifiedBalances = balances
	f.store.committeeWeight = 0
	for _, val := range balances {
		if val > 0 {
			f.store.committeeWeight += val
		}
	}
	f.store.committeeWeight /= uint64(params.BeaconConfig().SlotsPerEpoch)
	return nil
}

// Slot returns the slot of the given root if it's known to forkchoice
func (f *ForkChoice) Slot(root [32]byte) (primitives.Slot, error) {
	n, ok := f.store.emptyNodeByRoot[root]
	if !ok || n == nil {
		return 0, ErrNilNode
	}
	return n.node.slot, nil
}

// DependentRoot returns the last root of the epoch prior to the requested ecoch in the canonical chain.
func (f *ForkChoice) DependentRoot(epoch primitives.Epoch) ([32]byte, error) {
	return f.DependentRootForEpoch(f.CachedHeadRoot(), epoch)
}

// DependentRootForEpoch return the last root of the epoch prior to the requested epoch for the given root.
func (f *ForkChoice) DependentRootForEpoch(root [32]byte, epoch primitives.Epoch) ([32]byte, error) {
	tr, err := f.TargetRootForEpoch(root, epoch)
	if err != nil {
		return [32]byte{}, err
	}
	if tr == [32]byte{} {
		return [32]byte{}, nil
	}
	en, ok := f.store.emptyNodeByRoot[tr]
	if !ok || en == nil {
		return [32]byte{}, ErrNilNode
	}
	if slots.ToEpoch(en.node.slot) >= epoch {
		if en.node.parent != nil {
			en = en.node.parent
		} else {
			return f.store.finalizedDependentRoot, nil
		}
	}
	return en.node.root, nil
}

// TargetRootForEpoch returns the root of the target block for a given epoch.
// The epoch parameter is crucial to identify the correct target root. For example:
// When inserting a block at slot 63 with block root 0xA and target root 0xB (pointing to the block at slot 32),
// and at slot 64, where the block is skipped, the attestation will reference the target root as 0xA (for slot 63), not 0xB (for slot 32).
// This implies that if the input slot exceeds the block slot, the target root will be the same as the block root.
// We also allow for the epoch to be below the current target for this root, in
// which case we return the root of the checkpoint of the chain containing the
// passed root, at the given epoch
func (f *ForkChoice) TargetRootForEpoch(root [32]byte, epoch primitives.Epoch) ([32]byte, error) {
	n, ok := f.store.emptyNodeByRoot[root]
	if !ok || n == nil {
		return [32]byte{}, ErrNilNode
	}
	node := n.node
	nodeEpoch := slots.ToEpoch(node.slot)
	if epoch > nodeEpoch {
		return node.root, nil
	}
	if node.target == nil {
		return [32]byte{}, nil
	}
	targetRoot := node.target.root
	if epoch == nodeEpoch {
		return targetRoot, nil
	}
	targetNode, ok := f.store.emptyNodeByRoot[targetRoot]
	if !ok || targetNode == nil {
		return [32]byte{}, ErrNilNode
	}
	// If slot 0 was not missed we consider a previous block to go back at least one epoch
	if nodeEpoch == slots.ToEpoch(targetNode.node.slot) {
		targetNode = targetNode.node.parent
		if targetNode == nil {
			return [32]byte{}, ErrNilNode
		}
	}
	return f.TargetRootForEpoch(targetNode.node.root, epoch)
}

// ParentRoot returns the block root of the parent node if it is in forkchoice.
// The exception is for the finalized checkpoint root which we return the zero
// hash.
func (f *ForkChoice) ParentRoot(root [32]byte) ([32]byte, error) {
	n, ok := f.store.emptyNodeByRoot[root]
	if !ok || n == nil {
		return [32]byte{}, ErrNilNode
	}
	// Return the zero hash for the tree root
	parent := n.node.parent
	if parent == nil {
		return [32]byte{}, nil
	}
	return parent.node.root, nil
}
