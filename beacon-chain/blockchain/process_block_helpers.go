package blockchain

import (
	"context"
	"fmt"
	"time"

	lightclient "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/light-client"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	statefeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	doublylinkedtree "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/prysmaticlabs/prysm/v5/beacon-chain/forkchoice/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	field_params "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	consensus_blocks "github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	mathutil "github.com/prysmaticlabs/prysm/v5/math"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpbv2 "github.com/prysmaticlabs/prysm/v5/proto/eth/v2"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

// CurrentSlot returns the current slot based on time.
func (s *Service) CurrentSlot() primitives.Slot {
	return slots.CurrentSlot(uint64(s.genesisTime.Unix()))
}

// getFCUArgs returns the arguments to call forkchoice update
func (s *Service) getFCUArgs(cfg *postBlockProcessConfig, fcuArgs *fcuConfig) error {
	if err := s.getFCUArgsEarlyBlock(cfg, fcuArgs); err != nil {
		return err
	}
	if !s.inRegularSync() {
		return nil
	}
	slot := cfg.roblock.Block().Slot()
	if slots.WithinVotingWindow(uint64(s.genesisTime.Unix()), slot) {
		return nil
	}
	return s.computePayloadAttributes(cfg, fcuArgs)
}

func (s *Service) getFCUArgsEarlyBlock(cfg *postBlockProcessConfig, fcuArgs *fcuConfig) error {
	if cfg.roblock.Root() == cfg.headRoot {
		fcuArgs.headState = cfg.postState
		fcuArgs.headBlock = cfg.roblock
		fcuArgs.headRoot = cfg.headRoot
		fcuArgs.proposingSlot = s.CurrentSlot() + 1
		return nil
	}
	return s.fcuArgsNonCanonicalBlock(cfg, fcuArgs)
}

// logNonCanonicalBlockReceived prints a message informing that the received
// block is not the head of the chain. It requires the caller holds a lock on
// Forkchoice.
func (s *Service) logNonCanonicalBlockReceived(blockRoot [32]byte, headRoot [32]byte) {
	receivedWeight, err := s.cfg.ForkChoiceStore.Weight(blockRoot)
	if err != nil {
		log.WithField("root", fmt.Sprintf("%#x", blockRoot)).Warn("could not determine node weight")
	}
	headWeight, err := s.cfg.ForkChoiceStore.Weight(headRoot)
	if err != nil {
		log.WithField("root", fmt.Sprintf("%#x", headRoot)).Warn("could not determine node weight")
	}
	log.WithFields(logrus.Fields{
		"receivedRoot":   fmt.Sprintf("%#x", blockRoot),
		"receivedWeight": receivedWeight,
		"headRoot":       fmt.Sprintf("%#x", headRoot),
		"headWeight":     headWeight,
	}).Debug("Head block is not the received block")
}

// fcuArgsNonCanonicalBlock returns the arguments to the FCU call when the
// incoming block is non-canonical, that is, based on the head root.
func (s *Service) fcuArgsNonCanonicalBlock(cfg *postBlockProcessConfig, fcuArgs *fcuConfig) error {
	headState, headBlock, err := s.getStateAndBlock(cfg.ctx, cfg.headRoot)
	if err != nil {
		return err
	}
	fcuArgs.headState = headState
	fcuArgs.headBlock = headBlock
	fcuArgs.headRoot = cfg.headRoot
	fcuArgs.proposingSlot = s.CurrentSlot() + 1
	return nil
}

// sendStateFeedOnBlock sends an event that a new block has been synced
func (s *Service) sendStateFeedOnBlock(cfg *postBlockProcessConfig) {
	optimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(cfg.roblock.Root())
	if err != nil {
		log.WithError(err).Debug("Could not check if block is optimistic")
		optimistic = true
	}
	// Send notification of the processed block to the state feed.
	s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.BlockProcessed,
		Data: &statefeed.BlockProcessedData{
			Slot:        cfg.roblock.Block().Slot(),
			BlockRoot:   cfg.roblock.Root(),
			SignedBlock: cfg.roblock,
			Verified:    true,
			Optimistic:  optimistic,
		},
	})
}

