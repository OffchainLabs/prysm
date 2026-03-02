package client

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type pubkey = [fieldparams.BLSPubkeyLength]byte

// dutyStore stores validator duties from the beacon node.
// Both the legacy combined endpoint and the split per-duty endpoints
// populate the same internal maps, so accessor methods have a single code path.
type dutyStore struct {
	currentDuties map[pubkey]*ethpb.AttesterDuty
	nextDuties    map[pubkey]*ethpb.AttesterDuty

	attesterDependentRoot []byte
	proposerDependentRoot []byte

	// Derived maps for fast lookups.
	proposerSlots  map[primitives.ValidatorIndex][]primitives.Slot
	syncCurrentMap map[primitives.ValidatorIndex]bool
	syncNextMap    map[primitives.ValidatorIndex]bool

	// Sync cache (period-based, hits 255/256 epochs).
	syncPeriod      uint64
	syncCurrentResp *ethpb.SyncCommitteeDutiesResponse
	syncNextResp    *ethpb.SyncCommitteeDutiesResponse

	initialized bool
}

// Reset clears all duty data, marking the store as uninitialized.
func (ds *dutyStore) Reset() {
	*ds = dutyStore{}
}

// IsInitialized returns true if any duty data has been populated.
func (ds *dutyStore) IsInitialized() bool {
	if ds == nil {
		return false
	}
	return ds.initialized
}

// DependentRoots returns the attester and proposer dependent roots.
func (ds *dutyStore) DependentRoots() (attester, proposer []byte) {
	if !ds.IsInitialized() {
		return nil, nil
	}
	return ds.attesterDependentRoot, ds.proposerDependentRoot
}

// CurrentEpochDuties returns the current epoch attester duties.
func (ds *dutyStore) CurrentEpochDuties() map[pubkey]*ethpb.AttesterDuty {
	if !ds.IsInitialized() {
		return nil
	}
	return ds.currentDuties
}

// NextEpochDuties returns the next epoch attester duties.
func (ds *dutyStore) NextEpochDuties() map[pubkey]*ethpb.AttesterDuty {
	if !ds.IsInitialized() {
		return nil
	}
	return ds.nextDuties
}

// CurrentAttesterDuty returns the current epoch duty for a given pubkey.
func (ds *dutyStore) CurrentAttesterDuty(pk pubkey) (*ethpb.AttesterDuty, bool) {
	if !ds.IsInitialized() {
		return nil, false
	}
	d, ok := ds.currentDuties[pk]
	return d, ok
}

// ProposerSlots returns the proposer slots for a given validator index.
func (ds *dutyStore) ProposerSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	if !ds.IsInitialized() {
		return nil
	}
	return ds.proposerSlots[idx]
}

// IsSyncCommittee returns whether a validator is in the current sync committee.
func (ds *dutyStore) IsSyncCommittee(idx primitives.ValidatorIndex) bool {
	if !ds.IsInitialized() {
		return false
	}
	return ds.syncCurrentMap[idx]
}

// IsNextSyncCommittee returns whether a validator is in the next epoch's sync committee.
func (ds *dutyStore) IsNextSyncCommittee(idx primitives.ValidatorIndex) bool {
	if !ds.IsInitialized() {
		return false
	}
	return ds.syncNextMap[idx]
}

