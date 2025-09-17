package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	forkchoicetypes "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/features"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

func (s *Service) setupForkchoice(st state.BeaconState) error {
	if err := s.setupForkchoiceCheckpoints(); err != nil {
		return errors.Wrap(err, "could not set up forkchoice checkpoints")
	}
	if err := s.setupForkchoiceTree(st); err != nil {
		return errors.Wrap(err, "could not set up forkchoice tree")
	}
	if err := s.initializeHead(s.ctx, st); err != nil {
		return errors.Wrap(err, "could not initialize head from db")
	}
	return nil
}

func (s *Service) startupHeadRoot() [32]byte {
	headStr := features.Get().ForceHead
	jp := s.CurrentJustifiedCheckpt()
	jRoot := s.ensureRootNotZeros([32]byte(jp.Root))
	if headStr == "" {
		return jRoot
	}
	if headStr == "head" {
		root, err := s.cfg.BeaconDB.HeadBlockRoot()
		if err != nil {
			log.WithError(err).Error("Could not get head block root, starting with justified block as head")
			return jRoot
		}
		log.Infof("Using Head root of %#x", root)
		return root
	}
	root, err := bytesutil.DecodeHexWithLength(headStr, 32)
	if err != nil {
		log.WithError(err).Error("Could not parse head root, starting with justified block as head")
		return jRoot
	}
	return [32]byte(root)
}

func (s *Service) setupForkchoiceTree(st state.BeaconState) error {
	headRoot := s.startupHeadRoot()
	cp := s.FinalizedCheckpt()
	fRoot := s.ensureRootNotZeros([32]byte(cp.Root))
	if err := s.setupForkchoiceRoot(st); err != nil {
		return errors.Wrap(err, "could not set up forkchoice root")
	}
	if headRoot == fRoot {
		return nil
	}
	blk, err := s.cfg.BeaconDB.Block(s.ctx, headRoot)
	if err != nil {
		log.WithError(err).Error("Could not get head block, starting with finalized block as head")
		return nil
	}
	if slots.ToEpoch(blk.Block().Slot()) < cp.Epoch {
		log.WithField("headRoot", fmt.Sprintf("%#x", headRoot)).Error("Head block is older than finalized block, starting with finalized block as head")
		return nil
	}
	chain, err := s.buildForkchoiceChain(s.ctx, blk)
	if err != nil {
		log.WithError(err).Error("Could not build forkchoice chain, starting with finalized block as head")
		return nil
	}
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return s.cfg.ForkChoiceStore.InsertChain(s.ctx, chain)
}

func (s *Service) buildForkchoiceChain(ctx context.Context, head interfaces.ReadOnlySignedBeaconBlock) ([]*forkchoicetypes.BlockAndCheckpoints, error) {
	chain := []*forkchoicetypes.BlockAndCheckpoints{}
	cp := s.FinalizedCheckpt()
	fRoot := s.ensureRootNotZeros([32]byte(cp.Root))
	jp := s.CurrentJustifiedCheckpt()
	root, err := head.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get head block root")
	}
	for {
		roblock, err := blocks.NewROBlockWithRoot(head, root)
		if err != nil {
			return nil, err
		}
		// This chain sets the justified checkpoint for every block, including some that are older than jp.
		// This should be however safe for forkchoice at startup. An alternative would be to hook during the
		// block processing pipeline when setting the head state, to compute the right states for the justified
		// checkpoint.
		chain = append(chain, &forkchoicetypes.BlockAndCheckpoints{Block: roblock, JustifiedCheckpoint: jp, FinalizedCheckpoint: cp})
		root = head.Block().ParentRoot()
		if root == fRoot {
			break
		}
		head, err = s.cfg.BeaconDB.Block(s.ctx, root)
		if err != nil {
			return nil, errors.Wrap(err, "could not get block")
		}
		if slots.ToEpoch(head.Block().Slot()) < cp.Epoch {
			return nil, errors.New("head block is not a descendant of the finalized checkpoint")
		}
	}
	slices.Reverse(chain)
	return chain, nil
}

func (s *Service) setupForkchoiceRoot(st state.BeaconState) error {
	cp := s.FinalizedCheckpt()
	fRoot := s.ensureRootNotZeros([32]byte(cp.Root))
	finalizedBlock, err := s.cfg.BeaconDB.Block(s.ctx, fRoot)
	if err != nil {
		return errors.Wrap(err, "could not get finalized checkpoint block")
	}
	roblock, err := blocks.NewROBlockWithRoot(finalizedBlock, fRoot)
	if err != nil {
		return err
	}
	if err := s.cfg.ForkChoiceStore.InsertNode(s.ctx, st, roblock); err != nil {
		return errors.Wrap(err, "could not insert finalized block to forkchoice")
	}
	if !features.Get().EnableStartOptimistic {
		lastValidatedCheckpoint, err := s.cfg.BeaconDB.LastValidatedCheckpoint(s.ctx)
		if err != nil {
			return errors.Wrap(err, "could not get last validated checkpoint")
		}
		if bytes.Equal(fRoot[:], lastValidatedCheckpoint.Root) {
			if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(s.ctx, fRoot); err != nil {
				return errors.Wrap(err, "could not set finalized block as validated")
			}
		}
	}
	return nil
}

func (s *Service) setupForkchoiceCheckpoints() error {
	justified, err := s.cfg.BeaconDB.JustifiedCheckpoint(s.ctx)
	if err != nil {
		return errors.Wrap(err, "could not get justified checkpoint")
	}
	if justified == nil {
		return errNilJustifiedCheckpoint
	}
	finalized, err := s.cfg.BeaconDB.FinalizedCheckpoint(s.ctx)
	if err != nil {
		return errors.Wrap(err, "could not get finalized checkpoint")
	}
	if finalized == nil {
		return errNilFinalizedCheckpoint
	}

	fRoot := s.ensureRootNotZeros(bytesutil.ToBytes32(finalized.Root))
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	if err := s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(s.ctx, &forkchoicetypes.Checkpoint{Epoch: justified.Epoch,
		Root: bytesutil.ToBytes32(justified.Root)}); err != nil {
		return errors.Wrap(err, "could not update forkchoice's justified checkpoint")
	}
	if err := s.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: finalized.Epoch,
		Root: fRoot}); err != nil {
		return errors.Wrap(err, "could not update forkchoice's finalized checkpoint")
	}
	s.cfg.ForkChoiceStore.SetGenesisTime(s.genesisTime)
	return nil
}
