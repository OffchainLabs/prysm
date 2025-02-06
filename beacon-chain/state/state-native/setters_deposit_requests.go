package state_native

import (
	"github.com/prysmaticlabs/prysm/v6/beacon-chain/state/state-native/types"
	"github.com/prysmaticlabs/prysm/v6/runtime/version"
)

// SetDepositRequestsStartIndex for the beacon state. Updates the DepositRequestsStartIndex
func (b *BeaconState) SetDepositRequestsStartIndex(index uint64) error {
	if b.version < version.Electra {
		return errNotSupported("SetDepositRequestsStartIndex", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.depositRequestsStartIndex = index
	b.markFieldAsDirty(types.DepositRequestsStartIndex)
	b.rebuildTrie[types.DepositRequestsStartIndex] = true
	return nil
}
