package blockchain

import (
	"fmt"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	forkchoicetypes "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/features"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// ValidationStage handles initial validation checks
type ValidationStage struct {
	service *Service
}

func (s *ValidationStage) Name() string { return "validation" }
func (s *ValidationStage) SupportsBatch() bool { return true }

func (s *ValidationStage) Execute(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	for i, block := range blocks {
		blockRoot := ctx.BlockRoots[i]
		
		// Check if block is blacklisted
		if features.BlacklistedBlock(blockRoot) {
			return errBlacklistedRoot
		}
		
		// For single mode, check if already synced
		if ctx.Mode == ModeSingle {
			if s.service.InForkchoice(blockRoot) {
				return fmt.Errorf("block already synced: %#x", blockRoot)
			}
			
			// Set block as being synced
			err := s.service.blockBeingSynced.set(blockRoot)
			if errors.Is(err, errBlockBeingSynced) {
				return fmt.Errorf("block currently being synced: %#x", blockRoot)
			}
			defer s.service.blockBeingSynced.unset(blockRoot)
		}
		
		// Validate block structure
		if err := consensusblocks.BeaconBlockIsNil(block); err != nil {
			return invalidBlock{error: err}
		}
	}
	
	return nil
}

// StateTransitionStage handles state transitions
type StateTransitionStage struct {
	service *Service
}

func (s *StateTransitionStage) Name() string { return "state_transition" }
func (s *StateTransitionStage) SupportsBatch() bool { return true }

func (s *StateTransitionStage) Execute(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	if ctx.Mode == ModeBatch && len(blocks) > 1 {
		return s.executeBatch(ctx, blocks)
	}
	return s.executeSingle(ctx, blocks[0])
}

func (s *StateTransitionStage) executeSingle(ctx *ProcessingContext, block consensusblocks.ROBlock) error {
	idx := ctx.CurrentBlockIndex
	
	// Get pre-state
	preState, err := s.service.getBlockPreState(ctx.Context, block.Block())
	if err != nil {
		return errors.Wrap(err, "could not get block's prestate")
	}
	ctx.PreStates[0] = preState
	ctx.CurrentPreState = preState
	
	// Execute state transition
	postState, err := s.service.validateStateTransition(ctx.Context, preState, block)
	if err != nil {
		return err
	}
	ctx.States[0] = postState
	ctx.CurrentState = postState
	
	// Save current checkpoints
	cp := s.service.saveCurrentCheckpoints(preState)
	ctx.Checkpoints[idx] = []*ethpb.Checkpoint{
		{Epoch: cp.j, Root: nil},
		{Epoch: cp.f, Root: nil},
		{Epoch: cp.c, Root: nil},
	}
	
	return nil
}

func (s *StateTransitionStage) executeBatch(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	if len(blocks) == 0 {
		return errors.New("no blocks provided")
	}
	
	// Get initial pre-state
	preState, err := s.service.cfg.StateGen.StateByRootInitialSync(ctx.Context, blocks[0].Block().ParentRoot())
	if err != nil {
		return err
	}
	if preState == nil || preState.IsNil() {
		return fmt.Errorf("nil pre state for slot %d", blocks[0].Block().Slot())
	}
	
	// Fill in missing blocks for forkchoice
	if err := s.service.fillInForkChoiceMissingBlocks(ctx.Context, blocks[0], preState.CurrentJustifiedCheckpoint(), preState.FinalizedCheckpoint()); err != nil {
		return errors.Wrap(err, "could not fill in missing blocks to forkchoice")
	}
	
	// Process each block in sequence, reusing the same state variable
	for i, block := range blocks {
		// Store pre-state for validation stages that need it
		ctx.CurrentPreState = preState.Copy()
		
		// Execute state transition without signature verification
		var set *bls.SignatureBatch
		set, preState, err = transition.ExecuteStateTransitionNoVerifyAnySig(ctx.Context, preState, block)
		if err != nil {
			return invalidBlock{error: err}
		}
		
		// Save boundary states at epoch transitions (like original)
		if slots.IsEpochStart(preState.Slot()) {
			ctx.BoundaryStates[block.Root()] = preState.Copy()
		}
		
		ctx.CurrentState = preState
		ctx.SigSets[i] = set
		
		// Save checkpoints
		ctx.Checkpoints[i] = []*ethpb.Checkpoint{
			preState.CurrentJustifiedCheckpoint(),
			preState.FinalizedCheckpoint(),
		}
	}
	
	return nil
}

// SignatureVerificationStage handles signature verification
type SignatureVerificationStage struct {
	service *Service
}

func (s *SignatureVerificationStage) Name() string { return "signature_verification" }
func (s *SignatureVerificationStage) SupportsBatch() bool { return true }

func (s *SignatureVerificationStage) Execute(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	if ctx.Mode == ModeBatch && len(blocks) > 1 {
		return s.verifyBatchSignatures(ctx)
	}
	return s.verifySingleSignatures(ctx, blocks[0])
}

func (s *SignatureVerificationStage) verifyBatchSignatures(ctx *ProcessingContext) error {
	sigSet := bls.NewSet()
	for _, set := range ctx.SigSets {
		if set != nil {
			sigSet.Join(set)
		}
	}
	
	var verify bool
	var err error
	if features.Get().EnableVerboseSigVerification {
		verify, err = sigSet.VerifyVerbosely()
	} else {
		verify, err = sigSet.Verify()
	}
	if err != nil {
		return invalidBlock{error: err}
	}
	if !verify {
		return errors.New("batch block signature verification failed")
	}
	
	return nil
}

func (s *SignatureVerificationStage) verifySingleSignatures(ctx *ProcessingContext, block consensusblocks.ROBlock) error {
	// For single blocks, signature verification is done during state transition
	// This stage is mainly for batch mode
	return nil
}

// ExecutionValidationStage handles execution layer validation
type ExecutionValidationStage struct {
	service *Service
}

func (s *ExecutionValidationStage) Name() string { return "execution_validation" }
func (s *ExecutionValidationStage) SupportsBatch() bool { return true }

func (s *ExecutionValidationStage) Execute(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	if ctx.Mode == ModeBatch && len(blocks) > 1 {
		return s.validateBatchExecution(ctx, blocks)
	}
	return s.validateSingleExecution(ctx, blocks[0])
}

func (s *ExecutionValidationStage) validateSingleExecution(ctx *ProcessingContext, block consensusblocks.ROBlock) error {
	idx := ctx.CurrentBlockIndex
	preStateVersion, preStateHeader, err := getStateVersionAndPayload(ctx.PreStates[0])
	if err != nil {
		return err
	}
	
	eg, _ := errgroup.WithContext(ctx.Context)
	var isValidPayload bool
	
	eg.Go(func() error {
		var err error
		isValidPayload, err = s.service.validateExecutionOnBlock(ctx.Context, preStateVersion, preStateHeader, block)
		if err != nil {
			return errors.Wrap(err, "could not notify the engine of the new payload")
		}
		return nil
	})
	
	if err := eg.Wait(); err != nil {
		return err
	}
	
	ctx.IsValidPayloads[idx] = isValidPayload
	return nil
}

func (s *ExecutionValidationStage) validateBatchExecution(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	// Need to process blocks to get the states for validation
	// This is called after StateTransitionStage, so CurrentState has the final state
	// We need to replay to get intermediate states for validation
	
	// Get initial pre-state
	preState, err := s.service.cfg.StateGen.StateByRootInitialSync(ctx.Context, blocks[0].Block().ParentRoot())
	if err != nil {
		return err
	}
	
	for i, block := range blocks {
		preVersion, preHeader, err := getStateVersionAndPayload(preState)
		if err != nil {
			return err
		}
		
		// Execute transition to get post state (already done in StateTransitionStage, but we need intermediate states)
		var postState state.BeaconState
		if i == len(blocks)-1 {
			// Last block, use the current state
			postState = ctx.CurrentState
		} else {
			// Need to compute intermediate state
			_, postState, err = transition.ExecuteStateTransitionNoVerifyAnySig(ctx.Context, preState, block)
			if err != nil {
				return err
			}
		}
		
		postVersion, postHeader, err := getStateVersionAndPayload(postState)
		if err != nil {
			return err
		}
		
		isValidPayload, err := s.service.notifyNewPayload(ctx.Context,
			postVersion, postHeader, block)
		if err != nil {
			blockRoot := ctx.BlockRoots[i]
			return s.service.handleInvalidExecutionError(ctx.Context, err, blockRoot, block.Block().ParentRoot())
		}
		
		ctx.IsValidPayloads[i] = isValidPayload
		
		if isValidPayload {
			if err := s.service.validateMergeTransitionBlock(ctx.Context, preVersion,
				preHeader, block); err != nil {
				return err
			}
		}
		
		preState = postState
	}
	
	return nil
}

// DataAvailabilityStage handles data availability checks
type DataAvailabilityStage struct {
	service *Service
}

func (s *DataAvailabilityStage) Name() string { return "data_availability" }
func (s *DataAvailabilityStage) SupportsBatch() bool { return false }

func (s *DataAvailabilityStage) Execute(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	// DA checks are done per block
	block := blocks[0]
	idx := ctx.CurrentBlockIndex
	blockRoot := ctx.BlockRoots[idx]
	
	start := time.Now()
	defer func() {
		ctx.DAWaitedTime = time.Since(start)
	}()
	
	if ctx.AVS == nil {
		blockCopy, err := block.Copy()
		if err != nil {
			return err
		}
		return s.service.isDataAvailable(ctx.Context, blockRoot, blockCopy)
	}
	
	return ctx.AVS.IsDataAvailable(ctx.Context, s.service.CurrentSlot(), block)
}

// ForkchoiceStage handles forkchoice operations
type ForkchoiceStage struct {
	service *Service
}

func (s *ForkchoiceStage) Name() string { return "forkchoice" }
func (s *ForkchoiceStage) SupportsBatch() bool { return true }

func (s *ForkchoiceStage) Execute(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	if ctx.Mode == ModeBatch && len(blocks) > 1 {
		return s.executeBatchForkchoice(ctx, blocks)
	}
	return s.executeSingleForkchoice(ctx, blocks[0])
}

func (s *ForkchoiceStage) executeSingleForkchoice(ctx *ProcessingContext, block consensusblocks.ROBlock) error {
	idx := ctx.CurrentBlockIndex
	blockRoot := ctx.BlockRoots[idx]
	
	// Save post state info
	blockCopy, err := block.Copy()
	if err != nil {
		return err
	}
	
	if err := s.service.savePostStateInfo(ctx.Context, blockRoot, blockCopy, ctx.States[0]); err != nil {
		return errors.Wrap(err, "could not save post state info")
	}
	
	// Execute post block processing
	args := &postBlockProcessConfig{
		ctx:            ctx.Context,
		roblock:        block,
		postState:      ctx.States[0],
		isValidPayload: ctx.IsValidPayloads[idx],
	}
	
	return s.service.postBlockProcess(args)
}

func (s *ForkchoiceStage) executeBatchForkchoice(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	// Save blocks and prepare forkchoice nodes
	pendingNodes := make([]*forkchoicetypes.BlockAndCheckpoints, len(blocks))
	
	for i, b := range blocks {
		root := b.Root()
		
		// Save block to database
		if err := s.service.saveInitSyncBlock(ctx.Context, root, b); err != nil {
			return err
		}
		
		// Save state summary
		if err := s.service.cfg.BeaconDB.SaveStateSummary(ctx.Context, &ethpb.StateSummary{
			Slot: b.Block().Slot(),
			Root: root[:],
		}); err != nil {
			return err
		}
		
		// Prepare forkchoice node
		args := &forkchoicetypes.BlockAndCheckpoints{
			Block:               b,
			JustifiedCheckpoint: ctx.Checkpoints[i][0],
			FinalizedCheckpoint: ctx.Checkpoints[i][1],
		}
		pendingNodes[len(blocks)-i-1] = args
		
		// Update justified/finalized checkpoints if needed
		if i > 0 && ctx.Checkpoints[i][0].Epoch > ctx.Checkpoints[i-1][0].Epoch {
			if err := s.service.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx.Context, ctx.Checkpoints[i][0]); err != nil {
				return err
			}
		}
		if i > 0 && ctx.Checkpoints[i][1].Epoch > ctx.Checkpoints[i-1][1].Epoch {
			if err := s.service.updateFinalized(ctx.Context, ctx.Checkpoints[i][1]); err != nil {
				return err
			}
		}
	}
	
	// Save boundary states
	for r, st := range ctx.BoundaryStates {
		if err := s.service.cfg.StateGen.SaveState(ctx.Context, r, st); err != nil {
			return err
		}
	}
	
	lastBlock := blocks[len(blocks)-1]
	lastRoot := ctx.BlockRoots[len(blocks)-1]
	
	// Save the final state
	if err := s.service.cfg.StateGen.SaveState(ctx.Context, lastRoot, ctx.CurrentState); err != nil {
		return err
	}
	
	// Insert all nodes but the last one to forkchoice
	if len(pendingNodes) > 1 {
		if err := s.service.cfg.ForkChoiceStore.InsertChain(ctx.Context, pendingNodes[:len(pendingNodes)-1]); err != nil {
			return errors.Wrap(err, "could not insert batch to forkchoice")
		}
	}
	
	// Insert the last block to forkchoice
	if err := s.service.cfg.ForkChoiceStore.InsertNode(ctx.Context, ctx.CurrentState, lastBlock); err != nil {
		return errors.Wrap(err, "could not insert last block in batch to forkchoice")
	}
	
	// Set optimistic status
	if ctx.IsValidPayloads[len(blocks)-1] {
		if err := s.service.cfg.ForkChoiceStore.SetOptimisticToValid(ctx.Context, lastRoot); err != nil {
			return errors.Wrap(err, "could not set optimistic block to valid")
		}
	}
	
	// Notify forkchoice update
	fcuArgs := &fcuConfig{
		headState: ctx.CurrentState,
		headRoot:  lastRoot,
		headBlock: lastBlock,
	}
	if _, err := s.service.notifyForkchoiceUpdate(ctx.Context, fcuArgs); err != nil {
		return err
	}
	
	return s.service.saveHeadNoDB(ctx.Context, lastBlock, lastRoot, ctx.CurrentState, !ctx.IsValidPayloads[len(blocks)-1])
}

