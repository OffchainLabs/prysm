package state_native

import (
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/time/slots"
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

	slot, err := slots.EpochStart(params.BeaconConfig().EPBSForkEpoch)
	if err != nil {
		return false, err
	}
	if b.slot == slot {
		return true, nil
	}

	headerBlockHash := bytesutil.ToBytes32(b.latestExecutionPayloadHeaderEPBS.BlockHash)
	// if the header block hash is empty, the parent is from the previous fork
	if headerBlockHash == [32]byte{} {
		return true, nil
	}
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

	return b.latestWithdrawalsRoot[:], nil
}