// sendLightClientFeeds sends the light client feeds when feature flag is enabled.
func (s *Service) sendLightClientFeeds(cfg *postBlockProcessConfig) {
	if features.Get().EnableLightClient {
		if _, err := s.sendLightClientOptimisticUpdate(cfg.ctx, cfg.roblock, cfg.postState); err != nil {
			log.WithError(err).Error("Failed to send light client optimistic update")
		}

		// Get the finalized checkpoint
		finalized := s.ForkChoicer().FinalizedCheckpoint()

		// LightClientFinalityUpdate needs super majority
		s.tryPublishLightClientFinalityUpdate(cfg.ctx, cfg.roblock, finalized, cfg.postState)
	}
}

func (s *Service) tryPublishLightClientFinalityUpdate(
	ctx context.Context,
	signed interfaces.ReadOnlySignedBeaconBlock,
	finalized *forkchoicetypes.Checkpoint,
	postState state.BeaconState,
) {
	if finalized.Epoch <= s.lastPublishedLightClientEpoch {
		return
	}

	config := params.BeaconConfig()
	if finalized.Epoch < config.AltairForkEpoch {
		return
	}

	syncAggregate, err := signed.Block().Body().SyncAggregate()
	if err != nil || syncAggregate == nil {
		return
	}

	// LightClientFinalityUpdate needs super majority
	if syncAggregate.SyncCommitteeBits.Count()*3 < config.SyncCommitteeSize*2 {
		return
	}

	_, err = s.sendLightClientFinalityUpdate(ctx, signed, postState)
	if err != nil {
		log.WithError(err).Error("Failed to send light client finality update")
	} else {
		s.lastPublishedLightClientEpoch = finalized.Epoch
	}
}

// sendLightClientFinalityUpdate sends a light client finality update notification to the state feed.
func (s *Service) sendLightClientFinalityUpdate(ctx context.Context, signed interfaces.ReadOnlySignedBeaconBlock,
	postState state.BeaconState) (int, error) {
	// Get attested state
	attestedRoot := signed.Block().ParentRoot()
	attestedBlock, err := s.cfg.BeaconDB.Block(ctx, attestedRoot)
	if err != nil {
		return 0, errors.Wrap(err, "could not get attested block")
	}
	attestedState, err := s.cfg.StateGen.StateByRoot(ctx, attestedRoot)
	if err != nil {
		return 0, errors.Wrap(err, "could not get attested state")
	}

	// Get finalized block
	var finalizedBlock interfaces.ReadOnlySignedBeaconBlock
	finalizedCheckPoint := attestedState.FinalizedCheckpoint()
	if finalizedCheckPoint != nil {
		finalizedRoot := bytesutil.ToBytes32(finalizedCheckPoint.Root)
		finalizedBlock, err = s.cfg.BeaconDB.Block(ctx, finalizedRoot)
		if err != nil {
			finalizedBlock = nil
		}
	}

	_, err = lightclient.NewLightClientFinalityUpdateFromBeaconState(
		ctx,
		postState.Slot(),
		postState,
		signed,
		attestedState,
		attestedBlock,
		finalizedBlock,
	)
	if err != nil {
		return 0, errors.Wrap(err, "could not create light client update")
	}

	// Return the result
	result := &ethpbv2.LightClientFinalityUpdateWithVersion{
		Version: ethpbv2.Version(signed.Version()),
		// TODO: Get back to this when revisiting events
		//Data:    update,
	}

	// Send event
	return s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.LightClientFinalityUpdate,
		Data: result,
	}), nil
}

// sendLightClientOptimisticUpdate sends a light client optimistic update notification to the state feed.
func (s *Service) sendLightClientOptimisticUpdate(ctx context.Context, signed interfaces.ReadOnlySignedBeaconBlock,
	postState state.BeaconState) (int, error) {
	// Get attested state
	attestedRoot := signed.Block().ParentRoot()
	attestedBlock, err := s.cfg.BeaconDB.Block(ctx, attestedRoot)
	if err != nil {
		return 0, errors.Wrap(err, "could not get attested block")
	}
	attestedState, err := s.cfg.StateGen.StateByRoot(ctx, attestedRoot)
	if err != nil {
		return 0, errors.Wrap(err, "could not get attested state")
	}

	_, err = lightclient.NewLightClientOptimisticUpdateFromBeaconState(
		ctx,
		postState.Slot(),
		postState,
		signed,
		attestedState,
		attestedBlock,
	)
	if err != nil {
		return 0, errors.Wrap(err, "could not create light client update")
	}

	// Return the result
	result := &ethpbv2.LightClientOptimisticUpdateWithVersion{
		Version: ethpbv2.Version(signed.Version()),
		// TODO: Get back to this when revisiting events
		//Data:    update,
	}

	return s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.LightClientOptimisticUpdate,
		Data: result,
	}), nil
}

