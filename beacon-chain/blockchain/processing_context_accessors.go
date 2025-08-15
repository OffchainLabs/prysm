package blockchain

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
)

// ProcessingStrategy encapsulates the differences between single and batch processing
type ProcessingStrategy interface {
	// Processing mode queries
	IsSingle() bool
	IsBatch() bool
	NeedsForkchoiceLock() bool
	
	// State accessors
	GetPreState(ctx *ProcessingContext, blockIndex int) state.BeaconState
	GetPostState(ctx *ProcessingContext, blockIndex int) state.BeaconState
	SetPreState(ctx *ProcessingContext, blockIndex int, state state.BeaconState)
	SetPostState(ctx *ProcessingContext, blockIndex int, state state.BeaconState)
	
	// Current state management (for streaming)
	GetCurrentPreState(ctx *ProcessingContext) state.BeaconState
	GetCurrentPostState(ctx *ProcessingContext) state.BeaconState
	SetCurrentPreState(ctx *ProcessingContext, state state.BeaconState)
	SetCurrentPostState(ctx *ProcessingContext, state state.BeaconState)
	
	// Execution strategy
	ShouldProcessInParallel() bool
	ShouldStreamStates() bool
}

// SingleBlockStrategy handles single block processing
type SingleBlockStrategy struct{}

func (s *SingleBlockStrategy) IsSingle() bool            { return true }
func (s *SingleBlockStrategy) IsBatch() bool             { return false }
func (s *SingleBlockStrategy) NeedsForkchoiceLock() bool { return true }
func (s *SingleBlockStrategy) ShouldProcessInParallel() bool  { return true }
func (s *SingleBlockStrategy) ShouldStreamStates() bool       { return false }

func (s *SingleBlockStrategy) GetPreState(ctx *ProcessingContext, blockIndex int) state.BeaconState {
	return ctx.PreStates[0]
}

func (s *SingleBlockStrategy) GetPostState(ctx *ProcessingContext, blockIndex int) state.BeaconState {
	return ctx.States[0]
}

func (s *SingleBlockStrategy) SetPreState(ctx *ProcessingContext, blockIndex int, state state.BeaconState) {
	ctx.PreStates[0] = state
}

func (s *SingleBlockStrategy) SetPostState(ctx *ProcessingContext, blockIndex int, state state.BeaconState) {
	ctx.States[0] = state
}

func (s *SingleBlockStrategy) GetCurrentPreState(ctx *ProcessingContext) state.BeaconState {
	return ctx.PreStates[0]
}

func (s *SingleBlockStrategy) GetCurrentPostState(ctx *ProcessingContext) state.BeaconState {
	return ctx.States[0]
}

func (s *SingleBlockStrategy) SetCurrentPreState(ctx *ProcessingContext, state state.BeaconState) {
	ctx.PreStates[0] = state
	ctx.CurrentPreState = state
}

func (s *SingleBlockStrategy) SetCurrentPostState(ctx *ProcessingContext, state state.BeaconState) {
	ctx.States[0] = state
	ctx.CurrentState = state
}

// BatchBlockStrategy handles batch processing (including single block in batch mode)
type BatchBlockStrategy struct {
	blockCount int
}

func NewBatchBlockStrategy(blockCount int) *BatchBlockStrategy {
	return &BatchBlockStrategy{blockCount: blockCount}
}

func (b *BatchBlockStrategy) IsSingle() bool            { return false }
func (b *BatchBlockStrategy) IsBatch() bool             { return true }
func (b *BatchBlockStrategy) NeedsForkchoiceLock() bool { return false } // Lock held by ReceiveBlockBatch
func (b *BatchBlockStrategy) ShouldProcessInParallel() bool  { return false } // Batch processing is sequential
func (b *BatchBlockStrategy) ShouldStreamStates() bool       { return true }

func (b *BatchBlockStrategy) GetPreState(ctx *ProcessingContext, blockIndex int) state.BeaconState {
	return ctx.CurrentPreState
}

func (b *BatchBlockStrategy) GetPostState(ctx *ProcessingContext, blockIndex int) state.BeaconState {
	return ctx.CurrentState
}

func (b *BatchBlockStrategy) SetPreState(ctx *ProcessingContext, blockIndex int, state state.BeaconState) {
	ctx.CurrentPreState = state
}

func (b *BatchBlockStrategy) SetPostState(ctx *ProcessingContext, blockIndex int, state state.BeaconState) {
	ctx.CurrentState = state
}

func (b *BatchBlockStrategy) GetCurrentPreState(ctx *ProcessingContext) state.BeaconState {
	return ctx.CurrentPreState
}

func (b *BatchBlockStrategy) GetCurrentPostState(ctx *ProcessingContext) state.BeaconState {
	return ctx.CurrentState
}

func (b *BatchBlockStrategy) SetCurrentPreState(ctx *ProcessingContext, state state.BeaconState) {
	ctx.CurrentPreState = state
}

func (b *BatchBlockStrategy) SetCurrentPostState(ctx *ProcessingContext, state state.BeaconState) {
	ctx.CurrentState = state
}

// Strategy returns the appropriate processing strategy for the context
func (ctx *ProcessingContext) Strategy() ProcessingStrategy {
	if ctx.Mode == ModeSingle {
		return &SingleBlockStrategy{}
	}
	return NewBatchBlockStrategy(ctx.BatchSize)
}

// Convenience methods on ProcessingContext that delegate to strategy
func (ctx *ProcessingContext) IsSingle() bool {
	return ctx.Strategy().IsSingle()
}

func (ctx *ProcessingContext) IsBatch() bool {
	return ctx.Strategy().IsBatch()
}

func (ctx *ProcessingContext) NeedsForkchoiceLock() bool {
	return ctx.Strategy().NeedsForkchoiceLock()
}

func (ctx *ProcessingContext) ShouldProcessInParallel() bool {
	return ctx.Strategy().ShouldProcessInParallel()
}

func (ctx *ProcessingContext) GetPreStateForBlock(blockIndex int) state.BeaconState {
	return ctx.Strategy().GetPreState(ctx, blockIndex)
}

func (ctx *ProcessingContext) GetPostStateForBlock(blockIndex int) state.BeaconState {
	return ctx.Strategy().GetPostState(ctx, blockIndex)
}

func (ctx *ProcessingContext) SetPreStateForBlock(blockIndex int, state state.BeaconState) {
	ctx.Strategy().SetPreState(ctx, blockIndex, state)
}

func (ctx *ProcessingContext) SetPostStateForBlock(blockIndex int, state state.BeaconState) {
	ctx.Strategy().SetPostState(ctx, blockIndex, state)
}