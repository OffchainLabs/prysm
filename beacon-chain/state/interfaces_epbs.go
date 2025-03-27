package state

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
)

type ReadOnlyEpbsFields interface {
	IsParentBlockFull() (bool, error)
	LatestExecutionPayloadHeaderEPBS() (*enginev1.ExecutionPayloadHeaderEPBS, error)
	LatestBlockHash() ([]byte, error)
	LatestFullSlot() (primitives.Slot, error)
	LastWithdrawalsRoot() ([]byte, error)
}

type WriteOnlyEpbsFields interface {
	SetLatestExecutionPayloadHeaderEPBS(val *enginev1.ExecutionPayloadHeaderEPBS) error
	SetLatestBlockHash(val []byte) error
	SetLatestFullSlot(val primitives.Slot) error
	SetLastWithdrawalsRoot(val []byte) error
}