// updateCachesPostBlockProcessing updates the next slot cache and handles the epoch
// boundary in order to compute the right proposer indices after processing
// state transition. This function is called on late blocks while still locked,
// before sending FCU to the engine.
func (s *Service) updateCachesPostBlockProcessing(cfg *postBlockProcessConfig) error {
	slot := cfg.postState.Slot()
	root := cfg.roblock.Root()
	if err := transition.UpdateNextSlotCache(cfg.ctx, root[:], cfg.postState); err != nil {
		return errors.Wrap(err, "could not update next slot state cache")
	}
	if !slots.IsEpochEnd(slot) {
		return nil
	}
	return s.handleEpochBoundary(cfg.ctx, slot, cfg.postState, root[:])
}

// handleSecondFCUCall handles a second call to FCU when syncing a new block.
// This is useful when proposing in the next block and we want to defer the
// computation of the next slot shuffling.
func (s *Service) handleSecondFCUCall(cfg *postBlockProcessConfig, fcuArgs *fcuConfig) {
	if (fcuArgs.attributes == nil || fcuArgs.attributes.IsEmpty()) && cfg.headRoot == cfg.roblock.Root() {
		go s.sendFCUWithAttributes(cfg, fcuArgs)
	}
}

// reportProcessingTime reports the metric of how long it took to process the
// current block
func reportProcessingTime(startTime time.Time) {
	onBlockProcessingTime.Observe(float64(time.Since(startTime).Milliseconds()))
}

// computePayloadAttributes modifies the passed FCU arguments to
// contain the right payload attributes with the tracked proposer. It gets
// called on blocks that arrive after the attestation voting window, or in a
// background routine after syncing early blocks.
func (s *Service) computePayloadAttributes(cfg *postBlockProcessConfig, fcuArgs *fcuConfig) error {
	if cfg.roblock.Root() == cfg.headRoot {
		if err := s.updateCachesPostBlockProcessing(cfg); err != nil {
			return err
		}
	}
	fcuArgs.attributes = s.getPayloadAttribute(cfg.ctx, fcuArgs.headState, fcuArgs.proposingSlot, cfg.headRoot[:])
	return nil
}

// getBlockPreState returns the pre state of an incoming block. It uses the parent root of the block
// to retrieve the state in DB. It verifies the pre state's validity and the incoming block
// is in the correct time window.
func (s *Service) getBlockPreState(ctx context.Context, b interfaces.ReadOnlyBeaconBlock) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.getBlockPreState")
	defer span.End()

	// Verify incoming block has a valid pre state.
	if err := s.verifyBlkPreState(ctx, b.ParentRoot()); err != nil {
		return nil, err
	}

	preState, err := s.cfg.StateGen.StateByRoot(ctx, b.ParentRoot())
	if err != nil {
		return nil, errors.Wrapf(err, "could not get pre state for slot %d", b.Slot())
	}
	if preState == nil || preState.IsNil() {
		return nil, errors.Wrapf(err, "nil pre state for slot %d", b.Slot())
	}

	// Verify block slot time is not from the future.
	if err := slots.VerifyTime(uint64(s.genesisTime.Unix()), b.Slot(), params.BeaconConfig().MaximumGossipClockDisparityDuration()); err != nil {
		return nil, err
	}

	// Verify block is later than the finalized epoch slot.
	if err := s.verifyBlkFinalizedSlot(b); err != nil {
		return nil, err
	}

	return preState, nil
}

