package state_native

import (
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// DepositRequestsStartIndex is used for returning the deposit requests start index which is used for eip6110
func (b *BeaconState) DepositRequestsStartIndex() (uint64, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("DepositRequestsStartIndex", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.depositRequestsStartIndex, nil
}
