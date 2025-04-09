package state_native

import (
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// PreviousEpochAttestations corresponding to blocks on the beacon chain.
func (b *BeaconState) PreviousEpochAttestations() ([]*ethpb.PendingAttestation, error) {
	if b.version != version.Phase0 {
		return nil, errNotSupported("PreviousEpochAttestations", b.version)
	}

	if b.previousEpochAttestations == nil {
		return nil, nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.previousEpochAttestationsVal(), nil
}

// previousEpochAttestationsVal corresponding to blocks on the beacon chain.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) previousEpochAttestationsVal() []*ethpb.PendingAttestation {
	if b.previousEpochAttestations == nil {
		return nil
	}

	res := make([]*ethpb.PendingAttestation, len(b.previousEpochAttestations))
	for i := 0; i < len(res); i++ {
		res[i] = b.previousEpochAttestations[i].Copy()
	}
	return res
}

// CurrentEpochAttestations corresponding to blocks on the beacon chain.
func (b *BeaconState) CurrentEpochAttestations() ([]*ethpb.PendingAttestation, error) {
	if b.version != version.Phase0 {
		return nil, errNotSupported("CurrentEpochAttestations", b.version)
	}

	if b.currentEpochAttestations == nil {
		return nil, nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.currentEpochAttestationsVal(), nil
}

// currentEpochAttestations corresponding to blocks on the beacon chain.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) currentEpochAttestationsVal() []*ethpb.PendingAttestation {
	if b.currentEpochAttestations == nil {
		return nil
	}

	res := make([]*ethpb.PendingAttestation, len(b.currentEpochAttestations))
	for i := 0; i < len(res); i++ {
		res[i] = b.currentEpochAttestations[i].Copy()
	}
	return res
}
