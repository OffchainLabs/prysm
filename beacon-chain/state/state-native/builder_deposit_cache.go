package state_native

import (
	"slices"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache/depositsignature"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// BuilderDepositSignatureCache returns the ephemeral cache shared by copies of this state.
func (b *BeaconState) BuilderDepositSignatureCache() *depositsignature.Cache {
	return b.builderDepositSignatureCache
}

// PendingDepositsForPreverification returns a shallow snapshot whose elements must remain read-only.
func (b *BeaconState) PendingDepositsForPreverification() ([]*ethpb.PendingDeposit, error) {
	if b.version < version.Electra {
		return nil, errNotSupported("PendingDepositsForPreverification", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return slices.Clone(b.pendingDeposits), nil
}
