package forkchoice

import (
	"time"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
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
