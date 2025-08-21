package state_native

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/types"
	"github.com/pkg/errors"
)

// SetRandaoMixes for the beacon state. Updates the entire
// randao mixes to a new value by overwriting the previous one.
func (b *BeaconState) SetRandaoMixes(val [][]byte) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.randaoMixesMultiValue != nil {
		b.randaoMixesMultiValue.Detach(b)
	}
	b.randaoMixesMultiValue = NewMultiValueRandaoMixes(val)

	b.markFieldAsDirty(types.RandaoMixes)
	b.rebuildTrie[types.RandaoMixes] = true
	return nil
}

// UpdateRandaoMixesAtIndex for the beacon state. Updates the randao mixes
// at a specific index to a new value.
func (b *BeaconState) UpdateRandaoMixesAtIndex(idx uint64, val [32]byte) error {
	if err := b.randaoMixesMultiValue.UpdateAt(b, idx, val); err != nil {
		return errors.Wrap(err, "could not update randao mixes")
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.markFieldAsDirty(types.RandaoMixes)
	b.addDirtyIndices(types.RandaoMixes, []uint64{idx})
	return nil
}
