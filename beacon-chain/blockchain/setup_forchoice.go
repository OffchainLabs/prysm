package blockchain

import (
	"bytes"

	"github.com/pkg/errors"
	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
)

func (s *Service) setupForkchoice(st state.BeaconState) error {
	if err := s.setupForkchoiceCheckpoints(); err != nil {
		return errors.Wrap(err, "could not set up forkchoice checkpoints")
	}
	if err := s.setupForkchoiceRoot(st); err != nil {
		return errors.Wrap(err, "could not set up forkchoice root")
	}
	if err := s.initializeHeadFromDB(s.ctx, st); err != nil {
		return errors.Wrap(err, "could not initialize head from db")
	}
	return nil
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
	s.cfg.ForkChoiceStore.SetGenesisTime(uint64(s.genesisTime.Unix()))
	return nil
}
