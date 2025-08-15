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
	Context   context.Context
	Mode      ProcessingMode
	AVS       das.AvailabilityStore
	BatchSize int

	// Single state tracking (reused in batch mode)
	CurrentState    state.BeaconState
	CurrentPreState state.BeaconState
	
	// Boundary states (only epoch boundaries saved)
	BoundaryStates map[[32]byte]state.BeaconState
	
	// For single mode only (size 1)
	States    []state.BeaconState
	PreStates []state.BeaconState
	
	// Lightweight data - OK to keep all
	Checkpoints     [][]*ethpb.Checkpoint
	SigSets         []*bls.SignatureBatch
	IsValidPayloads []bool
	BlockRoots      [][32]byte

	// Timing info
	ReceivedTime time.Time
	DAWaitedTime time.Duration

	// Current block processing info
	CurrentBlockIndex int
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
func (bp *BlockProcessor) Process(pc *ProcessingContext, blocks []consensusblocks.ROBlock) error {
	// Initialize based on mode
	if pc.Mode == ModeSingle {
		// Single mode: allocate single-element arrays
		pc.States = make([]state.BeaconState, 1)
		pc.PreStates = make([]state.BeaconState, 1)
	} else {
		// Batch mode: use streaming approach with boundary states
		pc.BoundaryStates = make(map[[32]byte]state.BeaconState)
	}
	
	// Lightweight arrays - always allocate full size
	pc.Checkpoints = make([][]*ethpb.Checkpoint, len(blocks))
	pc.SigSets = make([]*bls.SignatureBatch, len(blocks))
	pc.IsValidPayloads = make([]bool, len(blocks))
	pc.BlockRoots = make([][32]byte, len(blocks))

	for i, block := range blocks {
		pc.BlockRoots[i] = block.Root()
	}

	// Execute each stage
	for _, stage := range bp.stages {
		if pc.Mode == ModeBatch && !stage.SupportsBatch() && len(blocks) > 1 {
			// Process individually for stages that don't support batch
			for i, block := range blocks {
				pc.CurrentBlockIndex = i
				if err := stage.Execute(pc, []consensusblocks.ROBlock{block}); err != nil {
					return errors.Wrapf(err, "stage %s failed for block %d", stage.Name(), i)
				}
			}
		} else {
			if err := stage.Execute(pc, blocks); err != nil {
				return errors.Wrapf(err, "stage %s failed", stage.Name())
			}
		}
	}

	return nil
}