// PostProcessingStage handles post-processing operations
type PostProcessingStage struct {
	service *Service
}

func (s *PostProcessingStage) Name() string { return "post_processing" }
func (s *PostProcessingStage) SupportsBatch() bool { return false }

func (s *PostProcessingStage) Execute(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	if ctx.Mode == ModeBatch {
		return s.executeBatchPostProcessing(ctx, blocks)
	}
	return s.executeSinglePostProcessing(ctx, blocks[0])
}

func (s *PostProcessingStage) executeSinglePostProcessing(ctx *ProcessingContext, block consensusblocks.ROBlock) error {
	idx := ctx.CurrentBlockIndex
	blockRoot := ctx.BlockRoots[idx]
	preState := ctx.PreStates[0]
	postState := ctx.States[0]
	
	// Update checkpoints
	cp := ffgCheckpoints{
		j: ctx.Checkpoints[idx][0].Epoch,
		f: ctx.Checkpoints[idx][1].Epoch,
		c: ctx.Checkpoints[idx][2].Epoch,
	}
	
	if err := s.service.updateCheckpoints(ctx.Context, cp, preState, postState, blockRoot); err != nil {
		return err
	}
	
	// Handle slasher if enabled
	if s.service.slasherEnabled {
		blockCopy, err := block.Copy()
		if err != nil {
			return err
		}
		go s.service.sendBlockAttestationsToSlasher(blockCopy, preState)
	}
	
	// Prune operation pools
	blockCopy, err := block.Copy()
	if err != nil {
		return err
	}
	if err := s.service.prunePostBlockOperationPools(ctx.Context, blockCopy, blockRoot); err != nil {
		log.WithError(err).Error("Could not prune canonical objects from pool")
	}
	
	// Check save hot state DB
	if err := s.service.checkSaveHotStateDB(ctx.Context); err != nil {
		return err
	}
	
	// Handle caches
	if err := s.service.handleCaches(); err != nil {
		return err
	}
	
	// Report processing metrics
	s.service.reportPostBlockProcessing(blockCopy, blockRoot, ctx.ReceivedTime, ctx.DAWaitedTime)
	
	return nil
}

func (s *PostProcessingStage) executeBatchPostProcessing(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	// For batch mode, minimal post-processing is done
	// Most of the batch-specific work is already done in ForkchoiceStage
	return nil
}