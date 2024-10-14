package state

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

type ReadOnlyEpbsFields interface {
	IsParentBlockFull() bool
	ExecutionPayloadHeader() *enginev1.ExecutionPayloadHeaderEPBS
	LatestBlockHash() []byte
	LatestFullSlot() primitives.Slot
	LastWithdrawalsRoot() []byte
}

type WriteOnlyEpbsFields interface {
	SetExecutionPayloadHeader(val *enginev1.ExecutionPayloadHeaderEPBS)
	SetLatestBlockHash(val []byte)
	SetLatestFullSlot(val primitives.Slot)
	SetLastWithdrawalsRoot(val []byte)
}
