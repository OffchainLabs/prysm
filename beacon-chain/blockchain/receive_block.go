package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed"
	statefeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	coreTime "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/time"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/das"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/slasher/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	consensus_blocks "github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	ethpbv1 "github.com/prysmaticlabs/prysm/v5/proto/eth/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1/attestation"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"golang.org/x/sync/errgroup"
)

// This defines how many epochs since finality the run time will begin to save hot state on to the DB.
var epochsSinceFinalitySaveHotStateDB = primitives.Epoch(100)

// This defines how many epochs since finality the run time will begin to expand our respective cache sizes.
var epochsSinceFinalityExpandCache = primitives.Epoch(4)

// BlockReceiver interface defines the methods of chain service for receiving and processing new blocks.
type BlockReceiver interface {
	ReceiveBlock(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte, avs das.AvailabilityStore) error
	ReceiveExecutionPayloadEnvelope(ctx context.Context, env interfaces.ROExecutionPayloadEnvelope, avs das.AvailabilityStore) error
	ReceiveBlockBatch(ctx context.Context, blocks []blocks.ROBlock, avs das.AvailabilityStore) error
	HasBlock(ctx context.Context, root [32]byte) bool
	RecentBlockSlot(root [32]byte) (primitives.Slot, error)
	BlockBeingSynced([32]byte) bool
}

// PayloadAttestationReceiver defines methods of the chain service for receiving
// and processing new payload attestations and payload attestation messages
type PayloadAttestationReceiver interface {
	ReceivePayloadAttestationMessage(ctx context.Context, a *ethpb.PayloadAttestationMessage) error
}

// BlobReceiver interface defines the methods of chain service for receiving new
// blobs
type BlobReceiver interface {
	ReceiveBlob(context.Context, blocks.VerifiedROBlob) error
}

// ExecutionPayloadReceiver interface defines the methods of chain service for receiving `ROExecutionPayloadEnvelope`.
type ExecutionPayloadReceiver interface {
	ReceiveExecutionPayloadEnvelope(ctx context.Context, envelope interfaces.ROExecutionPayloadEnvelope, _ das.AvailabilityStore) error
}

// SlashingReceiver interface defines the methods of chain service for receiving validated slashing over the wire.
type SlashingReceiver interface {
	ReceiveAttesterSlashing(ctx context.Context, slashing ethpb.AttSlashing)
}

// ReceiveBlock is a function that defines the operations (minus pubsub)
// that are performed on a received block. The operations consist of:
//  1. Validate block, apply state transition and update checkpoints
//  2. Apply fork choice to the processed block
//  3. Save latest head info
func (s *Service) ReceiveBlock(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte, avs das.AvailabilityStore) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.ReceiveBlock")
	defer span.End()
	// Return early if the block has been synced
	if s.InForkchoice(blockRoot) {
		log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot)).Debug("Ignoring already synced block")
		return nil
	}
	receivedTime := time.Now()
	s.blockBeingSynced.set(blockRoot)
	defer s.blockBeingSynced.unset(blockRoot)

	blockCopy, err := block.Copy()
	if err != nil {
		return err
	}
	preState, err := s.getBlockPreState(ctx, blockCopy.Block())
	if err != nil {
		return errors.Wrap(err, "could not get block's prestate")
	}

	currentCheckpoints := s.saveCurrentCheckpoints(preState)
	postState, isValidPayload, err := s.validateExecutionAndConsensus(ctx, preState, blockCopy, blockRoot)
	if err != nil {
		return err
	}
	daWaitedTime, err := s.handleDA(ctx, blockCopy, blockRoot, avs)
	if err != nil {
		return err
	}
	// Defragment the state before continuing block processing.
	s.defragmentState(postState)

	// The rest of block processing takes a lock on forkchoice.
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	if err := s.savePostStateInfo(ctx, blockRoot, blockCopy, postState); err != nil {
		return errors.Wrap(err, "could not save post state info")
	}
	roblock, err := consensus_blocks.NewROBlockWithRoot(blockCopy, blockRoot)
	if err != nil {
		return err
	}
	args := &postBlockProcessConfig{
		ctx:            ctx,
		roblock:        roblock,
		postState:      postState,
		isValidPayload: isValidPayload,
	}
	if err := s.postBlockProcess(args); err != nil {
		err := errors.Wrap(err, "could not process block")
		tracing.AnnotateError(span, err)
		return err
	}
	if err := s.updateCheckpoints(ctx, currentCheckpoints, preState, postState, blockRoot); err != nil {
		return err
	}
	// If slasher is configured, forward the attestations in the block via an event feed for processing.
	if features.Get().EnableSlasher {
		go s.sendBlockAttestationsToSlasher(blockCopy, preState)
	}

	// Handle post block operations such as pruning exits and bls messages if incoming block is the head
	if err := s.prunePostBlockOperationPools(ctx, blockCopy, blockRoot); err != nil {
		log.WithError(err).Error("Could not prune canonical objects from pool ")
	}

	// Have we been finalizing? Should we start saving hot states to db?
	if err := s.checkSaveHotStateDB(ctx); err != nil {
		return err
	}

	// We apply the same heuristic to some of our more important caches.
	if err := s.handleCaches(); err != nil {
		return err
	}
	s.reportPostBlockProcessing(blockCopy, blockRoot, receivedTime, daWaitedTime)
	return nil
}

