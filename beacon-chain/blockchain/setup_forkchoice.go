package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
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
	if blk == nil || blk.IsNil() {
		log.Error("Head block is nil, starting with finalized block as head")
		return nil
	}
	blkBlock := blk.Block()
	if blkBlock == nil || blkBlock.IsNil() {
		log.Error("Head beacon block is nil, starting with finalized block as head")
		return nil
	}
	if slots.ToEpoch(blkBlock.Slot()) < cp.Epoch {
		log.WithField("headRoot", fmt.Sprintf("%#x", headRoot)).Error("Head block is older than finalized block, starting with finalized block as head")
		return nil
	}
	chain, err := s.buildForkchoiceChain(s.ctx, blk)
	if err != nil {
		log.WithError(err).Error("Could not build forkchoice chain, starting with finalized block as head")
		return nil
	}
	resolveChainPayloadStatus(chain)
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	if err := s.markFinalizedRootFull(chain, fRoot); err != nil {
		log.WithError(err).Error("Could not mark finalized root as full in forkchoice")
	}
	return s.cfg.ForkChoiceStore.InsertChain(s.ctx, chain)
}

func (s *Service) buildForkchoiceChain(ctx context.Context, head interfaces.ReadOnlySignedBeaconBlock) ([]*forkchoicetypes.BlockAndCheckpoints, error) {
	chain := []*forkchoicetypes.BlockAndCheckpoints{}
	cp := s.FinalizedCheckpt()
	fRoot := s.ensureRootNotZeros([32]byte(cp.Root))
	jp := s.CurrentJustifiedCheckpt()
	if head == nil || head.IsNil() {
		return nil, errors.New("head block is nil")
	}
	headBlock := head.Block()
	if headBlock == nil || headBlock.IsNil() {
		return nil, errors.New("head beacon block is nil")
	}
	root, err := headBlock.HashTreeRoot()
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
		root = headBlock.ParentRoot()
		if root == fRoot {
			break
		}
		head, err = s.cfg.BeaconDB.Block(s.ctx, root)
		if err != nil {
			return nil, errors.Wrap(err, "could not get block")
		}
		if head == nil || head.IsNil() {
			return nil, errors.New("head block is nil")
		}
		headBlock = head.Block()
		if headBlock == nil || headBlock.IsNil() {
			return nil, errors.New("head beacon block is nil")
		}
		if slots.ToEpoch(headBlock.Slot()) < cp.Epoch {
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

// resolveChainPayloadStatus determines which blocks in the chain had their
// execution payloads delivered by checking if consecutive blocks' bids indicate
// payload delivery. For each pair of blocks (chain[i], chain[i+1]), if the next
// block's bid parentBlockHash equals the current block's bid blockHash, the
// current block's payload was delivered.
func resolveChainPayloadStatus(chain []*forkchoicetypes.BlockAndCheckpoints) {
	for i := 0; i < len(chain)-1; i++ {
		curr := chain[i].Block.Block()
		next := chain[i+1].Block.Block()
		if curr.Version() < version.Gloas || next.Version() < version.Gloas {
			continue
		}
		currBid, err := curr.Body().SignedExecutionPayloadBid()
		if err != nil || currBid == nil || currBid.Message == nil {
			continue
		}
		nextBid, err := next.Body().SignedExecutionPayloadBid()
		if err != nil || nextBid == nil || nextBid.Message == nil {
			continue
		}
		if bytes.Equal(nextBid.Message.ParentBlockHash, currBid.Message.BlockHash) {
			chain[i].HasPayload = true
		}
	}
}

// markFinalizedRootFull checks whether the finalized root block's execution
// payload was delivered by inspecting the first block in the chain. If the first
// block's bid parentBlockHash equals the finalized block's bid blockHash, the
// finalized block's payload was delivered and a full node must be created in
// forkchoice. The caller must hold the forkchoice lock.
func (s *Service) markFinalizedRootFull(chain []*forkchoicetypes.BlockAndCheckpoints, fRoot [32]byte) error {
	if len(chain) == 0 {
		return nil
	}
	firstBlock := chain[0].Block.Block()
	if firstBlock.Version() < version.Gloas {
		return nil
	}
	firstBid, err := firstBlock.Body().SignedExecutionPayloadBid()
	if err != nil || firstBid == nil || firstBid.Message == nil {
		return nil
	}
	fBlock, err := s.cfg.BeaconDB.Block(s.ctx, fRoot)
	if err != nil {
		return errors.Wrap(err, "could not get finalized block")
	}
	if fBlock.Block().Version() < version.Gloas {
		return nil
	}
	fBid, err := fBlock.Block().Body().SignedExecutionPayloadBid()
	if err != nil || fBid == nil || fBid.Message == nil {
		return nil
	}
	if !bytes.Equal(firstBid.Message.ParentBlockHash, fBid.Message.BlockHash) {
		return nil
	}
	// The finalized block's payload was delivered. Create the full node.
	s.cfg.ForkChoiceStore.MarkFullNode(fRoot)
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
		log.WithError(err).Error("Could not update forkchoice's justified checkpoint, trying to update finalized checkpoint anyway")
	}
	if err := s.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: finalized.Epoch,
		Root: fRoot}); err != nil {
		log.WithError(err).Error("Could not update forkchoice's finalized checkpoint")
	}
	s.cfg.ForkChoiceStore.SetGenesisTime(s.genesisTime)
	return nil
}
