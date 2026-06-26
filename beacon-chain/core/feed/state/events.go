// Package state contains types for state operation-specific events fired
// during the runtime of a beacon node such state initialization, state updates,
// and chain start.
package state

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

const (
	// BlockProcessed is sent after a block has been processed and updated the state database.
	BlockProcessed = iota + 1
	// ChainStarted is sent when enough validators are active to start proposing blocks.
	ChainStarted
	// deprecated: Initialized is sent when the internal beacon node's state is ready to be accessed.
	_
	// deprecated: Synced is sent when the beacon node has completed syncing and is ready to participate in the network.
	_
	// Reorg is an event sent when the new head is not a descendant of the previous head.
	Reorg
	// FinalizedCheckpoint event.
	FinalizedCheckpoint
	// NewHead of the chain event.
	NewHead
	// NewHeadV2 of the chain event, carrying the versioned, Gloas-aware head_v2 payload.
	NewHeadV2
	// MissedSlot is sent when we need to notify users that a slot was missed.
	MissedSlot
	// LightClientFinalityUpdate event
	LightClientFinalityUpdate
	// LightClientOptimisticUpdate event
	LightClientOptimisticUpdate
	// PayloadAttributes events are fired upon a missed slot or new head.
	PayloadAttributes
	// ExecutionPayloadAvailable is sent when a new execution payload is available (without EL validation results).
	ExecutionPayloadAvailable
	// ExecutionPayloadProcessed is sent after a payload envelope has been processed.
	ExecutionPayloadProcessed
)

// BlockProcessedData is the data sent with BlockProcessed events.
type BlockProcessedData struct {
	// Slot is the slot of the processed block.
	Slot primitives.Slot
	// BlockRoot of the processed block.
	BlockRoot [32]byte
	// SignedBlock is the physical processed block.
	SignedBlock interfaces.ReadOnlySignedBeaconBlock
	// CurrDependentRoot is the current dependent root
	CurrDependentRoot [32]byte
	// PrevDependentRoot is the previous dependent root
	PrevDependentRoot [32]byte
	// Verified is true if the block's BLS contents have been verified.
	Verified bool
	// Optimistic is true if the block is optimistic.
	Optimistic bool
}

// ChainStartedData is the data sent with ChainStarted events.
type ChainStartedData struct {
	// StartTime is the time at which the chain started.
	StartTime time.Time
}

// SyncedData is the data sent with Synced events.
type SyncedData struct {
	// StartTime is the time at which the chain started.
	StartTime time.Time
}

// InitializedData is the data sent with Initialized events.
type InitializedData struct {
	// StartTime is the time at which the chain started.
	StartTime time.Time
	// GenesisValidatorsRoot represents state.validators.HashTreeRoot().
	GenesisValidatorsRoot []byte
}

// ExecutionPayloadAvailableData is the data sent with ExecutionPayloadAvailable events.
type ExecutionPayloadAvailableData struct {
	Slot      primitives.Slot
	BlockRoot [32]byte
}

// ExecutionPayloadProcessedData is the data sent with ExecutionPayloadProcessed events.
type ExecutionPayloadProcessedData struct {
	Slot         primitives.Slot
	BuilderIndex primitives.BuilderIndex
	BlockHash    [32]byte
	BlockRoot    [32]byte
	// Optimistic is true if the imported payload has not been fully validated by the execution layer.
	Optimistic bool
}

type PayloadStatus int

const (
	PayloadStatusEmpty = iota + 1
	PayloadStatusFull
)

func (ps PayloadStatus) String() string {
	switch ps {
	case PayloadStatusEmpty:
		return "empty"
	case PayloadStatusFull:
		return "full"
	default:
		return "unknown"
	}
}

// HeadData is the data sent with NewHead events.
type HeadData struct {
	Slot                      primitives.Slot
	Block                     [32]byte
	State                     [32]byte
	EpochTransition           bool
	PreviousDutyDependentRoot [32]byte
	CurrentDutyDependentRoot  [32]byte
	ExecutionOptimistic       bool
}

// HeadV2Data is the data sent with NewHeadV2 events.
type HeadV2Data struct {
	Slot                      primitives.Slot
	Block                     [32]byte
	State                     [32]byte
	EpochTransition           bool
	ExecutionOptimistic       bool
	CurrentEpochDependentRoot [32]byte
	NextEpochDependentRoot    [32]byte
	PayloadStatus             PayloadStatus
	Version                   int
}
