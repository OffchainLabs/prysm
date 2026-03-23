package confirmation

import (
	"context"

	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
)

// ForkchoiceReader is the read subset of forkchoice FCR uses, OnFastConfirmation takes the read lock itself around tree reads.
type ForkchoiceReader interface {
	RLock()
	RUnlock()
	CachedHeadRoot() [32]byte
	FinalizedCheckpoint() *forkchoicetypes.Checkpoint
	UnrealizedJustifiedCheckpoint() *forkchoicetypes.Checkpoint
	Slot(root [32]byte) (primitives.Slot, error)
	ParentRoot(root [32]byte) ([32]byte, error)
	IsOptimistic(root [32]byte) (bool, error)
	AncestorRoot(ctx context.Context, root [32]byte, slot primitives.Slot) ([32]byte, error)
	AncestorRoots(root [32]byte, terminalRoot [32]byte) ([][32]byte, error)
	IsAncestor(root [32]byte, ancestorRoot [32]byte) (bool, error)
	UnrealizedJustification(root [32]byte) (*forkchoicetypes.Checkpoint, error)
	VotingSource(root [32]byte) (*forkchoicetypes.Checkpoint, error)
	VoteSnapshot(buf []forkchoicetypes.VoteData) []forkchoicetypes.VoteData
	SlashedIndices() map[primitives.ValidatorIndex]bool
}

// CommitteeAccessor implements the spec's get_slot_committee, it must support slots from current_epoch - 2 onward.
type CommitteeAccessor interface {
	Committee(ctx context.Context, slot primitives.Slot) ([]primitives.ValidatorIndex, error)
}

// BalanceAccessor maps to the spec's balance_source and get_pulled_up_head_state state reads.
type BalanceAccessor interface {
	BalanceInfoByCheckpoint(ctx context.Context, root [32]byte) (balances []uint64, totalActiveBalance uint64, err error)
	PulledUpHeadState(ctx context.Context, headRoot [32]byte) (*FFGStateInfo, error)
}
