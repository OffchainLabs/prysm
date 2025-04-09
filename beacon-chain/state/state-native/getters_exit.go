package state_native

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// ExitBalanceToConsume is used for returning the ExitBalanceToConsume as part of eip 7251
func (b *BeaconState) ExitBalanceToConsume() (primitives.Gwei, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("ExitBalanceToConsume", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.exitBalanceToConsume, nil
}

// EarliestExitEpoch is used for returning the EarliestExitEpoch as part of eip 7251
func (b *BeaconState) EarliestExitEpoch() (primitives.Epoch, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("EarliestExitEpoch", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.earliestExitEpoch, nil
}
