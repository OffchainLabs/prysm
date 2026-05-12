package doublylinkedtree

import (
	"context"
	"fmt"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	consensus_blocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// head starts from justified root and then follows the best descendant links
// to find the best block for head.
func (s *Store) head(ctx context.Context) ([32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.head")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return [32]byte{}, err
	}

	// JustifiedRoot has to be known
	var jn *Node
	ej := s.emptyNodeByRoot[s.justifiedCheckpoint.Root]
	if ej != nil {
		jn = ej.node
	} else {
		// If the justifiedCheckpoint is from genesis, then the root is
		// zeroHash. In this case it should be the root of forkchoice
		// tree.
		if s.justifiedCheckpoint.Epoch == params.BeaconConfig().GenesisEpoch {
			jn = s.treeRootNode
		} else {
			return [32]byte{}, errors.WithMessage(errUnknownJustifiedRoot, fmt.Sprintf("%#x", s.justifiedCheckpoint.Root))
		}
	}

	// If the justified node doesn't have a best descendant,
	// the best node is itself.
	bestDescendant := jn.bestDescendant
	if bestDescendant == nil {
		bestDescendant = jn
	}
	currentEpoch := slots.EpochsSinceGenesis(s.genesisTime)
	if !bestDescendant.viableForHead(s.justifiedCheckpoint.Epoch, currentEpoch) {
		s.allTipsAreInvalid = true
		return [32]byte{}, fmt.Errorf("head at slot %d with weight %d is not eligible, finalizedEpoch, justified Epoch %d, %d != %d, %d",
			bestDescendant.slot, bestDescendant.weight/10e9, bestDescendant.finalizedEpoch, bestDescendant.justifiedEpoch, s.finalizedCheckpoint.Epoch, s.justifiedCheckpoint.Epoch)
	}
	s.allTipsAreInvalid = false

	// Update metrics.
	if bestDescendant != s.headNode {
		headChangesCount.Inc()
		headSlotNumber.Set(float64(bestDescendant.slot))
		s.headNode = bestDescendant
	}

	return bestDescendant.root, nil
}

// insert registers a new block node to the fork choice store's node list.
// It then updates the new node's parent with the best child and descendant node.
func (s *Store) insert(ctx context.Context,
	roblock consensus_blocks.ROBlock,
	justifiedEpoch, finalizedEpoch primitives.Epoch,
) (*PayloadNode, error) {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.insert")
	defer span.End()

	root := roblock.Root()
	// Return if the block has been inserted into Store before.
	if n, ok := s.emptyNodeByRoot[root]; ok {
		return n, nil
	}

	block := roblock.Block()
	slot := block.Slot()
	var parent *PayloadNode
	blockHash := &[32]byte{}
	if block.Version() >= version.Gloas {
		if err := s.resolveParentPayloadStatus(block, &parent, blockHash); err != nil {
			return nil, err
		}
	} else {
		if block.Version() >= version.Bellatrix {
			execution, err := block.Body().Execution()
			if err != nil {
				return nil, err
			}
			copy(blockHash[:], execution.BlockHash())
		}
		parentRoot := block.ParentRoot()
		en := s.emptyNodeByRoot[parentRoot]
		parent = s.fullNodeByRoot[parentRoot]
		if parent == nil && en != nil {
			// pre-Gloas only full parents are allowed.
			return nil, errInvalidParentRoot
		}
	}

	n := &Node{
		slot:                        slot,
		root:                        root,
		parent:                      parent,
		justifiedEpoch:              justifiedEpoch,
		unrealizedJustifiedEpoch:    justifiedEpoch,
		finalizedEpoch:              finalizedEpoch,
		unrealizedFinalizedEpoch:    finalizedEpoch,
		blockHash:                   *blockHash,
		payloadAvailabilityVote:     bitfield.NewBitvector512(),
		payloadDataAvailabilityVote: bitfield.NewBitvector512(),
	}
	// Set the node's target checkpoint
	if slot%params.BeaconConfig().SlotsPerEpoch == 0 {
		n.target = n
	} else if parent != nil {
		if slots.ToEpoch(slot) == slots.ToEpoch(parent.node.slot) {
			n.target = parent.node.target
		} else {
			n.target = parent.node
		}
	}
	var ret *PayloadNode
	optimistic := true
	if parent != nil {
		optimistic = n.parent.optimistic
	}
	// Make the empty node.It's optimistic status equals it's parent's status.
	pn := &PayloadNode{
		node:       n,
		optimistic: optimistic,
		timestamp:  time.Now(),
		children:   make([]*Node, 0),
	}
	s.emptyNodeByRoot[root] = pn
	ret = pn
	if block.Version() < version.Gloas {
		// Make also the full node, this is optimistic until the engine returns the execution payload validation.
		fn := &PayloadNode{
			node:       n,
			optimistic: true,
			timestamp:  time.Now(),
			full:       true,
		}
		ret = fn
		s.fullNodeByRoot[root] = fn
	}

	if parent == nil {
		if s.treeRootNode == nil {
			s.treeRootNode = n
			s.headNode = n
			s.highestReceivedNode = n
		} else {
			delete(s.emptyNodeByRoot, root)
			delete(s.fullNodeByRoot, root)
			updatePayloadNodeMetrics(s)
			return nil, errInvalidParentRoot
		}
	} else {
		parent.children = append(parent.children, n)
		// Apply proposer boost
		now := time.Now()
		if now.Before(s.genesisTime) {
			return ret, nil
		}
		currentSlot := slots.CurrentSlot(s.genesisTime)
		sss, err := slots.SinceSlotStart(currentSlot, s.genesisTime, now)
		if err != nil {
			return nil, fmt.Errorf("could not determine time since current slot started: %w", err)
		}
		bps := params.BeaconConfig().AttestationDueBPS
		if block.Version() >= version.Gloas {
			bps = params.BeaconConfig().AttestationDueBPSGloas
		}
		boostThreshold := params.BeaconConfig().SlotComponentDuration(bps)
		isFirstBlock := s.proposerBoostRoot == [32]byte{}
		if currentSlot == slot && sss < boostThreshold && isFirstBlock {
			s.proposerBoostRoot = root
		}

		// Update best descendants
		jEpoch := s.justifiedCheckpoint.Epoch
		fEpoch := s.finalizedCheckpoint.Epoch
		if err := s.updateBestDescendantConsensusNode(ctx, s.treeRootNode, jEpoch, fEpoch, slots.ToEpoch(currentSlot)); err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"slot": slot,
				"root": root,
			}).Error("Could not update best descendant")
		}
	}
	// Update metrics.
	processedBlockCount.Inc()
	nodeCount.Set(float64(len(s.emptyNodeByRoot)))
	updatePayloadNodeMetrics(s)

	// Only update received block slot if it's within epoch from current time.
	if slot+params.BeaconConfig().SlotsPerEpoch > slots.CurrentSlot(s.genesisTime) {
		s.receivedBlocksLastEpoch[slot%params.BeaconConfig().SlotsPerEpoch] = slot
	}
	// Update highest slot tracking.
	if slot > s.highestReceivedNode.slot {
		s.highestReceivedNode = n
	}

	return ret, nil
}

