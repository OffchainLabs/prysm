package client

import (
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// dutyStore stores validator duty data as pre-computed attesterDutyView slices.
// Both the legacy combined endpoint and the split per-duty endpoints eagerly
// convert into the same internal representation, so accessor methods have a
// single code path with no branching on which endpoint populated the store.
type dutyStore struct {
	// Pre-computed duty views.
	currentDuties []*attesterDutyView
	nextDuties    []*attesterDutyView

	// Dependent roots.
	prevDependentRoot []byte
	currDependentRoot []byte

	// Pubkey -> current duty lookup (for O(1) CurrentAttesterDuty).
	pubkeyToDuty map[[fieldparams.BLSPubkeyLength]byte]*attesterDutyView

	// Sync committee membership (O(1) lookup, independent of duty views).
	syncCurrentMap map[primitives.ValidatorIndex]bool
	syncNextMap    map[primitives.ValidatorIndex]bool

	// Split cache entries (kept for updateDutiesSplit cache-skip logic).
	attester *attesterDutiesCacheEntry
	proposer *proposerDutiesCacheEntry
	sync     *syncDutiesCacheEntry

	// Whether initialized with valid data.
	initialized bool
}

// attesterDutyView is a unified view of an attester duty plus validator status,
// since the split AttesterDuty proto doesn't carry status.
type attesterDutyView struct {
	Pubkey                  [fieldparams.BLSPubkeyLength]byte
	ValidatorIndex          primitives.ValidatorIndex
	CommitteeIndex          primitives.CommitteeIndex
	CommitteeLength         uint64
	CommitteesAtSlot        uint64
	ValidatorCommitteeIndex uint64
	Slot                    primitives.Slot
	ProposerSlots           []primitives.Slot
	IsSyncCommittee         bool
	Status                  ethpb.ValidatorStatus
}

// Reset clears all duty data, marking the store as uninitialized.
func (ds *dutyStore) Reset() {
	ds.currentDuties = nil
	ds.nextDuties = nil
	ds.prevDependentRoot = nil
	ds.currDependentRoot = nil
	ds.pubkeyToDuty = nil
	ds.syncCurrentMap = nil
	ds.syncNextMap = nil
	ds.attester = nil
	ds.proposer = nil
	ds.sync = nil
	ds.initialized = false
}

// IsInitialized returns true if any duty data has been populated.
func (ds *dutyStore) IsInitialized() bool {
	if ds == nil {
		return false
	}
	return ds.initialized
}

// DependentRoots returns the previous and current dependent roots.
func (ds *dutyStore) DependentRoots() (prev, curr []byte) {
	if !ds.IsInitialized() {
		return nil, nil
	}
	return ds.prevDependentRoot, ds.currDependentRoot
}

// CurrentEpochDuties returns the current epoch duties as attesterDutyView slices.
func (ds *dutyStore) CurrentEpochDuties() []*attesterDutyView {
	if !ds.IsInitialized() {
		return nil
	}
	return ds.currentDuties
}

// NextEpochDuties returns the next epoch duties as attesterDutyView slices.
func (ds *dutyStore) NextEpochDuties() []*attesterDutyView {
	if !ds.IsInitialized() {
		return nil
	}
	return ds.nextDuties
}

// CurrentAttesterDuty returns the current epoch duty for a given pubkey. O(1) lookup.
func (ds *dutyStore) CurrentAttesterDuty(pubkey [fieldparams.BLSPubkeyLength]byte) (*attesterDutyView, bool) {
	if !ds.IsInitialized() {
		return nil, false
	}
	d, ok := ds.pubkeyToDuty[pubkey]
	return d, ok
}

// ProposerSlots returns the proposer slots for a given validator index.
func (ds *dutyStore) ProposerSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	if !ds.IsInitialized() {
		return nil
	}
	for _, d := range ds.currentDuties {
		if d.ValidatorIndex == idx {
			return d.ProposerSlots
		}
	}
	return nil
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

// AllCurrentExitedCount returns the count of EXITED validators in current epoch duties.
func (ds *dutyStore) AllCurrentExitedCount() (exited, total int) {
	if !ds.IsInitialized() {
		return 0, 0
	}
	for _, d := range ds.currentDuties {
		if d.Status == ethpb.ValidatorStatus_EXITED {
			exited++
		}
	}
	return exited, len(ds.currentDuties)
}

// statusForPubkey looks up validator status from a pubkeyToStatus map, defaulting to ACTIVE.
func statusForPubkey(pk [fieldparams.BLSPubkeyLength]byte, pubkeyToStatus map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus) ethpb.ValidatorStatus {
	if pubkeyToStatus != nil {
		if st, ok := pubkeyToStatus[pk]; ok && st.status != nil {
			return st.status.Status
		}
	}
	return ethpb.ValidatorStatus_ACTIVE
}

// legacyDutyToView converts a legacy ValidatorDuty to an attesterDutyView.
func legacyDutyToView(d *ethpb.ValidatorDuty) *attesterDutyView {
	if d == nil {
		return nil
	}
	var pk [fieldparams.BLSPubkeyLength]byte
	copy(pk[:], d.PublicKey)
	return &attesterDutyView{
		Pubkey:                  pk,
		ValidatorIndex:          d.ValidatorIndex,
		CommitteeIndex:          d.CommitteeIndex,
		CommitteeLength:         d.CommitteeLength,
		CommitteesAtSlot:        d.CommitteesAtSlot,
		ValidatorCommitteeIndex: d.ValidatorCommitteeIndex,
		Slot:                    d.AttesterSlot,
		ProposerSlots:           d.ProposerSlots,
		IsSyncCommittee:         d.IsSyncCommittee,
		Status:                  d.Status,
	}
}

// SetLegacy stores a legacy combined duties response by converting it to the unified format.
func (ds *dutyStore) SetLegacy(container *ethpb.ValidatorDutiesContainer, pubkeyToStatus map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus) {
	// Clear cache entries (legacy doesn't use per-duty caching).
	ds.attester = nil
	ds.proposer = nil
	ds.sync = nil

	if container == nil {
		ds.currentDuties = nil
		ds.nextDuties = nil
		ds.prevDependentRoot = nil
		ds.currDependentRoot = nil
		ds.pubkeyToDuty = nil
		ds.syncCurrentMap = nil
		ds.syncNextMap = nil
		ds.initialized = false
		return
	}

	// Convert current epoch duties.
	ds.currentDuties = make([]*attesterDutyView, 0, len(container.CurrentEpochDuties))
	ds.pubkeyToDuty = make(map[[fieldparams.BLSPubkeyLength]byte]*attesterDutyView, len(container.CurrentEpochDuties))
	ds.syncCurrentMap = make(map[primitives.ValidatorIndex]bool)
	for _, d := range container.CurrentEpochDuties {
		view := legacyDutyToView(d)
		if view != nil {
			ds.currentDuties = append(ds.currentDuties, view)
			ds.pubkeyToDuty[view.Pubkey] = view
			if view.IsSyncCommittee {
				ds.syncCurrentMap[view.ValidatorIndex] = true
			}
		}
	}

	// Convert next epoch duties.
	ds.nextDuties = make([]*attesterDutyView, 0, len(container.NextEpochDuties))
	ds.syncNextMap = make(map[primitives.ValidatorIndex]bool)
	for _, d := range container.NextEpochDuties {
		view := legacyDutyToView(d)
		if view != nil {
			ds.nextDuties = append(ds.nextDuties, view)
			if view.IsSyncCommittee {
				ds.syncNextMap[view.ValidatorIndex] = true
			}
		}
	}

	ds.prevDependentRoot = container.PrevDependentRoot
	ds.currDependentRoot = container.CurrDependentRoot
	ds.initialized = true
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

// buildDutyViews converts split attester duties into attesterDutyView slices,
// merging proposer and sync committee data.
func buildDutyViews(
	attResp *ethpb.AttesterDutiesResponse,
	propSlots map[primitives.ValidatorIndex][]primitives.Slot,
	syncMembership map[primitives.ValidatorIndex]bool,
	indexToPubkey map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte,
	pubkeyToStatus map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus,
) []*attesterDutyView {
	if attResp == nil {
		return nil
	}
	seen := make(map[primitives.ValidatorIndex]bool, len(attResp.Duties))
	views := make([]*attesterDutyView, 0, len(attResp.Duties))
	for _, d := range attResp.Duties {
		seen[d.ValidatorIndex] = true
		pk := indexToPubkey[d.ValidatorIndex]
		views = append(views, &attesterDutyView{
			Pubkey:                  pk,
			ValidatorIndex:          d.ValidatorIndex,
			CommitteeIndex:          d.CommitteeIndex,
			CommitteeLength:         d.CommitteeLength,
			CommitteesAtSlot:        d.CommitteesAtSlot,
			ValidatorCommitteeIndex: d.ValidatorCommitteeIndex,
			Slot:                    d.Slot,
			ProposerSlots:           propSlots[d.ValidatorIndex],
			IsSyncCommittee:         syncMembership[d.ValidatorIndex],
			Status:                  statusForPubkey(pk, pubkeyToStatus),
		})
	}
	// Add proposer-only validators not in the attester list (skip non-local validators).
	for idx, pSlots := range propSlots {
		if seen[idx] {
			continue
		}
		pk, ok := indexToPubkey[idx]
		if !ok {
			continue
		}
		views = append(views, &attesterDutyView{
			Pubkey:          pk,
			ValidatorIndex:  idx,
			ProposerSlots:   pSlots,
			IsSyncCommittee: syncMembership[idx],
			Status:          statusForPubkey(pk, pubkeyToStatus),
		})
	}
	return views
}

// SetSplit stores split per-duty responses by converting them to the unified format.
func (ds *dutyStore) SetSplit(
	att *attesterDutiesCacheEntry,
	prop *proposerDutiesCacheEntry,
	sc *syncDutiesCacheEntry,
	indexToPubkey map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte,
	pubkeyToStatus map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus,
) {
	// Store cache entries for updateDutiesSplit cache-skip logic.
	ds.attester = att
	ds.proposer = prop
	ds.sync = sc

	if att == nil || att.current == nil {
		ds.currentDuties = nil
		ds.nextDuties = nil
		ds.prevDependentRoot = nil
		ds.currDependentRoot = nil
		ds.pubkeyToDuty = nil
		ds.syncCurrentMap = nil
		ds.syncNextMap = nil
		ds.initialized = false
		return
	}

	// Extract dependent roots.
	ds.prevDependentRoot = att.current.DependentRoot
	if prop != nil && prop.current != nil {
		ds.currDependentRoot = prop.current.DependentRoot
	} else {
		ds.currDependentRoot = nil
	}

	// Build sync membership maps.
	if sc != nil {
		ds.syncCurrentMap = syncMap(sc.current)
		ds.syncNextMap = syncMap(sc.next)
	} else {
		ds.syncCurrentMap = make(map[primitives.ValidatorIndex]bool)
		ds.syncNextMap = make(map[primitives.ValidatorIndex]bool)
	}

	// Build current epoch views.
	var propSlotsMap map[primitives.ValidatorIndex][]primitives.Slot
	if prop != nil && prop.current != nil {
		propSlotsMap = proposerSlotsMap(prop.current)
	} else {
		propSlotsMap = make(map[primitives.ValidatorIndex][]primitives.Slot)
	}
	ds.currentDuties = buildDutyViews(att.current, propSlotsMap, ds.syncCurrentMap, indexToPubkey, pubkeyToStatus)

	// Build pubkey lookup.
	ds.pubkeyToDuty = make(map[[fieldparams.BLSPubkeyLength]byte]*attesterDutyView, len(ds.currentDuties))
	for _, d := range ds.currentDuties {
		ds.pubkeyToDuty[d.Pubkey] = d
	}

	// Build next epoch views.
	var nextPropSlots map[primitives.ValidatorIndex][]primitives.Slot
	if prop != nil && prop.next != nil {
		nextPropSlots = proposerSlotsMap(prop.next)
	} else {
		nextPropSlots = make(map[primitives.ValidatorIndex][]primitives.Slot)
	}
	ds.nextDuties = buildDutyViews(att.next, nextPropSlots, ds.syncNextMap, indexToPubkey, pubkeyToStatus)

	ds.initialized = true
}

// dutyViewToProto converts an attesterDutyView to a ValidatorDuty proto for SubscribeCommitteeSubnets compat.
func dutyViewToProto(dv *attesterDutyView) *ethpb.ValidatorDuty {
	return &ethpb.ValidatorDuty{
		PublicKey:               dv.Pubkey[:],
		ValidatorIndex:          dv.ValidatorIndex,
		CommitteeIndex:          dv.CommitteeIndex,
		CommitteeLength:         dv.CommitteeLength,
		CommitteesAtSlot:        dv.CommitteesAtSlot,
		ValidatorCommitteeIndex: dv.ValidatorCommitteeIndex,
		AttesterSlot:            dv.Slot,
		ProposerSlots:           dv.ProposerSlots,
		Status:                  dv.Status,
		IsSyncCommittee:         dv.IsSyncCommittee,
	}
}

// newDutyStoreFromLegacy creates a dutyStore from a legacy ValidatorDutiesContainer.
// This is a convenience helper primarily used in tests.
func newDutyStoreFromLegacy(container *ethpb.ValidatorDutiesContainer) *dutyStore {
	ds := &dutyStore{}
	ds.SetLegacy(container, nil)
	return ds
}

// AttesterDutiesCache returns the attester duties cache entry (for dependent root checking).
func (ds *dutyStore) AttesterDutiesCache() *attesterDutiesCacheEntry {
	return ds.attester
}

// ProposerDutiesCache returns the proposer duties cache entry.
func (ds *dutyStore) ProposerDutiesCache() *proposerDutiesCacheEntry {
	return ds.proposer
}

// SyncDutiesCache returns the sync duties cache entry.
func (ds *dutyStore) SyncDutiesCache() *syncDutiesCacheEntry {
	return ds.sync
}
