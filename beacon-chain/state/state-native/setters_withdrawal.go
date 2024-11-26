package state_native

import (
	"errors"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stateutil"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// SetNextWithdrawalIndex sets the index that will be assigned to the next withdrawal.
func (b *BeaconState) SetNextWithdrawalIndex(i uint64) error {
	if b.version < version.Capella {
		return errNotSupported("SetNextWithdrawalIndex", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.nextWithdrawalIndex = i
	b.markFieldAsDirty(types.NextWithdrawalIndex)
	return nil
}

// SetNextWithdrawalValidatorIndex sets the index of the validator which is
// next in line for a partial withdrawal.
func (b *BeaconState) SetNextWithdrawalValidatorIndex(i primitives.ValidatorIndex) error {
	if b.version < version.Capella {
		return errNotSupported("SetNextWithdrawalValidatorIndex", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.nextWithdrawalValidatorIndex = i
	b.markFieldAsDirty(types.NextWithdrawalValidatorIndex)
	return nil
}

// AppendPendingPartialWithdrawal is a mutating call to the beacon state which appends the given
// value to the end of the pending partial withdrawals slice in the state. This method requires
// access to the Lock on the state and only applies in electra or later.
func (b *BeaconState) AppendPendingPartialWithdrawal(ppw *eth.PendingPartialWithdrawal) error {
	if b.version < version.Electra {
		return errNotSupported("AppendPendingPartialWithdrawal", b.version)
	}

	if ppw == nil {
		return errors.New("cannot append nil pending partial withdrawal")
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.PendingPartialWithdrawals].MinusRef()
	b.sharedFieldReferences[types.PendingPartialWithdrawals] = stateutil.NewRef(1)

	b.pendingPartialWithdrawals = append(b.pendingPartialWithdrawals, ppw)

	b.markFieldAsDirty(types.PendingPartialWithdrawals)
	b.rebuildTrie[types.PendingPartialWithdrawals] = true
	return nil
}

// DequeuePendingPartialWithdrawals removes the partial withdrawals from the beginning of the partial withdrawals list.
func (b *BeaconState) DequeuePendingPartialWithdrawals(n uint64) error {
	if b.version < version.Electra {
		return errNotSupported("DequeuePendingPartialWithdrawals", b.version)
	}

	if n > uint64(len(b.pendingPartialWithdrawals)) {
		return errors.New("cannot dequeue more withdrawals than are in the queue")
	}

	if n == 0 {
		return nil // Don't wait on a lock for no reason.
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.PendingPartialWithdrawals].MinusRef()
	b.sharedFieldReferences[types.PendingPartialWithdrawals] = stateutil.NewRef(1)

	b.pendingPartialWithdrawals = b.pendingPartialWithdrawals[n:]

	b.markFieldAsDirty(types.PendingPartialWithdrawals)
	b.rebuildTrie[types.PendingPartialWithdrawals] = true

	return nil
}
