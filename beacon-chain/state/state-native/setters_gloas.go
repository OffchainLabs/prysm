package state_native

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// SetBuilderPendingPayment sets a builder pending payment at the specified slot.
func (b *BeaconState) SetBuilderPendingPayment(index uint64, payment *ethpb.BuilderPendingPayment) error {
	if b.version < version.Gloas {
		return errNotSupported("SetBuilderPendingPayment", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if index >= uint64(len(b.builderPendingPayments)) {
		return fmt.Errorf("builder pending payment index %d out of range (len=%d)", index, len(b.builderPendingPayments))
	}
	b.builderPendingPayments[index] = ethpb.CopyBuilderPendingPayment(payment)

	b.markFieldAsDirty(types.BuilderPendingPayments)
	return nil
}