type ffgCheckpoints struct {
	j, f, c primitives.Epoch
}

func (s *Service) saveCurrentCheckpoints(state state.BeaconState) (cp ffgCheckpoints) {
	// Save current justified and finalized epochs for future use.
	cp.j = s.CurrentJustifiedCheckpt().Epoch
	cp.f = s.FinalizedCheckpt().Epoch
	cp.c = coreTime.CurrentEpoch(state)
	return
}

func (s *Service) updateCheckpoints(
	ctx context.Context,
	cp ffgCheckpoints,
	preState, postState state.BeaconState,
	blockRoot [32]byte,
) error {
	if coreTime.CurrentEpoch(postState) > cp.c && s.cfg.ForkChoiceStore.IsCanonical(blockRoot) {
		headSt, err := s.HeadState(ctx)
		if err != nil {
			return errors.Wrap(err, "could not get head state")
		}
		if err := reportEpochMetrics(ctx, postState, headSt); err != nil {
			log.WithError(err).Error("could not report epoch metrics")
		}
	}
	if err := s.updateJustificationOnBlock(ctx, preState, postState, cp.j); err != nil {
		return errors.Wrap(err, "could not update justified checkpoint")
	}

	newFinalized, err := s.updateFinalizationOnBlock(ctx, preState, postState, cp.f)
	if err != nil {
		return errors.Wrap(err, "could not update finalized checkpoint")
	}
	// Send finalized events and finalized deposits in the background
	if newFinalized {
		// hook to process all post state finalization tasks
		s.executePostFinalizationTasks(ctx, postState)
	}
	return nil
}

