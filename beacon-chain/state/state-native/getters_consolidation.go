package state_native

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// EarliestConsolidationEpoch is a non-mutating call to the beacon state which returns the value of
// the earliest consolidation epoch field. This method requires access to the RLock on the state and
// only applies in electra or later.
func (b *BeaconState) EarliestConsolidationEpoch() (primitives.Epoch, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("EarliestConsolidationEpoch", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.earliestConsolidationEpoch, nil
}

// ConsolidationBalanceToConsume is a non-mutating call to the beacon state which returns the value
// of the consolidation balance to consume field. This method requires access to the RLock on the
// state and only applies in electra or later.
func (b *BeaconState) ConsolidationBalanceToConsume() (primitives.Gwei, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("ConsolidationBalanceToConsume", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.consolidationBalanceToConsume, nil
}

// PendingConsolidations is a non-mutating call to the beacon state which returns a deep copy of the
// pending consolidations slice. This method requires access to the RLock on the state and only
// applies in electra or later.
func (b *BeaconState) PendingConsolidations() ([]*ethpb.PendingConsolidation, error) {
	if b.version < version.Electra {
		return nil, errNotSupported("PendingConsolidations", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.pendingConsolidationsVal(), nil
}

// NumPendingConsolidations is a non-mutating call to the beacon state which returns the number of
// pending consolidations in the beacon state. This method requires access to the RLock on the state
// and only applies in electra or later.
func (b *BeaconState) NumPendingConsolidations() (uint64, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("NumPendingConsolidations", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return uint64(len(b.pendingConsolidations)), nil
}

func (b *BeaconState) pendingConsolidationsVal() []*ethpb.PendingConsolidation {
	if b.pendingConsolidations == nil {
		return nil
	}

	return ethpb.CopySlice(b.pendingConsolidations)
}
