package state_native

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/time"
	customtypes "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/custom-types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
)

// CurrentEpochParticipation corresponding to participation bits on the beacon chain.
func (b *BeaconState) CurrentEpochParticipation() ([]byte, error) {
	if b.version == version.Phase0 {
		return nil, errNotSupported("CurrentEpochParticipation", b.version)
	}

	if b.currentEpochParticipation == nil {
		return nil, nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.currentEpochParticipationVal(), nil
}

// PreviousEpochParticipation corresponding to participation bits on the beacon chain.
func (b *BeaconState) PreviousEpochParticipation() ([]byte, error) {
	if b.version == version.Phase0 {
		return nil, errNotSupported("PreviousEpochParticipation", b.version)
	}

	if b.previousEpochParticipation == nil {
		return nil, nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.previousEpochParticipationVal(), nil
}

// CurrentEpochParticipationReadOnly corresponding to participation bits on the beacon chain without copying the data.
func (b *BeaconState) CurrentEpochParticipationReadOnly() (customtypes.ReadOnlyParticipation, error) {
	if b.version == version.Phase0 {
		return customtypes.ReadOnlyParticipation{}, errNotSupported("CurrentEpochParticipation", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return customtypes.NewReadOnlyParticipation(b.currentEpochParticipation), nil
}

// PreviousEpochParticipationReadOnly corresponding to participation bits on the beacon chain without copying the data.
func (b *BeaconState) PreviousEpochParticipationReadOnly() (customtypes.ReadOnlyParticipation, error) {
	if b.version == version.Phase0 {
		return customtypes.ReadOnlyParticipation{}, errNotSupported("PreviousEpochParticipation", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return customtypes.NewReadOnlyParticipation(b.previousEpochParticipation), nil
}

// UnrealizedCheckpointBalances returns the total balances: active, target attested in
// current epoch and target attested in previous epoch. This function is used to
// compute the "unrealized justification" that a synced Beacon Block will have.
func (b *BeaconState) UnrealizedCheckpointBalances() (uint64, uint64, uint64, error) {
	if b.version == version.Phase0 {
		return 0, 0, 0, errNotSupported("UnrealizedCheckpointBalances", b.version)
	}

	currentEpoch := time.CurrentEpoch(b)
	b.lock.RLock()
	defer b.lock.RUnlock()

	cp := b.currentEpochParticipation
	pp := b.previousEpochParticipation
	if cp == nil || pp == nil {
		return 0, 0, 0, ErrNilParticipation
	}

	return stateutil.UnrealizedCheckpointBalances(cp, pp, stateutil.NewValMultiValueSliceReader(b.validatorsMultiValue, b), currentEpoch)
}

// currentEpochParticipationVal corresponding to participation bits on the beacon chain.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) currentEpochParticipationVal() []byte {
	tmp := make([]byte, len(b.currentEpochParticipation))
	copy(tmp, b.currentEpochParticipation)
	return tmp
}

// previousEpochParticipationVal corresponding to participation bits on the beacon chain.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) previousEpochParticipationVal() []byte {
	tmp := make([]byte, len(b.previousEpochParticipation))
	copy(tmp, b.previousEpochParticipation)
	return tmp
}
