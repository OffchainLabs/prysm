package forkchoice

import (
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type NodeValidity uint8

const (
	Valid NodeValidity = iota
	Invalid
	Optimistic
)

func (v NodeValidity) String() string {
	switch v {
	case Valid:
		return "valid"
	case Invalid:
		return "invalid"
	case Optimistic:
		return "optimistic"
	default:
		return "unknown"
	}
}

type PayloadStatus uint8

const (
	PayloadStatusEmpty   PayloadStatus = 0
	PayloadStatusFull    PayloadStatus = 1
	PayloadStatusPending PayloadStatus = 2
)

func (p PayloadStatus) String() string {
	switch p {
	case PayloadStatusEmpty:
		return "empty"
	case PayloadStatusFull:
		return "full"
	case PayloadStatusPending:
		return "pending"
	default:
		return "unknown"
	}
}

type DumpV2 struct {
	JustifiedCheckpoint           *eth.Checkpoint
	FinalizedCheckpoint           *eth.Checkpoint
	UnrealizedJustifiedCheckpoint *eth.Checkpoint
	UnrealizedFinalizedCheckpoint *eth.Checkpoint
	ProposerBoostRoot             []byte
	PreviousProposerBoostRoot     []byte
	HeadRoot                      []byte
	ForkChoiceNodes               []*NodeV2
}

type NodeV2 struct {
	BlockRoot                       []byte
	ParentRoot                      []byte
	ExecutionBlockHash              []byte
	Target                          []byte
	Timestamp                       time.Time
	Slot                            primitives.Slot
	Weight                          uint64
	Balance                         uint64
	JustifiedEpoch                  primitives.Epoch
	FinalizedEpoch                  primitives.Epoch
	UnrealizedJustifiedEpoch        primitives.Epoch
	UnrealizedFinalizedEpoch        primitives.Epoch
	PayloadAttesterCount            uint64
	PayloadAvailabilityYesCount     uint64
	PayloadDataAvailabilityYesCount uint64
	GasLimit                        uint64
	PayloadStatus                   PayloadStatus
	Validity                        NodeValidity
	ExecutionOptimistic             bool
}

type Dump struct {
	JustifiedCheckpoint           *eth.Checkpoint
	FinalizedCheckpoint           *eth.Checkpoint
	UnrealizedJustifiedCheckpoint *eth.Checkpoint
	UnrealizedFinalizedCheckpoint *eth.Checkpoint
	ProposerBoostRoot             []byte
	PreviousProposerBoostRoot     []byte
	HeadRoot                      []byte
	ForkChoiceNodes               []*Node
}

type Node struct {
	Validity                 NodeValidity
	ExecutionOptimistic      bool
	Slot                     primitives.Slot
	JustifiedEpoch           primitives.Epoch
	FinalizedEpoch           primitives.Epoch
	UnrealizedJustifiedEpoch primitives.Epoch
	UnrealizedFinalizedEpoch primitives.Epoch
	Balance                  uint64
	Weight                   uint64
	Timestamp                time.Time
	BlockRoot                []byte
	ParentRoot               []byte
	ExecutionBlockHash       []byte
	Target                   []byte
}
