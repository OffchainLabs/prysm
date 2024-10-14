package state_native

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
)

// SetExecutionPayloadHeader sets the execution payload header for the beacon state.
func (b *BeaconState) SetExecutionPayloadHeader(h *enginev1.ExecutionPayloadHeaderEPBS) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.executionPayloadHeader = h
	b.markFieldAsDirty(types.ExecutionPayloadHeader)
}

// SetLatestBlockHash sets the latest block hash for the beacon state.
func (b *BeaconState) SetLatestBlockHash(h []byte) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.latestBlockHash = bytesutil.ToBytes32(h)
	b.markFieldAsDirty(types.LatestBlockHash)
}

// SetLatestFullSlot sets the latest full slot for the beacon state.
func (b *BeaconState) SetLatestFullSlot(s primitives.Slot) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.latestFullSlot = s
	b.markFieldAsDirty(types.LatestFullSlot)
}

// SetLastWithdrawalsRoot sets the latest withdrawals root for the beacon state.
func (b *BeaconState) SetLastWithdrawalsRoot(r []byte) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.lastWithdrawalsRoot = bytesutil.ToBytes32(r)
	b.markFieldAsDirty(types.LastWithdrawalsRoot)
}
