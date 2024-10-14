package state_native

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

// ExecutionPayloadHeader retrieves a copy of the execution payload header.
// It returns an error if the operation is not supported for the beacon state's version.
func (b *BeaconState) ExecutionPayloadHeader() *enginev1.ExecutionPayloadHeaderEPBS {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.executionPayloadHeaderVal()
}

// IsParentBlockFull checks if the last committed payload header was fulfilled.
// Returns true if both the beacon block and payload were present.
// Call this function on a beacon state before processing the execution payload header.
func (b *BeaconState) IsParentBlockFull() bool {
	b.lock.RLock()
	defer b.lock.RUnlock()

	headerBlockHash := bytesutil.ToBytes32(b.executionPayloadHeader.BlockHash)
	return headerBlockHash == b.latestBlockHash
}

// LatestBlockHash returns the latest block hash.
func (b *BeaconState) LatestBlockHash() []byte {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.latestBlockHash[:]
}

// LatestFullSlot returns the slot of the latest full block.
func (b *BeaconState) LatestFullSlot() primitives.Slot {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.latestFullSlot
}

// LastWithdrawalsRoot returns the latest withdrawal root.
func (b *BeaconState) LastWithdrawalsRoot() []byte {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.lastWithdrawalsRoot[:]
}
