package blockchain

import (
	"context"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// ProcessingMode defines the mode of block processing
type ProcessingMode int

const (
	// ModeSingle processes blocks individually with full validation
	ModeSingle ProcessingMode = iota
	// ModeBatch processes multiple blocks with optimizations for throughput
	ModeBatch
)

// ProcessingContext contains all the context needed for block processing
type ProcessingContext struct {
	Context     context.Context
	Mode        ProcessingMode
	AVS         das.AvailabilityStore
	BatchSize   int
	
	// Results tracking
	States      []state.BeaconState
	Checkpoints [][]*ethpb.Checkpoint
	SigSets     []*bls.SignatureBatch
	IsValidPayloads []bool
	PreStates   []state.BeaconState
	
	// Timing info
	ReceivedTime time.Time
	DAWaitedTime time.Duration
	
	// Current block processing info
	CurrentBlockIndex int
	BlockRoots       [][32]byte
}

// ProcessingStage represents a stage in the block processing pipeline
type ProcessingStage interface {
	Name() string
	Execute(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error
	SupportsBatch() bool
}

// BlockProcessor handles both single and batch block processing using a pipeline
type BlockProcessor struct {
	service *Service
	stages  []ProcessingStage
}

// NewBlockProcessor creates a new unified block processor
func NewBlockProcessor(service *Service) *BlockProcessor {
	stages := []ProcessingStage{
		&ValidationStage{service: service},
		&StateTransitionStage{service: service},
		&SignatureVerificationStage{service: service},
		&ExecutionValidationStage{service: service},
		&DataAvailabilityStage{service: service},
		&ForkchoiceStage{service: service},
		&PostProcessingStage{service: service},
	}
	
	return &BlockProcessor{
		service: service,
		stages:  stages,
	}
}

// Process executes the block processing pipeline
func (bp *BlockProcessor) Process(ctx *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	// Initialize context state
	ctx.States = make([]state.BeaconState, len(blocks))
	ctx.Checkpoints = make([][]*ethpb.Checkpoint, len(blocks))
	ctx.SigSets = make([]*bls.SignatureBatch, len(blocks))
	ctx.IsValidPayloads = make([]bool, len(blocks))
	ctx.PreStates = make([]state.BeaconState, len(blocks))
	ctx.BlockRoots = make([][32]byte, len(blocks))
	
	for i, block := range blocks {
		ctx.BlockRoots[i] = block.Root()
	}
	
	// Execute each stage
	for _, stage := range bp.stages {
		if ctx.Mode == ModeBatch && !stage.SupportsBatch() && len(blocks) > 1 {
			// Process individually for stages that don't support batch
			for i, block := range blocks {
				ctx.CurrentBlockIndex = i
				if err := stage.Execute(ctx, []consensusblocks.ROBlock{block}); err != nil {
					return errors.Wrapf(err, "stage %s failed for block %d", stage.Name(), i)
				}
			}
		} else {
			if err := stage.Execute(ctx, blocks); err != nil {
				return errors.Wrapf(err, "stage %s failed", stage.Name())
			}
		}
	}
	
	return nil
}