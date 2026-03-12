package forkchoice

import (
	"context"
	"time"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	consensus_blocks "github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	forkchoice2 "github.com/OffchainLabs/prysm/v7/consensus-types/forkchoice"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// BalancesByRooter is a handler to obtain the effective balances of the state
// with the given block root
type BalancesByRooter func(context.Context, [32]byte) ([]uint64, error)

// ForkChoicer represents the full fork choice interface composed of all the sub-interfaces.
type ForkChoicer interface {
	RLocker // separate interface isolates  read locking for ROForkChoice.
	Lock()
	Unlock()
	HeadRetriever        // to compute head.
	BlockProcessor       // to track new block for fork choice.
	PayloadProcessor     // to track new payloads for fork choice.
	AttestationProcessor // to track new attestation for fork choice.
	Getter               // to retrieve fork choice information.
	Setter               // to set fork choice information.
}

// RLocker represents forkchoice's internal RWMutex read-only lock/unlock methods.
type RLocker interface {
	RLock()
	RUnlock()
}

// HeadRetriever retrieves head root and optimistic info of the current chain.
type HeadRetriever interface {
	Head(context.Context) ([32]byte, error)
	FullHead(context.Context) ([32]byte, [32]byte, bool, error)
	GetProposerHead() [32]byte
	CachedHeadRoot() [32]byte
}

// BlockProcessor processes the block that's used for accounting fork choice.
type BlockProcessor interface {
	InsertNode(context.Context, state.BeaconState, consensus_blocks.ROBlock) error
	InsertChain(context.Context, []*forkchoicetypes.BlockAndCheckpoints) error
}

// PayloadProcessor processes a payload envelope
type PayloadProcessor interface {
	InsertPayload(interfaces.ROExecutionPayloadEnvelope) error
}

// AttestationProcessor processes the attestation that's used for accounting fork choice.
type AttestationProcessor interface {
	ProcessAttestation(context.Context, []uint64, [32]byte, primitives.Slot, bool)
}

// Getter returns fork choice related information.
type Getter interface {
	FastGetter
	AncestorRoot(ctx context.Context, root [32]byte, slot primitives.Slot) ([32]byte, error)
	CommonAncestor(ctx context.Context, root1 [32]byte, root2 [32]byte) ([32]byte, primitives.Slot, error)
	ForkChoiceDump(context.Context) (*forkchoice2.Dump, error)
	Tips() ([][32]byte, []primitives.Slot)
}

type FastGetter interface {
	FinalizedCheckpoint() *forkchoicetypes.Checkpoint
	FinalizedPayloadBlockHash() [32]byte
	HasFullNode([32]byte) bool
	HasNode([32]byte) bool
	HighestReceivedBlockSlot() primitives.Slot
	HighestReceivedBlockRoot() [32]byte
	IsCanonical(root [32]byte) bool
	IsOptimistic(root [32]byte) (bool, error)
	IsViableForCheckpoint(*forkchoicetypes.Checkpoint) (bool, error)
	JustifiedCheckpoint() *forkchoicetypes.Checkpoint
	JustifiedPayloadBlockHash() [32]byte
	NodeCount() int
	PreviousJustifiedCheckpoint() *forkchoicetypes.Checkpoint
	ProposerBoost() [fieldparams.RootLength]byte
	ReceivedBlocksLastEpoch() (uint64, error)
	ShouldOverrideFCU() bool
	Slot([32]byte) (primitives.Slot, error)
	DependentRoot(primitives.Epoch) ([32]byte, error)
	DependentRootForEpoch([32]byte, primitives.Epoch) ([32]byte, error)
	TargetRootForEpoch([32]byte, primitives.Epoch) ([32]byte, error)
	UnrealizedJustifiedPayloadBlockHash() [32]byte
	Weight(root [32]byte) (uint64, error)
	ConsensusNodeWeight(root [32]byte) (uint64, error)
	PayloadWeights(root [32]byte) (emptyWeight, fullWeight uint64, err error)
	ParentRoot(root [32]byte) ([32]byte, error)
	BlockHash(root [32]byte) ([32]byte, error)
	CanonicalNodeAtSlot(slot primitives.Slot) ([32]byte, bool)
}

// Setter allows to set forkchoice information
type Setter interface {
	SetOptimisticToValid(context.Context, [fieldparams.RootLength]byte) error
	SetOptimisticToInvalid(context.Context, [32]byte, [32]byte, [32]byte, [32]byte) ([][32]byte, error)
	UpdateJustifiedCheckpoint(context.Context, *forkchoicetypes.Checkpoint) error
	UpdateFinalizedCheckpoint(*forkchoicetypes.Checkpoint) error
	SetGenesisTime(time.Time)
	SetOriginRoot([32]byte)
	NewSlot(context.Context, primitives.Slot) error
	SetBalancesByRooter(BalancesByRooter)
	InsertSlashedIndex(context.Context, primitives.ValidatorIndex)
	SetPTCVote(root [32]byte, ptcIdx uint64, payloadPresent, blobDataAvailable bool)
}
