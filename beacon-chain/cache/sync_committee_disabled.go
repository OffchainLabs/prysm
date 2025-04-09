//go:build fuzz

package cache

import (
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

// FakeSyncCommitteeCache is a fake `SyncCommitteeCache` to satisfy fuzzing.
type FakeSyncCommitteeCache struct {
}

// NewSyncCommittee initializes and returns a new SyncCommitteeCache.
func NewSyncCommittee() *FakeSyncCommitteeCache {
	return &FakeSyncCommitteeCache{}
}

// CurrentEpochIndexPosition -- fake.
func (s *FakeSyncCommitteeCache) CurrentPeriodIndexPosition(root [32]byte, valIdx primitives.ValidatorIndex) ([]primitives.CommitteeIndex, error) {
	return nil, nil
}

// NextEpochIndexPosition -- fake.
func (s *FakeSyncCommitteeCache) NextPeriodIndexPosition(root [32]byte, valIdx primitives.ValidatorIndex) ([]primitives.CommitteeIndex, error) {
	return nil, nil
}

// UpdatePositionsInCommittee -- fake.
func (s *FakeSyncCommitteeCache) UpdatePositionsInCommittee(syncCommitteeBoundaryRoot [32]byte, state state.BeaconState) error {
	return nil
}

// Clear -- fake.
func (s *FakeSyncCommitteeCache) Clear() {
	return
}
