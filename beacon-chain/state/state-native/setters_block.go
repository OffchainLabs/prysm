package state_native

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/types"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
)

// SetLatestBlockHeader in the beacon state.
func (b *BeaconState) SetLatestBlockHeader(val *ethpb.BeaconBlockHeader) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.latestBlockHeader = val.Copy()
	b.markFieldAsDirty(types.LatestBlockHeader)
	return nil
}

// SetBlockRoots for the beacon state. Updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetBlockRoots(val [][]byte) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.blockRootsMultiValue != nil {
		b.blockRootsMultiValue.Detach(b)
	}
	b.blockRootsMultiValue = NewMultiValueBlockRoots(val)

	b.markFieldAsDirty(types.BlockRoots)
	b.rebuildTrie[types.BlockRoots] = true
	return nil
}

// UpdateBlockRootAtIndex for the beacon state. Updates the block root
// at a specific index to a new value.
func (b *BeaconState) UpdateBlockRootAtIndex(idx uint64, blockRoot [32]byte) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if err := b.blockRootsMultiValue.UpdateAt(b, idx, blockRoot); err != nil {
		return errors.Wrap(err, "could not update block roots")
	}

	b.markFieldAsDirty(types.BlockRoots)
	b.addDirtyIndices(types.BlockRoots, []uint64{idx})
	return nil
}
