package state_native

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
)

// SetValidators for the beacon state. Updates the entire
// to a new value by overwriting the previous one.
func (b *BeaconState) SetValidators(val []*ethpb.Validator) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.validatorsMultiValue != nil {
		b.validatorsMultiValue.Detach(b)
	}
	b.validatorsMultiValue = NewMultiValueValidators(val)

	b.markFieldAsDirty(types.Validators)
	b.rebuildTrie[types.Validators] = true
	b.valMapHandler = stateutil.NewValMapHandler(val)
	return nil
}

// ApplyToEveryValidator applies the provided callback function to each validator in the
// validator registry.
func (b *BeaconState) ApplyToEveryValidator(f func(idx int, val state.ReadOnlyValidator) (*ethpb.Validator, error)) error {
	var changedVals []uint64
	l := b.validatorsMultiValue.Len(b)
	for i := 0; i < l; i++ {
		v, err := b.validatorsMultiValue.At(b, uint64(i))
		if err != nil {
			return err
		}
		ro, err := NewValidator(v)
		if err != nil {
			return err
		}
		newVal, err := f(i, ro)
		if err != nil {
			return err
		}
		if newVal != nil {
			changedVals = append(changedVals, uint64(i))
			if err = b.validatorsMultiValue.UpdateAt(b, uint64(i), newVal); err != nil {
				return errors.Wrapf(err, "could not update validator at index %d", i)
			}
		}
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if len(changedVals) > 0 {
		b.markFieldAsDirty(types.Validators)
		b.addDirtyIndices(types.Validators, changedVals)
	}
	return nil
}

// UpdateValidatorAtIndex for the beacon state. Updates the validator
// at a specific index to a new value.
func (b *BeaconState) UpdateValidatorAtIndex(idx primitives.ValidatorIndex, val *ethpb.Validator) error {
	if err := b.validatorsMultiValue.UpdateAt(b, uint64(idx), val); err != nil {
		return errors.Wrap(err, "could not update validator")
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.markFieldAsDirty(types.Validators)
	b.addDirtyIndices(types.Validators, []uint64{uint64(idx)})
	return nil
}

// SetBalances for the beacon state. Updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetBalances(val []uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.balancesMultiValue != nil {
		b.balancesMultiValue.Detach(b)
	}
	b.balancesMultiValue = NewMultiValueBalances(val)

	b.markFieldAsDirty(types.Balances)
	b.rebuildTrie[types.Balances] = true
	return nil
}

// UpdateBalancesAtIndex for the beacon state. This method updates the balance
// at a specific index to a new value.
func (b *BeaconState) UpdateBalancesAtIndex(idx primitives.ValidatorIndex, val uint64) error {
	if err := b.balancesMultiValue.UpdateAt(b, uint64(idx), val); err != nil {
		return errors.Wrap(err, "could not update balances")
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.markFieldAsDirty(types.Balances)
	b.addDirtyIndices(types.Balances, []uint64{uint64(idx)})
	return nil
}

// SetSlashings for the beacon state. Updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetSlashings(val []uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.Slashings].MinusRef()
	b.sharedFieldReferences[types.Slashings] = stateutil.NewRef(1)

	b.slashings = val
	b.markFieldAsDirty(types.Slashings)
	return nil
}

// UpdateSlashingsAtIndex for the beacon state. Updates the slashings
// at a specific index to a new value.
func (b *BeaconState) UpdateSlashingsAtIndex(idx, val uint64) error {
	if uint64(len(b.slashings)) <= idx {
		return errors.Errorf("invalid index provided %d", idx)
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	s := b.slashings
	if b.sharedFieldReferences[types.Slashings].Refs() > 1 {
		s = b.slashingsVal()
		b.sharedFieldReferences[types.Slashings].MinusRef()
		b.sharedFieldReferences[types.Slashings] = stateutil.NewRef(1)
	}

	s[idx] = val

	b.slashings = s

	b.markFieldAsDirty(types.Slashings)
	return nil
}

// AppendValidator for the beacon state. Appends the new value
// to the end of list.
func (b *BeaconState) AppendValidator(val *ethpb.Validator) error {
	b.validatorsMultiValue.Append(b, val)
	valIdx := primitives.ValidatorIndex(b.validatorsMultiValue.Len(b) - 1)

	b.lock.Lock()
	defer b.lock.Unlock()

	b.valMapHandler.Set(bytesutil.ToBytes48(val.PublicKey), valIdx)
	b.markFieldAsDirty(types.Validators)
	b.addDirtyIndices(types.Validators, []uint64{uint64(valIdx)})
	return nil
}

// AppendBalance for the beacon state. Appends the new value
// to the end of list.
func (b *BeaconState) AppendBalance(bal uint64) error {
	b.balancesMultiValue.Append(b, bal)
	balIdx := uint64(b.balancesMultiValue.Len(b) - 1)

	b.lock.Lock()
	defer b.lock.Unlock()

	b.markFieldAsDirty(types.Balances)
	b.addDirtyIndices(types.Balances, []uint64{balIdx})
	return nil
}

// AppendInactivityScore for the beacon state.
func (b *BeaconState) AppendInactivityScore(s uint64) error {
	if b.version == version.Phase0 {
		return errNotSupported("AppendInactivityScore", b.version)
	}

	b.inactivityScoresMultiValue.Append(b, s)

	b.lock.Lock()
	defer b.lock.Unlock()

	b.markFieldAsDirty(types.InactivityScores)
	return nil
}

// SetInactivityScores for the beacon state. Updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetInactivityScores(val []uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.version == version.Phase0 {
		return errNotSupported("SetInactivityScores", b.version)
	}

	if b.inactivityScoresMultiValue != nil {
		b.inactivityScoresMultiValue.Detach(b)
	}
	b.inactivityScoresMultiValue = NewMultiValueInactivityScores(val)

	b.markFieldAsDirty(types.InactivityScores)
	return nil
}
