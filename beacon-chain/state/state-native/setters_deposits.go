package state_native

import (
	"errors"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/types"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/stateutil"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// AppendPendingDeposit is a mutating call to the beacon state to create and append a pending
// balance deposit object on to the state. This method requires access to the Lock on the state and
// only applies in electra or later.
func (b *BeaconState) AppendPendingDeposit(pd *ethpb.PendingDeposit) error {
	if b.version < version.Electra {
		return errNotSupported("AppendPendingDeposit", b.version)
	}
	if pd == nil {
		return errors.New("cannot append nil pending deposit")
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.PendingDeposits].MinusRef()
	b.sharedFieldReferences[types.PendingDeposits] = stateutil.NewRef(1)

	b.pendingDeposits = append(b.pendingDeposits, pd)

	b.markFieldAsDirty(types.PendingDeposits)
	b.rebuildTrie[types.PendingDeposits] = true
	return nil
}

// SetPendingDeposits is a mutating call to the beacon state which replaces the pending
// balance deposit slice with the provided value. This method requires access to the Lock on the
// state and only applies in electra or later.
func (b *BeaconState) SetPendingDeposits(val []*ethpb.PendingDeposit) error {
	if b.version < version.Electra {
		return errNotSupported("SetPendingDeposits", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.PendingDeposits].MinusRef()
	b.sharedFieldReferences[types.PendingDeposits] = stateutil.NewRef(1)

	b.pendingDeposits = val

	b.markFieldAsDirty(types.PendingDeposits)
	b.rebuildTrie[types.PendingDeposits] = true
	return nil
}

// SetDepositBalanceToConsume is a mutating call to the beacon state which sets the deposit balance
// to consume value to the given value. This method requires access to the Lock on the state and
// only applies in electra or later.
func (b *BeaconState) SetDepositBalanceToConsume(dbtc primitives.Gwei) error {
	if b.version < version.Electra {
		return errNotSupported("SetDepositBalanceToConsume", b.version)
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.depositBalanceToConsume = dbtc

	b.markFieldAsDirty(types.DepositBalanceToConsume)
	b.rebuildTrie[types.DepositBalanceToConsume] = true
	return nil
}
