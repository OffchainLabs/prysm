package state_native

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
)

// ValidatorSweepThresholds returns a copy of the whole validator_sweep_thresholds list (EIP-8148).
func (b *BeaconState) ValidatorSweepThresholds() ([]uint64, error) {
	if b.version < version.Gloas {
		return nil, errNotSupported("ValidatorSweepThresholds", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.validatorSweepThresholdsVal(), nil
}

// ValidatorSweepThreshold returns the custom sweep threshold of the validator at “idx“ (EIP-8148).
// A zero value means the validator has no custom threshold configured.
func (b *BeaconState) ValidatorSweepThreshold(idx primitives.ValidatorIndex) (uint64, error) {
	if b.version < version.Gloas {
		return 0, errNotSupported("ValidatorSweepThreshold", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.validatorSweepThresholdAtIndex(idx)
}

// SetValidatorSweepThresholds overwrites the whole validator_sweep_thresholds list.
func (b *BeaconState) SetValidatorSweepThresholds(thresholds []uint64) error {
	if b.version < version.Gloas {
		return errNotSupported("SetValidatorSweepThresholds", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	if b.validatorSweepThresholdsMultiValue != nil {
		b.validatorSweepThresholdsMultiValue.Detach(b)
	}
	b.validatorSweepThresholdsMultiValue = NewMultiValueSweepThresholds(thresholds)

	b.markFieldAsDirty(types.ValidatorSweepThresholds)

	return nil
}

// SetValidatorSweepThresholdAtIndex sets the custom sweep threshold of a single validator.
func (b *BeaconState) SetValidatorSweepThresholdAtIndex(idx primitives.ValidatorIndex, threshold uint64) error {
	if b.version < version.Gloas {
		return errNotSupported("SetValidatorSweepThresholdAtIndex", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	validatorsLen := b.validatorsLen()
	if uint64(idx) >= uint64(validatorsLen) {
		return errors.Errorf("index %d out of range of %d validators", idx, validatorsLen)
	}

	if err := b.validatorSweepThresholdsMultiValue.UpdateAt(b, uint64(idx), threshold); err != nil {
		return errors.Wrap(err, "could not update validator sweep thresholds")
	}

	b.markFieldAsDirty(types.ValidatorSweepThresholds)

	return nil
}

// AppendValidatorSweepThreshold appends the sweep threshold for a newly registered validator.
func (b *BeaconState) AppendValidatorSweepThreshold(threshold uint64) error {
	if b.version < version.Gloas {
		return errNotSupported("AppendValidatorSweepThreshold", b.version)
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	b.validatorSweepThresholdsMultiValue.Append(b, threshold)
	b.markFieldAsDirty(types.ValidatorSweepThresholds)

	return nil
}

// validatorSweepThresholdsVal returns a copy of the validator sweep thresholds.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) validatorSweepThresholdsVal() []uint64 {
	if b.validatorSweepThresholdsMultiValue == nil {
		return nil
	}

	return b.validatorSweepThresholdsMultiValue.Value(b)
}

// validatorSweepThresholdAtIndex assumes that a lock is already held on BeaconState.
//
// The list is kept in lockstep with the validator registry, but a state that was
// upgraded mid-flight may still be shorter than the registry. Treat a missing entry
// as "no custom threshold" rather than an error so the sweep never stalls.
func (b *BeaconState) validatorSweepThresholdAtIndex(idx primitives.ValidatorIndex) (uint64, error) {
	if uint64(idx) >= uint64(b.validatorsLen()) {
		return 0, errors.Errorf("index %d out of range of %d validators", idx, b.validatorsLen())
	}

	if b.validatorSweepThresholdsMultiValue == nil ||
		uint64(idx) >= uint64(b.validatorSweepThresholdsMultiValue.Len(b)) {
		return 0, nil
	}

	return b.validatorSweepThresholdsMultiValue.At(b, uint64(idx))
}
