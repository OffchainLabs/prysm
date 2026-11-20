package state_native

import (
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

func expectedAggregationCommitteeWindowSize() primitives.Slot {
	return params.BeaconConfig().SlotsPerEpoch.Mul(uint64(2 + params.BeaconConfig().MinSeedLookahead))
}

func (b *BeaconState) AggregationCommitteeWindow() ([]*ethpb.AggregationCommittees, error) {
	if b.version != version.PQ {
		return nil, errNotSupported("AggregationCommitteeWindow", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	return ethpb.CopyAggregationCommitteeWindow(b.aggregationCommitteeWindow), nil
}

// Spec: get_aggregation_committee
func (b *BeaconState) AggregationCommittee(slot primitives.Slot, index primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error) {
	if b.version != version.PQ {
		return nil, errNotSupported("AggregationCommittee", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	epoch := slots.ToEpoch(slot)
	stateEpoch := slots.ToEpoch(b.slot)
	offset := slot.ModSlot(slotsPerEpoch)
	switch {
	case epoch+1 == stateEpoch:
	case epoch >= stateEpoch && epoch <= stateEpoch+params.BeaconConfig().MinSeedLookahead:
		offset += slotsPerEpoch.Mul(uint64(epoch - stateEpoch + 1))
	default:
		return nil, fmt.Errorf("slot %d is outside the aggregation committee window at state slot %d", slot, b.slot)
	}
	if uint64(offset) >= uint64(len(b.aggregationCommitteeWindow)) {
		return nil, fmt.Errorf("aggregation committee window offset %d out of range for size %d", offset, len(b.aggregationCommitteeWindow))
	}
	committees := b.aggregationCommitteeWindow[offset].Committees
	if uint64(index) >= uint64(len(committees)) {
		return nil, fmt.Errorf("committee index %d out of range for %d aggregation committees", index, len(committees))
	}
	return slices.Clone(committees[index]), nil
}

func (b *BeaconState) VrfRootAtIndex(idx primitives.ValidatorIndex) ([32]byte, error) {
	if b.version != version.PQ {
		return [32]byte{}, errNotSupported("VrfRootAtIndex", b.version)
	}
	b.lock.RLock()
	defer b.lock.RUnlock()
	v, err := b.validatorsMultiValue.At(b, uint64(idx))
	if err != nil {
		return [32]byte{}, err
	}
	return v.VrfRoot, nil
}

func (b *BeaconState) SetAggregationCommitteeWindow(window []*ethpb.AggregationCommittees) error {
	if b.version != version.PQ {
		return errNotSupported("SetAggregationCommitteeWindow", b.version)
	}
	expected := expectedAggregationCommitteeWindowSize()
	if uint64(len(window)) != uint64(expected) {
		return fmt.Errorf("invalid size for aggregation committee window: got %d want %d", len(window), expected)
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.AggregationCommitteeWindow].MinusRef()
	b.sharedFieldReferences[types.AggregationCommitteeWindow] = stateutil.NewRef(1)
	b.aggregationCommitteeWindow = ethpb.CopyAggregationCommitteeWindow(window)
	b.markFieldAsDirty(types.AggregationCommitteeWindow)
	return nil
}

func (b *BeaconState) RotateAggregationCommitteeWindow(newEpochSlots []*ethpb.AggregationCommittees) error {
	if b.version != version.PQ {
		return errNotSupported("RotateAggregationCommitteeWindow", b.version)
	}
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	if uint64(len(newEpochSlots)) != uint64(slotsPerEpoch) {
		return fmt.Errorf("invalid new epoch slots size: got %d want %d", len(newEpochSlots), slotsPerEpoch)
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	expected := expectedAggregationCommitteeWindowSize()
	if uint64(len(b.aggregationCommitteeWindow)) != uint64(expected) {
		return fmt.Errorf("invalid aggregation committee window size: got %d want %d", len(b.aggregationCommitteeWindow), expected)
	}
	b.sharedFieldReferences[types.AggregationCommitteeWindow].MinusRef()
	b.sharedFieldReferences[types.AggregationCommitteeWindow] = stateutil.NewRef(1)

	newWindow := make([]*ethpb.AggregationCommittees, expected)
	lastEpochStart := expected - slotsPerEpoch
	copy(newWindow[:lastEpochStart], b.aggregationCommitteeWindow[slotsPerEpoch:])
	copy(newWindow[lastEpochStart:], ethpb.CopyAggregationCommitteeWindow(newEpochSlots))
	b.aggregationCommitteeWindow = newWindow
	b.markFieldAsDirty(types.AggregationCommitteeWindow)
	return nil
}

func (b *BeaconState) AppendValidatorPQ(val *ethpb.Validator, vrfRoot [32]byte) error {
	if b.version != version.PQ {
		return errNotSupported("AppendValidatorPQ", b.version)
	}
	compactValidator := stateutil.CompactValidatorFromProto(val)
	compactValidator.PQ, compactValidator.VrfRoot = true, vrfRoot
	b.validatorsMultiValue.Append(b, compactValidator)
	valIdx := primitives.ValidatorIndex(b.validatorsMultiValue.Len(b) - 1)

	b.lock.Lock()
	defer b.lock.Unlock()

	b.valMapHandler.Set(bytesutil.ToBytes48(val.PublicKey), valIdx)
	b.markFieldAsDirty(types.Validators)
	b.addDirtyIndices(types.Validators, []uint64{uint64(valIdx)})
	return nil
}
