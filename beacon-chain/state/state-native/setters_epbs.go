package state_native

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// SetLatestExecutionPayloadHeaderEPBS sets the latest execution payload header for the epbs beacon state.
func (b *BeaconState) SetLatestExecutionPayloadHeaderEPBS(h *enginev1.ExecutionPayloadHeaderEPBS) error {
	if b.version < version.EPBS {
		return errNotSupported("SetLatestExecutionPayloadHeaderEPBS", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.latestExecutionPayloadHeaderEPBS = h
	b.markFieldAsDirty(types.ExecutionPayloadHeader)

	return nil
}

// SetLatestBlockHash sets the latest block hash for the beacon state.
func (b *BeaconState) SetLatestBlockHash(h []byte) error {
	if b.version < version.EPBS {
		return errNotSupported("SetLatestBlockHash", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.latestBlockHash = bytesutil.ToBytes32(h)
	b.markFieldAsDirty(types.LatestBlockHash)

	return nil
}

// SetLatestFullSlot sets the latest full slot for the beacon state.
func (b *BeaconState) SetLatestFullSlot(s primitives.Slot) error {
	if b.version < version.EPBS {
		return errNotSupported("SetLatestFullSlot", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.latestFullSlot = s
	b.markFieldAsDirty(types.LatestFullSlot)

	return nil
}

// SetLastWithdrawalsRoot sets the latest withdrawals root for the beacon state.
func (b *BeaconState) SetLastWithdrawalsRoot(r []byte) error {
	if b.version < version.EPBS {
		return errNotSupported("SetLastWithdrawalsRoot", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.lastWithdrawalsRoot = bytesutil.ToBytes32(r)
	b.markFieldAsDirty(types.LastWithdrawalsRoot)

	return nil
}
