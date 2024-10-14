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

// UpdatePreviousInclusionListData updates the data of previous inclusion list with latest values.
func (b *BeaconState) UpdatePreviousInclusionListData() {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.previousInclusionListProposer = b.latestInclusionListProposer
	b.previousInclusionListSlot = b.latestInclusionListSlot
	b.markFieldAsDirty(types.PreviousInclusionListProposer)
	b.markFieldAsDirty(types.PreviousInclusionListSlot)
}

// SetLatestInclusionListProposer sets the latest inclusion list proposer for the beacon state.
func (b *BeaconState) SetLatestInclusionListProposer(i primitives.ValidatorIndex) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.latestInclusionListProposer = i
	b.markFieldAsDirty(types.LatestInclusionListProposer)
}

// SetLatestInclusionListSlot sets the latest inclusion list slot for the beacon state.
func (b *BeaconState) SetLatestInclusionListSlot(s primitives.Slot) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.latestInclusionListSlot = s
	b.markFieldAsDirty(types.LatestInclusionListSlot)
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