func (s *Service) validateExecutionAndConsensus(
	ctx context.Context,
	preState state.BeaconState,
	block interfaces.SignedBeaconBlock,
	blockRoot [32]byte,
) (state.BeaconState, bool, error) {
	preStateVersion, preStateHeader, err := getStateVersionAndPayload(preState)
	if err != nil {
		return nil, false, err
	}
	eg, _ := errgroup.WithContext(ctx)
	var postState state.BeaconState
	eg.Go(func() error {
		var err error
		postState, err = s.validateStateTransition(ctx, preState, block)
		if err != nil {
			return errors.Wrap(err, "failed to validate consensus state transition function")
		}
		return nil
	})
	var isValidPayload bool
	eg.Go(func() error {
		var err error
		isValidPayload, err = s.validateExecutionOnBlock(ctx, preStateVersion, preStateHeader, block, blockRoot)
		if err != nil {
			return errors.Wrap(err, "could not notify the engine of the new payload")
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return nil, false, err
	}
	return postState, isValidPayload, nil
}

func (s *Service) handleDA(
	ctx context.Context,
	block interfaces.SignedBeaconBlock,
	blockRoot [32]byte,
	avs das.AvailabilityStore,
) (time.Duration, error) {
	daStartTime := time.Now()
	if avs != nil {
		rob, err := blocks.NewROBlockWithRoot(block, blockRoot)
		if err != nil {
			return 0, err
		}
		if err := avs.IsDataAvailable(ctx, s.CurrentSlot(), rob); err != nil {
			return 0, errors.Wrap(err, "could not validate blob data availability (AvailabilityStore.IsDataAvailable)")
		}
	} else {
		if err := s.isDataAvailable(ctx, blockRoot, block); err != nil {
			return 0, errors.Wrap(err, "could not validate blob data availability")
		}
	}
	daWaitedTime := time.Since(daStartTime)
	dataAvailWaitedTime.Observe(float64(daWaitedTime.Milliseconds()))
	return daWaitedTime, nil
}

func (s *Service) reportPostBlockProcessing(
	block interfaces.SignedBeaconBlock,
	blockRoot [32]byte,
	receivedTime time.Time,
	daWaitedTime time.Duration,
) {
	// Reports on block and fork choice metrics.
	cp := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	finalized := &ethpb.Checkpoint{Epoch: cp.Epoch, Root: bytesutil.SafeCopyBytes(cp.Root[:])}
	reportSlotMetrics(block.Block().Slot(), s.HeadSlot(), s.CurrentSlot(), finalized)

	// Log block sync status.
	cp = s.cfg.ForkChoiceStore.JustifiedCheckpoint()
	justified := &ethpb.Checkpoint{Epoch: cp.Epoch, Root: bytesutil.SafeCopyBytes(cp.Root[:])}
	if err := logBlockSyncStatus(block.Block(), blockRoot, justified, finalized, receivedTime, uint64(s.genesisTime.Unix()), daWaitedTime); err != nil {
		log.WithError(err).Error("Unable to log block sync status")
	}
	// Log payload data
	if err := logPayload(block.Block()); err != nil {
		log.WithError(err).Error("Unable to log debug block payload data")
	}
	// Log state transition data.
	if err := logStateTransitionData(block.Block()); err != nil {
		log.WithError(err).Error("Unable to log state transition data")
	}
	timeWithoutDaWait := time.Since(receivedTime) - daWaitedTime
	chainServiceProcessingTime.Observe(float64(timeWithoutDaWait.Milliseconds()))
}

func (s *Service) executePostFinalizationTasks(ctx context.Context, finalizedState state.BeaconState) {
	finalized := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	go func() {
		s.sendNewFinalizedEvent(ctx, finalizedState)
	}()
	depCtx, cancel := context.WithTimeout(context.Background(), depositDeadline)
	go func() {
		s.insertFinalizedDeposits(depCtx, finalized.Root)
		cancel()
	}()
}

// ReceiveBlockBatch processes the whole block batch at once, assuming the block batch is linear ,transitioning
// the state, performing batch verification of all collected signatures and then performing the appropriate
// actions for a block post-transition.
func (s *Service) ReceiveBlockBatch(ctx context.Context, blocks []blocks.ROBlock, avs das.AvailabilityStore) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.ReceiveBlockBatch")
	defer span.End()

	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()

	// Apply state transition on the incoming newly received block batches, one by one.
	if err := s.onBlockBatch(ctx, blocks, avs); err != nil {
		err := errors.Wrap(err, "could not process block in batch")
		tracing.AnnotateError(span, err)
		return err
	}

	lastBR := blocks[len(blocks)-1].Root()
	optimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(lastBR)
	if err != nil {
		lastSlot := blocks[len(blocks)-1].Block().Slot()
		log.WithError(err).Errorf("Could not check if block is optimistic, Root: %#x, Slot: %d", lastBR, lastSlot)
		optimistic = true
	}

	for _, b := range blocks {
		blockCopy, err := b.Copy()
		if err != nil {
			return err
		}
		// Send notification of the processed block to the state feed.
		s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.BlockProcessed,
			Data: &statefeed.BlockProcessedData{
				Slot:        blockCopy.Block().Slot(),
				BlockRoot:   b.Root(),
				SignedBlock: blockCopy,
				Verified:    true,
				Optimistic:  optimistic,
			},
		})

		// Reports on blockCopy and fork choice metrics.
		cp := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
		finalized := &ethpb.Checkpoint{Epoch: cp.Epoch, Root: bytesutil.SafeCopyBytes(cp.Root[:])}
		reportSlotMetrics(blockCopy.Block().Slot(), s.HeadSlot(), s.CurrentSlot(), finalized)
	}

	if err := s.cfg.BeaconDB.SaveBlocks(ctx, s.getInitSyncBlocks()); err != nil {
		return err
	}
	finalized := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	if finalized == nil {
		return errNilFinalizedInStore
	}
	if err := s.wsVerifier.VerifyWeakSubjectivity(s.ctx, finalized.Epoch); err != nil {
		// log.Fatalf will prevent defer from being called
		span.End()
		// Exit run time if the node failed to verify weak subjectivity checkpoint.
		log.WithError(err).Fatal("Could not verify weak subjectivity checkpoint")
	}

	return nil
}

// HasBlock returns true if the block of the input root exists in initial sync blocks cache or DB.
func (s *Service) HasBlock(ctx context.Context, root [32]byte) bool {
	if s.BlockBeingSynced(root) {
		return false
	}
	return s.hasBlockInInitSyncOrDB(ctx, root)
}