// verifyBlkPreState validates input block has a valid pre-state.
func (s *Service) verifyBlkPreState(ctx context.Context, parentRoot [field_params.RootLength]byte) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.verifyBlkPreState")
	defer span.End()

	// Loosen the check to HasBlock because state summary gets saved in batches
	// during initial syncing. There's no risk given a state summary object is just a
	// subset of the block object.
	if !s.cfg.BeaconDB.HasStateSummary(ctx, parentRoot) && !s.cfg.BeaconDB.HasBlock(ctx, parentRoot) {
		return errors.New("could not reconstruct parent state")
	}

	has, err := s.cfg.StateGen.HasState(ctx, parentRoot)
	if err != nil {
		return err
	}
	if !has {
		if err := s.cfg.BeaconDB.SaveBlocks(ctx, s.getInitSyncBlocks()); err != nil {
			return errors.Wrap(err, "could not save initial sync blocks")
		}
		s.clearInitSyncBlocks()
	}
	return nil
}

// verifyBlkFinalizedSlot validates input block is not less than or equal
// to current finalized slot.
func (s *Service) verifyBlkFinalizedSlot(b interfaces.ReadOnlyBeaconBlock) error {
	finalized := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	finalizedSlot, err := slots.EpochStart(finalized.Epoch)
	if err != nil {
		return err
	}
	if finalizedSlot >= b.Slot() {
		err = fmt.Errorf("block is equal or earlier than finalized block, slot %d < slot %d", b.Slot(), finalizedSlot)
		return invalidBlock{error: err}
	}
	return nil
}

// updateFinalized saves the init sync blocks, finalized checkpoint, migrates
// to cold old states and saves the last validated checkpoint to DB. It returns
// early if the new checkpoint is older than the one on db.
func (s *Service) updateFinalized(ctx context.Context, cp *ethpb.Checkpoint) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.updateFinalized")
	defer span.End()

	// return early if new checkpoint is not newer than the one in DB
	currentFinalized, err := s.cfg.BeaconDB.FinalizedCheckpoint(ctx)
	if err != nil {
		return err
	}
	if cp.Epoch <= currentFinalized.Epoch {
		return nil
	}

	// Blocks need to be saved so that we can retrieve finalized block from
	// DB when migrating states.
	if err := s.cfg.BeaconDB.SaveBlocks(ctx, s.getInitSyncBlocks()); err != nil {
		return err
	}
	s.clearInitSyncBlocks()

	if err := s.cfg.BeaconDB.SaveFinalizedCheckpoint(ctx, cp); err != nil {
		return err
	}

	fRoot := bytesutil.ToBytes32(cp.Root)
	optimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(fRoot)
	if err != nil && !errors.Is(err, doublylinkedtree.ErrNilNode) {
		return err
	}
	if !optimistic {
		err = s.cfg.BeaconDB.SaveLastValidatedCheckpoint(ctx, cp)
		if err != nil {
			return err
		}
	}
	go func() {
		// We do not pass in the parent context from the method as this method call
		// is meant to be asynchronous and run in the background rather than being
		// tied to the execution of a block.
		if err := s.cfg.StateGen.MigrateToCold(s.ctx, fRoot); err != nil {
			log.WithError(err).Error("could not migrate to cold")
		}
	}()
	return nil
}

// This retrieves an ancestor root using DB. The look up is recursively looking up DB. Slower than `ancestorByForkChoiceStore`.
func (s *Service) ancestorByDB(ctx context.Context, r [32]byte, slot primitives.Slot) (root [32]byte, err error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.ancestorByDB")
	defer span.End()

	root = [32]byte{}
	// Stop recursive ancestry lookup if context is cancelled.
	if ctx.Err() != nil {
		err = ctx.Err()
		return
	}

	signed, err := s.getBlock(ctx, r)
	if err != nil {
		return root, err
	}
	b := signed.Block()
	if b.Slot() == slot || b.Slot() < slot {
		return r, nil
	}

	return s.ancestorByDB(ctx, b.ParentRoot(), slot)
}

