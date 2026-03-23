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
	currentDuties map[pubkey]*ethpb.ValidatorDuty
	nextDuties    map[pubkey]*ethpb.ValidatorDuty

	prevDependentRoot []byte
	currDependentRoot []byte

	// Next-epoch attester dependent root, stored so that epoch promotion
	// can set prevDepRoot without re-fetching current-epoch duties.
	nextAttDepRoot []byte

	epoch primitives.Epoch

	proposerSlots  map[primitives.ValidatorIndex][]primitives.Slot
	ptcSlots       map[primitives.ValidatorIndex][]primitives.Slot
	syncCurrentMap map[primitives.ValidatorIndex]bool
	syncNextMap    map[primitives.ValidatorIndex]bool

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

// DependentRoots returns the previous and current dependent roots.
func (ds *dutyStore) DependentRoots() (prev, curr []byte) {
	if !ds.IsInitialized() {
		return nil, nil
	}
	return ds.prevDependentRoot, ds.currDependentRoot
}

// CurrentEpochDuties returns the current epoch duties.
func (ds *dutyStore) CurrentEpochDuties() map[pubkey]*ethpb.ValidatorDuty {
	if !ds.IsInitialized() {
		return nil
	}
	return ds.currentDuties
}

// NextEpochDuties returns the next epoch duties.
func (ds *dutyStore) NextEpochDuties() map[pubkey]*ethpb.ValidatorDuty {
	if !ds.IsInitialized() {
		return nil
	}
	return ds.nextDuties
}

// CurrentDuty returns the current epoch duty for a given pubkey.
func (ds *dutyStore) CurrentDuty(pk pubkey) (*ethpb.ValidatorDuty, bool) {
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

// PtcSlots returns the PTC slots for a given validator index.
func (ds *dutyStore) PtcSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	if !ds.IsInitialized() {
		return nil
	}
	return ds.ptcSlots[idx]
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

// ToContainer reconstructs a ValidatorDutiesContainer from the duty store's internal state.
func (ds *dutyStore) ToContainer() *ethpb.ValidatorDutiesContainer {
	if !ds.IsInitialized() {
		return &ethpb.ValidatorDutiesContainer{}
	}
	current := make([]*ethpb.ValidatorDuty, 0, len(ds.currentDuties))
	for _, d := range ds.currentDuties {
		current = append(current, d)
	}
	next := make([]*ethpb.ValidatorDuty, 0, len(ds.nextDuties))
	for _, d := range ds.nextDuties {
		next = append(next, d)
	}
	return &ethpb.ValidatorDutiesContainer{
		PrevDependentRoot:  ds.prevDependentRoot,
		CurrDependentRoot:  ds.currDependentRoot,
		CurrentEpochDuties: current,
		NextEpochDuties:    next,
	}
}

// SetFromCombinedDutiesResponse stores a combined duties response by decomposing it into
// duty maps, proposer slots, and sync committee maps.
// DEPRECATED: GetDutiesV2, use the split GetAttesterDuties, GetProposerDutiesV2, GetSyncCommitteeDuties, GetPTCduties endpoints.
func (ds *dutyStore) SetFromCombinedDutiesResponse(container *ethpb.ValidatorDutiesContainer) {
	if container == nil {
		ds.Reset()
		return
	}

	ds.proposerSlots = make(map[primitives.ValidatorIndex][]primitives.Slot)
	ds.ptcSlots = make(map[primitives.ValidatorIndex][]primitives.Slot)
	ds.syncCurrentMap = make(map[primitives.ValidatorIndex]bool)
	ds.syncNextMap = make(map[primitives.ValidatorIndex]bool)

	ds.currentDuties = make(map[pubkey]*ethpb.ValidatorDuty, len(container.CurrentEpochDuties))
	for _, d := range container.CurrentEpochDuties {
		if d == nil {
			continue
		}
		ds.currentDuties[bytesutil.ToBytes48(d.PublicKey)] = d
		if len(d.ProposerSlots) > 0 {
			ds.proposerSlots[d.ValidatorIndex] = d.ProposerSlots
		}
		if d.IsSyncCommittee {
			ds.syncCurrentMap[d.ValidatorIndex] = true
		}
		if len(d.PtcSlots) > 0 {
			ds.ptcSlots[d.ValidatorIndex] = d.PtcSlots
		}
	}

	ds.nextDuties = make(map[pubkey]*ethpb.ValidatorDuty, len(container.NextEpochDuties))
	for _, d := range container.NextEpochDuties {
		if d == nil {
			continue
		}
		ds.nextDuties[bytesutil.ToBytes48(d.PublicKey)] = d
		if d.IsSyncCommittee {
			ds.syncNextMap[d.ValidatorIndex] = true
		}
	}

	ds.prevDependentRoot = container.PrevDependentRoot
	ds.currDependentRoot = container.CurrDependentRoot
	ds.initialized = true
}