// ReceiveAttesterSlashing receives an attester slashing and inserts it to forkchoice
func (s *Service) ReceiveAttesterSlashing(ctx context.Context, slashing ethpb.AttSlashing) {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.InsertSlashingsToForkChoiceStore(ctx, []ethpb.AttSlashing{slashing})
}

// prunePostBlockOperationPools only runs on new head otherwise should return a nil.
func (s *Service) prunePostBlockOperationPools(ctx context.Context, blk interfaces.ReadOnlySignedBeaconBlock, root [32]byte) error {
	headRoot, err := s.HeadRoot(ctx)
	if err != nil {
		return err
	}
	// By comparing the current headroot, that has already gone through forkchoice,
	// we can assume that if equal the current block root is canonical.
	if !bytes.Equal(headRoot, root[:]) {
		return nil
	}

	// Mark block exits as seen so we don't include same ones in future blocks.
	for _, e := range blk.Block().Body().VoluntaryExits() {
		s.cfg.ExitPool.MarkIncluded(e)
	}

	// Mark block BLS changes as seen so we don't include same ones in future blocks.
	if err := s.markIncludedBlockBLSToExecChanges(blk.Block()); err != nil {
		return errors.Wrap(err, "could not process BLSToExecutionChanges")
	}

	// Mark slashings as seen so we don't include same ones in future blocks.
	for _, as := range blk.Block().Body().AttesterSlashings() {
		s.cfg.SlashingPool.MarkIncludedAttesterSlashing(as)
	}
	for _, ps := range blk.Block().Body().ProposerSlashings() {
		s.cfg.SlashingPool.MarkIncludedProposerSlashing(ps)
	}

	return nil
}

func (s *Service) markIncludedBlockBLSToExecChanges(headBlock interfaces.ReadOnlyBeaconBlock) error {
	if headBlock.Version() < version.Capella {
		return nil
	}
	changes, err := headBlock.Body().BLSToExecutionChanges()
	if err != nil {
		return errors.Wrap(err, "could not get BLSToExecutionChanges")
	}
	for _, change := range changes {
		s.cfg.BLSToExecPool.MarkIncluded(change)
	}
	return nil
}

// This checks whether it's time to start saving hot state to DB.
// It's time when there's `epochsSinceFinalitySaveHotStateDB` epochs of non-finality.
// Requires a read lock on forkchoice
func (s *Service) checkSaveHotStateDB(ctx context.Context) error {
	currentEpoch := slots.ToEpoch(s.CurrentSlot())
	// Prevent `sinceFinality` going underflow.
	var sinceFinality primitives.Epoch
	finalized := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	if finalized == nil {
		return errNilFinalizedInStore
	}
	if currentEpoch > finalized.Epoch {
		sinceFinality = currentEpoch - finalized.Epoch
	}

	if sinceFinality >= epochsSinceFinalitySaveHotStateDB {
		s.cfg.StateGen.EnableSaveHotStateToDB(ctx)
		return nil
	}

	return s.cfg.StateGen.DisableSaveHotStateToDB(ctx)
}

func (s *Service) handleCaches() error {
	currentEpoch := slots.ToEpoch(s.CurrentSlot())
	// Prevent `sinceFinality` going underflow.
	var sinceFinality primitives.Epoch
	finalized := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	if finalized == nil {
		return errNilFinalizedInStore
	}
	if currentEpoch > finalized.Epoch {
		sinceFinality = currentEpoch - finalized.Epoch
	}

	if sinceFinality >= epochsSinceFinalityExpandCache {
		helpers.ExpandCommitteeCache()
		return nil
	}

	helpers.CompressCommitteeCache()
	return nil
}

// This performs the state transition function and returns the poststate or an
// error if the block fails to verify the consensus rules
func (s *Service) validateStateTransition(ctx context.Context, preState state.BeaconState, signed interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
	b := signed.Block()
	// Verify that the parent block is in forkchoice
	parentRoot := b.ParentRoot()
	if !s.InForkchoice(parentRoot) {
		return nil, ErrNotDescendantOfFinalized
	}
	stateTransitionStartTime := time.Now()
	postState, err := transition.ExecuteStateTransition(ctx, preState, signed)
	if err != nil {
		return nil, invalidBlock{error: err}
	}
	stateTransitionProcessingTime.Observe(float64(time.Since(stateTransitionStartTime).Milliseconds()))
	return postState, nil
}