// SetLegacy stores a legacy combined duties response by decomposing it into
// attester duties, proposer slots, and sync committee maps.
func (ds *dutyStore) SetLegacy(container *ethpb.ValidatorDutiesContainer) {
	if container == nil {
		ds.Reset()
		return
	}

	ds.proposerSlots = make(map[primitives.ValidatorIndex][]primitives.Slot)
	ds.syncCurrentMap = make(map[primitives.ValidatorIndex]bool)
	ds.syncNextMap = make(map[primitives.ValidatorIndex]bool)

	// Convert current epoch duties.
	ds.currentDuties = make(map[pubkey]*ethpb.AttesterDuty, len(container.CurrentEpochDuties))
	for _, d := range container.CurrentEpochDuties {
		if d == nil {
			continue
		}
		ds.currentDuties[bytesutil.ToBytes48(d.PublicKey)] = &ethpb.AttesterDuty{
			Pubkey:                  d.PublicKey,
			ValidatorIndex:          d.ValidatorIndex,
			CommitteeIndex:          d.CommitteeIndex,
			CommitteeLength:         d.CommitteeLength,
			CommitteesAtSlot:        d.CommitteesAtSlot,
			ValidatorCommitteeIndex: d.ValidatorCommitteeIndex,
			Slot:                    d.AttesterSlot,
		}
		if len(d.ProposerSlots) > 0 {
			ds.proposerSlots[d.ValidatorIndex] = d.ProposerSlots
		}
		if d.IsSyncCommittee {
			ds.syncCurrentMap[d.ValidatorIndex] = true
		}
	}

	// Convert next epoch duties.
	ds.nextDuties = make(map[pubkey]*ethpb.AttesterDuty, len(container.NextEpochDuties))
	for _, d := range container.NextEpochDuties {
		if d == nil {
			continue
		}
		ds.nextDuties[bytesutil.ToBytes48(d.PublicKey)] = &ethpb.AttesterDuty{
			Pubkey:                  d.PublicKey,
			ValidatorIndex:          d.ValidatorIndex,
			CommitteeIndex:          d.CommitteeIndex,
			CommitteeLength:         d.CommitteeLength,
			CommitteesAtSlot:        d.CommitteesAtSlot,
			ValidatorCommitteeIndex: d.ValidatorCommitteeIndex,
			Slot:                    d.AttesterSlot,
		}
		if d.IsSyncCommittee {
			ds.syncNextMap[d.ValidatorIndex] = true
		}
	}

	ds.attesterDependentRoot = container.PrevDependentRoot
	ds.proposerDependentRoot = container.CurrDependentRoot
	ds.initialized = true
}

// attesterMap converts an attester duties response into a pubkey-keyed map.
func attesterMap(resp *ethpb.AttesterDutiesResponse) map[pubkey]*ethpb.AttesterDuty {
	if resp == nil {
		return nil
	}
	m := make(map[pubkey]*ethpb.AttesterDuty, len(resp.Duties))
	for _, d := range resp.Duties {
		m[bytesutil.ToBytes48(d.Pubkey)] = d
	}
	return m
}

// proposerSlotsMap builds a map of validator index to proposer slots from a proposer duties response.
func proposerSlotsMap(resp *ethpb.ProposerDutiesResponse) map[primitives.ValidatorIndex][]primitives.Slot {
	m := make(map[primitives.ValidatorIndex][]primitives.Slot)
	if resp != nil {
		for _, d := range resp.Duties {
			m[d.ValidatorIndex] = append(m[d.ValidatorIndex], d.Slot)
		}
	}
	return m
}

// syncMap builds a set of validator indices in a sync committee response.
func syncMap(resp *ethpb.SyncCommitteeDutiesResponse) map[primitives.ValidatorIndex]bool {
	m := make(map[primitives.ValidatorIndex]bool)
	if resp != nil {
		for _, d := range resp.Duties {
			m[d.ValidatorIndex] = true
		}
	}
	return m
}

// SyncCacheValid returns true if the stored sync data matches the given period.
func (ds *dutyStore) SyncCacheValid(period uint64) bool {
	return ds.syncCurrentResp != nil && ds.syncPeriod == period
}

// newDutyStoreFromLegacy creates a dutyStore from a legacy ValidatorDutiesContainer.
// This is a convenience helper primarily used in tests.
func newDutyStoreFromLegacy(container *ethpb.ValidatorDutiesContainer) *dutyStore {
	ds := &dutyStore{}
	ds.SetLegacy(container)
	return ds
}