// pruneFinalizedNodeByRootMap prunes the `nodeByRoot` maps
// starting from `node` down to the finalized Node or to a leaf of the Fork
// choice store.
func (s *Store) pruneFinalizedNodeByRootMap(ctx context.Context, node, finalizedNode *Node) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if node == finalizedNode {
		if node.target != node {
			node.target = nil
		}
		return nil
	}
	for _, child := range s.allConsensusChildren(node) {
		if err := s.pruneFinalizedNodeByRootMap(ctx, child, finalizedNode); err != nil {
			return err
		}
	}
	en := s.emptyNodeByRoot[node.root]
	en.children = nil
	delete(s.emptyNodeByRoot, node.root)
	fn := s.fullNodeByRoot[node.root]
	if fn != nil {
		fn.children = nil
		delete(s.fullNodeByRoot, node.root)
	}
	updatePayloadNodeMetrics(s)
	return nil
}

// prune prunes the fork choice store. It removes all nodes that compete with the finalized root.
// This function does not prune for invalid optimistically synced nodes, it deals only with pruning upon finalization
// TODO: Gloas, to ensure that chains up to a full node are found, we may want to consider pruning only up to the latest full block that was finalized
func (s *Store) prune(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "doublyLinkedForkchoice.Prune")
	defer span.End()

	finalizedRoot := s.finalizedCheckpoint.Root
	finalizedEpoch := s.finalizedCheckpoint.Epoch
	fen, ok := s.emptyNodeByRoot[finalizedRoot]
	if !ok || fen == nil {
		return errors.WithMessage(errUnknownFinalizedRoot, fmt.Sprintf("%#x", finalizedRoot))
	}
	fn := fen.node
	// return early if we haven't changed the finalized checkpoint
	if fn.parent == nil {
		return nil
	}

	// Save the new finalized dependent root because it will be pruned
	s.finalizedDependentRoot = fn.parent.node.root

	// Prune nodeByRoot starting from root
	if err := s.pruneFinalizedNodeByRootMap(ctx, s.treeRootNode, fn); err != nil {
		return err
	}

	fn.parent = nil
	s.treeRootNode = fn

	prunedCount.Inc()
	// Prune all children of the finalized checkpoint block that are incompatible with it
	checkpointMaxSlot, err := slots.EpochStart(finalizedEpoch)
	if err != nil {
		return errors.Wrap(err, "could not compute epoch start")
	}
	if fn.slot == checkpointMaxSlot {
		return nil
	}

	for _, child := range fen.children {
		if child != nil && child.slot <= checkpointMaxSlot {
			if err := s.pruneFinalizedNodeByRootMap(ctx, child, fn); err != nil {
				return errors.Wrap(err, "could not prune incompatible finalized child")
			}
		}
	}
	return nil
}

