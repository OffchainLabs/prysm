package state_native

import (
	"errors"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stateutil"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// AppendPendingConsolidation is a mutating call to the beacon state which appends the provided
// pending consolidation to the end of the slice on the state. This method requires access to the
// Lock on the state and only applies in electra or later.
func (b *BeaconState) AppendPendingConsolidation(val *ethpb.PendingConsolidation) error {
	if b.version < version.Electra {
		return errNotSupported("AppendPendingConsolidation", b.version)
	}
	if val == nil {
		return errors.New("cannot append nil pending consolidation")
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.PendingConsolidations].MinusRef()
	b.sharedFieldReferences[types.PendingConsolidations] = stateutil.NewRef(1)

	b.pendingConsolidations = append(b.pendingConsolidations, val)

	b.markFieldAsDirty(types.PendingConsolidations)
	b.rebuildTrie[types.PendingConsolidations] = true
	return nil
}

// SetPendingConsolidations is a mutating call to the beacon state which replaces the slice on the
// state with the given value. This method requires access to the Lock on the state and only applies
// in electra or later.
func (b *BeaconState) SetPendingConsolidations(val []*ethpb.PendingConsolidation) error {
	if b.version < version.Electra {
		return errNotSupported("SetPendingConsolidations", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.PendingConsolidations].MinusRef()
	b.sharedFieldReferences[types.PendingConsolidations] = stateutil.NewRef(1)

	b.pendingConsolidations = val

	b.markFieldAsDirty(types.PendingConsolidations)
	b.rebuildTrie[types.PendingConsolidations] = true
	return nil
}

// SetEarliestConsolidationEpoch is a mutating call to the beacon state which sets the earliest
// consolidation epoch value. This method requires access to the Lock on the state and only applies
// in electra or later.
func (b *BeaconState) SetEarliestConsolidationEpoch(epoch primitives.Epoch) error {
	if b.version < version.Electra {
		return errNotSupported("SetEarliestConsolidationEpoch", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.earliestConsolidationEpoch = epoch

	b.markFieldAsDirty(types.EarliestConsolidationEpoch)
	b.rebuildTrie[types.EarliestConsolidationEpoch] = true
	return nil
}

// SetConsolidationBalanceToConsume is a mutating call to the beacon state which sets the value of
// the consolidation balance to consume to the provided value. This method requires access to the
// Lock on the state and only applies in electra or later.
func (b *BeaconState) SetConsolidationBalanceToConsume(balance primitives.Gwei) error {
	if b.version < version.Electra {
		return errNotSupported("SetConsolidationBalanceToConsume", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.consolidationBalanceToConsume = balance

	b.markFieldAsDirty(types.ConsolidationBalanceToConsume)
	b.rebuildTrie[types.ConsolidationBalanceToConsume] = true
	return nil
}
