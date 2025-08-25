package state_native

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/types"
	"github.com/pkg/errors"
)

// SetStateRoots for the beacon state. Updates the state roots
// to a new value by overwriting the previous value.
func (b *BeaconState) SetStateRoots(val [][]byte) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.stateRootsMultiValue != nil {
		b.stateRootsMultiValue.Detach(b)
	}
	b.stateRootsMultiValue = NewMultiValueStateRoots(val)

	b.markFieldAsDirty(types.StateRoots)
	b.rebuildTrie[types.StateRoots] = true
	return nil
}

// UpdateStateRootAtIndex for the beacon state. Updates the state root
// at a specific index to a new value.
func (b *BeaconState) UpdateStateRootAtIndex(idx uint64, stateRoot [32]byte) error {
	if err := b.stateRootsMultiValue.UpdateAt(b, idx, stateRoot); err != nil {
		return errors.Wrap(err, "could not update state roots")
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.markFieldAsDirty(types.StateRoots)
	b.addDirtyIndices(types.StateRoots, []uint64{idx})
	return nil
}
