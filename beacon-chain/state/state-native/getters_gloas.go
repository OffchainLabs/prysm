package state_native

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
)

// BuilderPendingPayment returns the builder pending payment for the given index.
func (b *BeaconState) BuilderPendingPayment(index uint64) (*ethpb.BuilderPendingPayment, error) {
	if b.version < version.Gloas {
		return nil, errNotSupported("BuilderPendingPayment", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if index >= uint64(len(b.builderPendingPayments)) {
		return nil, fmt.Errorf("builder pending payment index %d out of range (len=%d)", index, len(b.builderPendingPayments))
	}
	return ethpb.CopyBuilderPendingPayment(b.builderPendingPayments[index]), nil
}

// ExecutionPayloadAvailability returns the execution payload availability bit for the given slot.
func (b *BeaconState) ExecutionPayloadAvailability(slot primitives.Slot) (uint64, error) {
	if b.version < version.Gloas {
		return 0, errNotSupported("ExecutionPayloadAvailability", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	slotIndex := slot % params.BeaconConfig().SlotsPerHistoricalRoot
	byteIndex := slotIndex / 8
	bitIndex := slotIndex % 8

	bit := (b.executionPayloadAvailability[byteIndex] >> bitIndex) & 1

	return uint64(bit), nil
}
