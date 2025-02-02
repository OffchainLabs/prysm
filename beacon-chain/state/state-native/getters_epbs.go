package state_native

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// LatestExecutionPayloadHeaderEPBS retrieves a copy of the execution payload header from epbs state.
// It returns an error if the operation is not supported for the beacon state's version.
func (b *BeaconState) LatestExecutionPayloadHeaderEPBS() (*enginev1.ExecutionPayloadHeaderEPBS, error) {
	if b.version < version.EPBS {
		return nil, errNotSupported("LatestExecutionPayloadHeaderEPBS", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.executionPayloadHeaderVal(), nil
}

// IsParentBlockFull checks if the last committed payload header was fulfilled.
// Returns true if both the beacon block and payload were present.
// Call this function on a beacon state before processing the execution payload header.
// It returns an error if the operation is not supported for the beacon state's version.
func (b *BeaconState) IsParentBlockFull() (bool, error) {
	if b.version < version.EPBS {
		return false, errNotSupported("IsParentBlockFull", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	headerBlockHash := bytesutil.ToBytes32(b.latestExecutionPayloadHeaderEPBS.BlockHash)
	return headerBlockHash == b.latestBlockHash, nil
}

// LatestBlockHash returns the latest block hash.
// It returns an error if the operation is not supported for the beacon state's version.
func (b *BeaconState) LatestBlockHash() ([]byte, error) {
	if b.version < version.EPBS {
		return nil, errNotSupported("LatestBlockHash", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.latestBlockHash[:], nil
}

// LatestFullSlot returns the slot of the latest full block.
// It returns an error if the operation is not supported for the beacon state's version.
func (b *BeaconState) LatestFullSlot() (primitives.Slot, error) {
	if b.version < version.EPBS {
		return 0, errNotSupported("LatestFullSlot", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.latestFullSlot, nil
}

// LastWithdrawalsRoot returns the latest withdrawal root.
// It returns an error if the operation is not supported for the beacon state's version.
func (b *BeaconState) LastWithdrawalsRoot() ([]byte, error) {
	if b.version < version.EPBS {
		return nil, errNotSupported("LastWithdrawalsRoot", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.lastWithdrawalsRoot[:], nil
}
