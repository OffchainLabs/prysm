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
		if ctx.IsSingle() {
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

// StateTransitionExecutionAndDAStage combines state transition, execution validation, and DA checks
// into a single stage for optimal batch performance while maintaining code reuse
type StateTransitionExecutionAndDAStage struct {
	service *Service
}

func (s *StateTransitionExecutionAndDAStage) Name() string { return "state_transition_execution_and_da" }
func (s *StateTransitionExecutionAndDAStage) SupportsBatch() bool { return true }

func (s *StateTransitionExecutionAndDAStage) Execute(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	if ctx.IsSingle() {
		return s.processSingleBlock(ctx, blocks[0])
	}
	return s.processBatchBlocks(ctx, blocks)
}

// ProcessMode defines how to execute block processing
type ProcessMode int

const (
	ProcessParallel ProcessMode = iota // Run state transition and execution in parallel
	ProcessSequential                  // Run state transition and execution sequentially
)

// BlockProcessResult contains the result of processing a single block
type BlockProcessResult struct {
	preState        state.BeaconState
	postState       state.BeaconState
	isValidPayload  bool
	sigSet          *bls.SignatureBatch
	checkpoints     []*ethpb.Checkpoint
	daWaitedTime    time.Duration
}

// processBlock handles the core validation logic shared between single and batch modes
func (s *StateTransitionExecutionAndDAStage) processBlock(
	ctx *ProcessingContext,
	block consensusblocks.ROBlock,
	blockIndex int,
	preState state.BeaconState,
	mode ProcessMode,
) (*BlockProcessResult, error) {
	blockRoot := ctx.BlockRoots[blockIndex]
	result := &BlockProcessResult{
		preState: preState,
	}

	// Save current checkpoints (reused logic)
	cp := s.service.saveCurrentCheckpoints(preState)
	result.checkpoints = []*ethpb.Checkpoint{
		{Epoch: cp.j, Root: nil},
		{Epoch: cp.f, Root: nil},
		{Epoch: cp.c, Root: nil},
	}

	// Get state version info for execution validation (reused logic)
	preStateVersion, preStateHeader, err := getStateVersionAndPayload(preState)
	if err != nil {
		return nil, err
	}

	if mode == ProcessParallel {
		// Single mode: Run state transition and execution validation IN PARALLEL
		eg, _ := errgroup.WithContext(ctx.Context)
		var postState state.BeaconState
		var isValidPayload bool

		eg.Go(func() error {
			var err error
			postState, err = s.service.validateStateTransition(ctx.Context, preState, block)
			if err != nil {
				return errors.Wrap(err, "failed to validate consensus state transition function")
			}
			return nil
		})

		eg.Go(func() error {
			var err error
			isValidPayload, err = s.service.validateExecutionOnBlock(ctx.Context, preStateVersion, preStateHeader, block)
			if err != nil {
				return errors.Wrap(err, "could not notify the engine of the new payload")
			}
			return nil
		})

		if err := eg.Wait(); err != nil {
			return nil, err
		}

		result.postState = postState
		result.isValidPayload = isValidPayload
	} else {
		// Batch mode: Run state transition and execution validation SEQUENTIALLY
		// Execute state transition without signature verification for batch optimization
		var set *bls.SignatureBatch
		set, result.postState, err = transition.ExecuteStateTransitionNoVerifyAnySig(ctx.Context, preState, block)
		if err != nil {
			return nil, invalidBlock{error: err}
		}
		result.sigSet = set

		// Validate execution payload
		postVersion, postHeader, err := getStateVersionAndPayload(result.postState)
		if err != nil {
			return nil, err
		}

		result.isValidPayload, err = s.service.notifyNewPayload(ctx.Context, postVersion, postHeader, block)
		if err != nil {
			return nil, s.service.handleInvalidExecutionError(ctx.Context, err, blockRoot, block.Block().ParentRoot())
		}

		// Validate merge transition if needed (reused logic)
		if result.isValidPayload {
			if err := s.service.validateMergeTransitionBlock(ctx.Context, preStateVersion, preStateHeader, block); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// checkDataAvailability handles DA checks for a single block (reused logic)
func (s *StateTransitionExecutionAndDAStage) checkDataAvailability(
	ctx *ProcessingContext,
	block consensusblocks.ROBlock,
	blockIndex int,
) (time.Duration, error) {
	blockRoot := ctx.BlockRoots[blockIndex]
	start := time.Now()

	if ctx.AVS == nil {
		blockCopy, err := block.Copy()
		if err != nil {
			return 0, err
		}
		return time.Since(start), s.service.isDataAvailable(ctx.Context, blockRoot, blockCopy)
	}

	return time.Since(start), ctx.AVS.IsDataAvailable(ctx.Context, s.service.CurrentSlot(), block)
}

func (s *StateTransitionExecutionAndDAStage) processSingleBlock(ctx *ProcessingContext, block consensusblocks.ROBlock) error {
	idx := ctx.CurrentBlockIndex
	
	// Get pre-state
	preState, err := s.service.getBlockPreState(ctx.Context, block.Block())
	if err != nil {
		return errors.Wrap(err, "could not get block's prestate")
	}
	ctx.SetPreStateForBlock(0, preState)
	
	// Process block using shared logic with parallel execution
	result, err := s.processBlock(ctx, block, idx, preState, ProcessParallel)
	if err != nil {
		return err
	}
	
	// Store results
	ctx.SetPostStateForBlock(0, result.postState)
	ctx.IsValidPayloads[idx] = result.isValidPayload
	ctx.Checkpoints[idx] = result.checkpoints
	
	// Check data availability using shared logic
	daWaitedTime, err := s.checkDataAvailability(ctx, block, idx)
	if err != nil {
		return err
	}
	ctx.DAWaitedTime = daWaitedTime
	
	return nil
}

func (s *StateTransitionExecutionAndDAStage) processBatchBlocks(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	// Single-loop batch processing for optimal performance
	
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
	
	// SINGLE LOOP: Process each block with state transition, execution validation, and DA checks
	for i, block := range blocks {
		// Store pre-state for other stages that need it
		ctx.CurrentPreState = preState.Copy()
		
		// Process block using shared logic with sequential execution
		result, err := s.processBlock(ctx, block, i, preState, ProcessSequential)
		if err != nil {
			return err
		}
		
		// Check data availability using shared logic
		_, err = s.checkDataAvailability(ctx, block, i)
		if err != nil {
			return err
		}
		
		// Store results
		ctx.IsValidPayloads[i] = result.isValidPayload
		ctx.SigSets[i] = result.sigSet
		ctx.Checkpoints[i] = result.checkpoints
		
		// Save boundary states at epoch transitions (like original)
		if slots.IsEpochStart(result.postState.Slot()) {
			ctx.BoundaryStates[block.Root()] = result.postState.Copy()
		}
		
		// Update streaming state for next iteration
		ctx.CurrentState = result.postState
		preState = result.postState
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
	if ctx.IsBatch() {
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



// ForkchoiceStage handles forkchoice operations
type ForkchoiceStage struct {
	service *Service
}

func (s *ForkchoiceStage) Name() string { return "forkchoice" }
func (s *ForkchoiceStage) SupportsBatch() bool { return true }

func (s *ForkchoiceStage) Execute(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	if ctx.IsBatch() {
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
	
	// Take forkchoice lock (single mode always needs it)
	s.service.cfg.ForkChoiceStore.Lock()
	defer s.service.cfg.ForkChoiceStore.Unlock()
	
	// Get the post state
	postState := ctx.GetPostStateForBlock(0)
	
	if err := s.service.savePostStateInfo(ctx.Context, blockRoot, blockCopy, postState); err != nil {
		return errors.Wrap(err, "could not save post state info")
	}
	
	// Execute post block processing (within the forkchoice lock)
	args := &postBlockProcessConfig{
		ctx:            ctx.Context,
		roblock:        block,
		postState:      postState,
		isValidPayload: ctx.IsValidPayloads[idx],
	}
	
	if err := s.service.postBlockProcess(args); err != nil {
		return errors.Wrap(err, "could not process block")
	}
	
	// IMPORTANT: Single-mode post-processing MUST happen within forkchoice lock
	// Update checkpoints (requires forkchoice lock)
	preState := ctx.GetPreStateForBlock(0)
	cp := ffgCheckpoints{
		j: ctx.Checkpoints[0][0].Epoch,
		f: ctx.Checkpoints[0][1].Epoch,
		c: ctx.Checkpoints[0][2].Epoch,
	}
	
	if err := s.service.updateCheckpoints(ctx.Context, cp, preState, postState, blockRoot); err != nil {
		return err
	}
	
	// Handle slasher if enabled
	if s.service.slasherEnabled {
		go s.service.sendBlockAttestationsToSlasher(blockCopy, preState)
	}
	
	// Prune operation pools (only if block is head)
	if err := s.service.prunePostBlockOperationPools(ctx.Context, blockCopy, blockRoot); err != nil {
		log.WithError(err).Error("Could not prune canonical objects from pool")
	}
	
	// Check save hot state DB (requires forkchoice lock)
	if err := s.service.checkSaveHotStateDB(ctx.Context); err != nil {
		return err
	}
	
	// Handle caches (requires forkchoice lock)
	if err := s.service.handleCaches(); err != nil {
		return err
	}
	
	// Report processing metrics
	s.service.reportPostBlockProcessing(blockCopy, blockRoot, ctx.ReceivedTime, ctx.DAWaitedTime)
	
	return nil
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