// tips returns a list of possible heads from fork choice store, it returns the
// roots and the slots of the leaf nodes.
func (s *Store) tips() ([][32]byte, []primitives.Slot) {
	var roots [][32]byte
	var slots []primitives.Slot

	for root, n := range s.emptyNodeByRoot {
		if len(s.allConsensusChildren(n.node)) == 0 {
			roots = append(roots, root)
			slots = append(slots, n.node.slot)
		}
	}
	return roots, slots
}

func (f *ForkChoice) HighestReceivedBlockRoot() [32]byte {
	if f.store.highestReceivedNode == nil {
		return [32]byte{}
	}
	return f.store.highestReceivedNode.root
}

// HighestReceivedBlockSlot returns the highest slot received by the forkchoice
func (f *ForkChoice) HighestReceivedBlockSlot() primitives.Slot {
	if f.store.highestReceivedNode == nil {
		return 0
	}
	return f.store.highestReceivedNode.slot
}

// ReceivedBlocksLastEpoch returns the number of blocks received in the last epoch
func (f *ForkChoice) ReceivedBlocksLastEpoch() (uint64, error) {
	count := uint64(0)
	lowerBound := slots.CurrentSlot(f.store.genesisTime)
	var err error
	if lowerBound > fieldparams.SlotsPerEpoch {
		lowerBound, err = lowerBound.SafeSub(fieldparams.SlotsPerEpoch)
		if err != nil {
			return 0, err
		}
	}

	for _, s := range f.store.receivedBlocksLastEpoch {
		if s != 0 && lowerBound <= s {
			count++
		}
	}
	return count, nil
}
