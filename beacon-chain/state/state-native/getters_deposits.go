package state_native

import (
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// DepositBalanceToConsume is a non-mutating call to the beacon state which returns the value of the
// deposit balance to consume field. This method requires access to the RLock on the state and only
// applies in electra or later.
func (b *BeaconState) DepositBalanceToConsume() (primitives.Gwei, error) {
	if b.version < version.Electra {
		return 0, errNotSupported("DepositBalanceToConsume", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.depositBalanceToConsume, nil
}

// PendingDeposits is a non-mutating call to the beacon state which returns a deep copy of
// the pending balance deposit slice. This method requires access to the RLock on the state and
// only applies in electra or later.
func (b *BeaconState) PendingDeposits() ([]*ethpb.PendingDeposit, error) {
	if b.version < version.Electra {
		return nil, errNotSupported("PendingDeposits", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.pendingDepositsVal(), nil
}

func (b *BeaconState) pendingDepositsVal() []*ethpb.PendingDeposit {
	if b.pendingDeposits == nil {
		return nil
	}

	return ethpb.CopySlice(b.pendingDeposits)
}