// updateJustificationOnBlock updates the justified checkpoint on DB if the
// incoming block has updated it on forkchoice.
func (s *Service) updateJustificationOnBlock(ctx context.Context, preState, postState state.BeaconState, preJustifiedEpoch primitives.Epoch) error {
	justified := s.cfg.ForkChoiceStore.JustifiedCheckpoint()
	preStateJustifiedEpoch := preState.CurrentJustifiedCheckpoint().Epoch
	postStateJustifiedEpoch := postState.CurrentJustifiedCheckpoint().Epoch
	if justified.Epoch > preJustifiedEpoch || (justified.Epoch == postStateJustifiedEpoch && justified.Epoch > preStateJustifiedEpoch) {
		if err := s.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, &ethpb.Checkpoint{
			Epoch: justified.Epoch, Root: justified.Root[:],
		}); err != nil {
			return err
		}
	}
	return nil
}

// updateFinalizationOnBlock performs some duties when the incoming block
// changes the finalized checkpoint. It returns true when this has happened.
func (s *Service) updateFinalizationOnBlock(ctx context.Context, preState, postState state.BeaconState, preFinalizedEpoch primitives.Epoch) (bool, error) {
	preStateFinalizedEpoch := preState.FinalizedCheckpoint().Epoch
	postStateFinalizedEpoch := postState.FinalizedCheckpoint().Epoch
	finalized := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	if finalized.Epoch > preFinalizedEpoch || (finalized.Epoch == postStateFinalizedEpoch && finalized.Epoch > preStateFinalizedEpoch) {
		if err := s.updateFinalized(ctx, &ethpb.Checkpoint{Epoch: finalized.Epoch, Root: finalized.Root[:]}); err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

// sendNewFinalizedEvent sends a new finalization checkpoint event over the
// event feed. It needs to be called on the background
func (s *Service) sendNewFinalizedEvent(ctx context.Context, postState state.BeaconState) {
	isValidPayload := false
	s.headLock.RLock()
	if s.head != nil {
		isValidPayload = s.head.optimistic
	}
	s.headLock.RUnlock()

	blk, err := s.cfg.BeaconDB.Block(ctx, bytesutil.ToBytes32(postState.FinalizedCheckpoint().Root))
	if err != nil {
		log.WithError(err).Error("Could not retrieve block for finalized checkpoint root. Finalized event will not be emitted")
		return
	}
	if blk == nil || blk.IsNil() || blk.Block() == nil || blk.Block().IsNil() {
		log.WithError(err).Error("Block retrieved for finalized checkpoint root is nil. Finalized event will not be emitted")
		return
	}
	stateRoot := blk.Block().StateRoot()
	// Send an event regarding the new finalized checkpoint over a common event feed.
	s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.FinalizedCheckpoint,
		Data: &ethpbv1.EventFinalizedCheckpoint{
			Epoch:               postState.FinalizedCheckpoint().Epoch,
			Block:               postState.FinalizedCheckpoint().Root,
			State:               stateRoot[:],
			ExecutionOptimistic: isValidPayload,
		},
	})
}

// sendBlockAttestationsToSlasher sends the incoming block's attestation to the slasher
func (s *Service) sendBlockAttestationsToSlasher(signed interfaces.ReadOnlySignedBeaconBlock, preState state.BeaconState) {
	// Feed the indexed attestation to slasher if enabled. This action
	// is done in the background to avoid adding more load to this critical code path.
	ctx := context.TODO()
	for _, att := range signed.Block().Body().Attestations() {
		committees, err := helpers.AttestationCommittees(ctx, preState, att)
		if err != nil {
			log.WithError(err).Error("Could not get attestation committees")
			return
		}
		indexedAtt, err := attestation.ConvertToIndexed(ctx, att, committees...)
		if err != nil {
			log.WithError(err).Error("Could not convert to indexed attestation")
			return
		}
		s.cfg.SlasherAttestationsFeed.Send(&types.WrappedIndexedAtt{IndexedAtt: indexedAtt})
	}
}

// validateExecutionOnBlock notifies the engine of the incoming block execution payload and returns true if the payload is valid
func (s *Service) validateExecutionOnBlock(ctx context.Context, ver int, header interfaces.ExecutionData, signed interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte) (bool, error) {
	isValidPayload, err := s.notifyNewPayload(ctx, ver, header, signed)
	if err != nil {
		s.cfg.ForkChoiceStore.Lock()
		err = s.handleInvalidExecutionError(ctx, err, blockRoot, signed.Block().ParentRoot())
		s.cfg.ForkChoiceStore.Unlock()
		return false, err
	}
	if signed.Version() < version.Capella && isValidPayload {
		if err := s.validateMergeTransitionBlock(ctx, ver, header, signed); err != nil {
			return isValidPayload, err
		}
	}
	return isValidPayload, nil
}
