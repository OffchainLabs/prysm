package doublylinkedtree

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/epoch/precompute"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

func (s *Store) setUnrealizedJustifiedCheckpoint(blockRoot [32]byte, epoch primitives.Epoch, cpRoot [32]byte) error {
	en, ok := s.emptyNodeByRoot[blockRoot]
	if !ok || en == nil {
		return errors.Wrap(ErrNilNode, "could not set unrealized justified checkpoint")
	}
	if epoch < en.node.unrealizedJustified.Epoch {
		return errInvalidUnrealizedJustifiedEpoch
	}
	en.node.unrealizedJustified.Epoch = epoch
	en.node.unrealizedJustified.Root = cpRoot
	return nil
}

func (s *Store) setUnrealizedFinalizedEpoch(root [32]byte, epoch primitives.Epoch) error {
	en, ok := s.emptyNodeByRoot[root]
	if !ok || en == nil {
		return errors.Wrap(ErrNilNode, "could not set unrealized finalized epoch")
	}
	if epoch < en.node.unrealizedFinalizedEpoch {
		return errInvalidUnrealizedFinalizedEpoch
	}
	en.node.unrealizedFinalizedEpoch = epoch
	return nil
}

// updateUnrealizedCheckpoints "realizes" the unrealized justified and finalized
// epochs stored within nodes. It should be called at the beginning of each epoch.
func (f *ForkChoice) updateUnrealizedCheckpoints(ctx context.Context) error {
	for _, en := range f.store.emptyNodeByRoot {
		node := en.node
		node.justifiedEpoch = node.unrealizedJustified.Epoch
		node.finalizedEpoch = node.unrealizedFinalizedEpoch
		if node.justifiedEpoch > f.store.justifiedCheckpoint.Epoch {
			f.store.prevJustifiedCheckpoint = f.store.justifiedCheckpoint
			f.store.justifiedCheckpoint = f.store.unrealizedJustifiedCheckpoint
			if err := f.updateJustifiedBalances(ctx, f.store.justifiedCheckpoint.Root); err != nil {
				return errors.Wrap(err, "could not update justified balances")
			}
		}
		if node.finalizedEpoch > f.store.finalizedCheckpoint.Epoch {
			f.store.finalizedCheckpoint = f.store.unrealizedFinalizedCheckpoint
		}
	}
	return nil
}

func (s *Store) pullTips(state state.BeaconState, node *Node, jc, fc *ethpb.Checkpoint) (*ethpb.Checkpoint, *ethpb.Checkpoint) {
	if node.parent == nil {
		return jc, fc
	}
	pn := node.parent.node
	currentEpoch := slots.ToEpoch(slots.CurrentSlot(s.genesisTime))
	stateSlot := state.Slot()
	stateEpoch := slots.ToEpoch(stateSlot)
	currJustified := pn.unrealizedJustified.Epoch == currentEpoch
	prevJustified := pn.unrealizedJustified.Epoch+1 == currentEpoch
	tooEarlyForCurr := slots.SinceEpochStarts(stateSlot)*3 < params.BeaconConfig().SlotsPerEpoch*2
	// Exit early if it's justified or too early to be justified.
	if currJustified || (stateEpoch == currentEpoch && prevJustified && tooEarlyForCurr) {
		node.unrealizedJustified.Epoch = pn.unrealizedJustified.Epoch
		node.unrealizedJustified.Root = pn.unrealizedJustified.Root
		node.unrealizedFinalizedEpoch = pn.unrealizedFinalizedEpoch
		return jc, fc
	}

	uj, uf, err := precompute.UnrealizedCheckpoints(state)
	if err != nil {
		log.WithError(err).Debug("could not compute unrealized checkpoints")
		uj, uf = jc, fc
	}

	// Update store's unrealized checkpoints.
	if uj.Epoch > s.unrealizedJustifiedCheckpoint.Epoch {
		s.unrealizedJustifiedCheckpoint = &forkchoicetypes.Checkpoint{
			Epoch: uj.Epoch, Root: bytesutil.ToBytes32(uj.Root),
		}
	}
	if uf.Epoch > s.unrealizedFinalizedCheckpoint.Epoch {
		s.unrealizedFinalizedCheckpoint = &forkchoicetypes.Checkpoint{
			Epoch: uf.Epoch, Root: bytesutil.ToBytes32(uf.Root),
		}
	}

	// Update node's checkpoints.
	node.unrealizedJustified.Epoch = uj.Epoch
	node.unrealizedJustified.Root = bytesutil.ToBytes32(uj.Root)
	node.unrealizedFinalizedEpoch = uf.Epoch
	if stateEpoch < currentEpoch {
		jc, fc = uj, uf
		node.justifiedEpoch = uj.Epoch
		node.finalizedEpoch = uf.Epoch
	}
	return jc, fc
}