// This retrieves missing blocks from DB (ie. the blocks that couldn't be received over sync) and inserts them to fork choice store.
// This is useful for block tree visualizer and additional vote accounting.
func (s *Service) fillInForkChoiceMissingBlocks(ctx context.Context, signed interfaces.ReadOnlySignedBeaconBlock,
	fCheckpoint, jCheckpoint *ethpb.Checkpoint) error {
	pendingNodes := make([]*forkchoicetypes.BlockAndCheckpoints, 0)

	// Fork choice only matters from last finalized slot.
	finalized := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	fSlot, err := slots.EpochStart(finalized.Epoch)
	if err != nil {
		return err
	}
	// The first block can have a bogus root since the block is not inserted in forkchoice
	roblock, err := consensus_blocks.NewROBlockWithRoot(signed, [32]byte{})
	if err != nil {
		return err
	}
	pendingNodes = append(pendingNodes, &forkchoicetypes.BlockAndCheckpoints{Block: roblock,
		JustifiedCheckpoint: jCheckpoint, FinalizedCheckpoint: fCheckpoint})
	// As long as parent node is not in fork choice store, and parent node is in DB.
	root := roblock.Block().ParentRoot()
	for !s.cfg.ForkChoiceStore.HasNode(root) && s.cfg.BeaconDB.HasBlock(ctx, root) {
		b, err := s.getBlock(ctx, root)
		if err != nil {
			return err
		}
		if b.Block().Slot() <= fSlot {
			break
		}
		roblock, err := consensus_blocks.NewROBlockWithRoot(b, root)
		if err != nil {
			return err
		}
		root = b.Block().ParentRoot()
		args := &forkchoicetypes.BlockAndCheckpoints{Block: roblock,
			JustifiedCheckpoint: jCheckpoint,
			FinalizedCheckpoint: fCheckpoint}
		pendingNodes = append(pendingNodes, args)
	}
	if len(pendingNodes) == 1 {
		return nil
	}
	if root != s.ensureRootNotZeros(finalized.Root) && !s.cfg.ForkChoiceStore.HasNode(root) {
		return ErrNotDescendantOfFinalized
	}
	return s.cfg.ForkChoiceStore.InsertChain(ctx, pendingNodes)
}

// inserts finalized deposits into our finalized deposit trie, needs to be
// called in the background
func (s *Service) insertFinalizedDeposits(ctx context.Context, fRoot [32]byte) {
	ctx, span := trace.StartSpan(ctx, "blockChain.insertFinalizedDeposits")
	defer span.End()
	startTime := time.Now()

	// Update deposit cache.
	finalizedState, err := s.cfg.StateGen.StateByRoot(ctx, fRoot)
	if err != nil {
		log.WithError(err).Error("could not fetch finalized state")
		return
	}
	// We update the cache up to the last deposit index in the finalized block's state.
	// We can be confident that these deposits will be included in some block
	// because the Eth1 follow distance makes such long-range reorgs extremely unlikely.
	eth1DepositIndex, err := mathutil.Int(finalizedState.Eth1DepositIndex())
	if err != nil {
		log.WithError(err).Error("could not cast eth1 deposit index")
		return
	}
	// The deposit index in the state is always the index of the next deposit
	// to be included(rather than the last one to be processed). This was most likely
	// done as the state cannot represent signed integers.
	finalizedEth1DepIdx := eth1DepositIndex - 1
	if err = s.cfg.DepositCache.InsertFinalizedDeposits(ctx, int64(finalizedEth1DepIdx), common.Hash(finalizedState.Eth1Data().BlockHash),
		0 /* Setting a zero value as we have no access to block height */); err != nil {
		log.WithError(err).Error("could not insert finalized deposits")
		return
	}
	// Deposit proofs are only used during state transition and can be safely removed to save space.
	if err = s.cfg.DepositCache.PruneProofs(ctx, int64(finalizedEth1DepIdx)); err != nil {
		log.WithError(err).Error("could not prune deposit proofs")
	}
	// Prune deposits which have already been finalized, the below method prunes all pending deposits (non-inclusive) up
	// to the provided eth1 deposit index.
	s.cfg.DepositCache.PrunePendingDeposits(ctx, int64(eth1DepositIndex)) // lint:ignore uintcast -- Deposit index should not exceed int64 in your lifetime.

	log.WithField("duration", time.Since(startTime).String()).Debugf("Finalized deposit insertion completed at index %d", finalizedEth1DepIdx)
}

// This ensures that the input root defaults to using genesis root instead of zero hashes. This is needed for handling
// fork choice justification routine.
func (s *Service) ensureRootNotZeros(root [32]byte) [32]byte {
	if root == params.BeaconConfig().ZeroHash {
		return s.originBlockRoot
	}
	return root
}
