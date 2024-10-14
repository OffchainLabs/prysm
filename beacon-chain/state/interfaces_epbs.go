package state

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

type ReadOnlyEpbsFields interface {
	PreviousInclusionListSlot() primitives.Slot
	PreviousInclusionListProposer() primitives.ValidatorIndex
	LatestInclusionListSlot() primitives.Slot
	LatestInclusionListProposer() primitives.ValidatorIndex
	IsParentBlockFull() bool
	ExecutionPayloadHeader() *enginev1.ExecutionPayloadHeaderEPBS
	LatestBlockHash() []byte
	LatestFullSlot() primitives.Slot
	LastWithdrawalsRoot() []byte
}

type WriteOnlyEpbsFields interface {
	SetExecutionPayloadHeader(val *enginev1.ExecutionPayloadHeaderEPBS)
	UpdatePreviousInclusionListData()
	SetLatestInclusionListSlot(val primitives.Slot)
	SetLatestInclusionListProposer(val primitives.ValidatorIndex)
	SetLatestBlockHash(val []byte)
	SetLatestFullSlot(val primitives.Slot)
	SetLastWithdrawalsRoot(val []byte)
}
