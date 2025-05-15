package state_native

import (
	customtypes "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/custom-types"
)

// RandaoMixes of block proposers on the beacon chain.
func (b *BeaconState) RandaoMixes() [][]byte {
	b.lock.RLock()
	defer b.lock.RUnlock()

	mixes := b.randaoMixesVal()
	if mixes == nil {
		return nil
	}
	return mixes.Slice()
}

func (b *BeaconState) randaoMixesVal() customtypes.RandaoMixes {
	if b.randaoMixesMultiValue == nil {
		return nil
	}
	return b.randaoMixesMultiValue.Value(b)
}

// RandaoMixAtIndex retrieves a specific block root based on an
// input index value.
func (b *BeaconState) RandaoMixAtIndex(idx uint64) ([]byte, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.randaoMixesMultiValue == nil {
		return nil, nil
	}
	r, err := b.randaoMixesMultiValue.At(b, idx)
	if err != nil {
		return nil, err
	}
	return r[:], nil
}

// RandaoMixesLength returns the length of the randao mixes slice.
func (b *BeaconState) RandaoMixesLength() int {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.randaoMixesMultiValue == nil {
		return 0
	}
	return b.randaoMixesMultiValue.Len(b)
}
