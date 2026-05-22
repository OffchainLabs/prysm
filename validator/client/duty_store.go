package client

import (
	"bytes"
	"iter"
	"slices"
	"sync"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// cloneValidatorDuty returns a deep copy: scalar fields are copied by value
// and slice fields are independently allocated, so the returned duty shares
// no memory with d.
func cloneValidatorDuty(d *ethpb.ValidatorDuty) *ethpb.ValidatorDuty {
	if d == nil {
		return nil
	}
	return &ethpb.ValidatorDuty{
		CommitteeLength:         d.CommitteeLength,
		CommitteeIndex:          d.CommitteeIndex,
		CommitteesAtSlot:        d.CommitteesAtSlot,
		ValidatorCommitteeIndex: d.ValidatorCommitteeIndex,
		AttesterSlot:            d.AttesterSlot,
		ProposerSlots:           slices.Clone(d.ProposerSlots),
		PublicKey:               bytes.Clone(d.PublicKey),
		Status:                  d.Status,
		ValidatorIndex:          d.ValidatorIndex,
		IsSyncCommittee:         d.IsSyncCommittee,
		PtcSlots:                slices.Clone(d.PtcSlots),
	}
}

type pubkey = [fieldparams.BLSPubkeyLength]byte

// dutyStoreData holds duty state with no synchronization. Methods on this
// type never lock; the surrounding dutyStore is responsible for serializing
// access. Maps and slices are aliased by snapshots rather than deep copied,
// which is safe because writers replace them wholesale via setFromContainer.
type dutyStoreData struct {
	missingNext       missingNextDuties
	initialized       bool
	syncNextMap       map[primitives.ValidatorIndex]bool
	syncCurrentMap    map[primitives.ValidatorIndex]bool
	ptcSlots          map[primitives.ValidatorIndex][]primitives.Slot
	proposerSlots     map[primitives.ValidatorIndex][]primitives.Slot
	nextDuties        map[pubkey]*ethpb.ValidatorDuty
	currentDuties     map[pubkey]*ethpb.ValidatorDuty
	epoch             primitives.Epoch
	currDependentRoot []byte
	prevDependentRoot []byte
	// indices is the sorted set of validator indices the last fetch was built
	// from. canPromote requires this to match the current request's indices.
	indices []primitives.ValidatorIndex
}

func (d *dutyStoreData) IsInitialized() bool { return d.initialized }

func (d *dutyStoreData) PrevDependentRoot() []byte {
	if !d.initialized {
		return nil
	}
	return bytes.Clone(d.prevDependentRoot)
}

func (d *dutyStoreData) CurrDependentRoot() []byte {
	if !d.initialized {
		return nil
	}
	return bytes.Clone(d.currDependentRoot)
}

func (d *dutyStoreData) CurrentDuty(pk pubkey) (*ethpb.ValidatorDuty, bool) {
	if !d.initialized {
		return nil, false
	}
	v, ok := d.currentDuties[pk]
	if !ok {
		return nil, false
	}
	return cloneValidatorDuty(v), true
}

func (d *dutyStoreData) ProposerSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	if !d.initialized {
		return nil
	}
	return slices.Clone(d.proposerSlots[idx])
}

func (d *dutyStoreData) PtcSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	if !d.initialized {
		return nil
	}
	return slices.Clone(d.ptcSlots[idx])
}

func (d *dutyStoreData) IsSyncCommittee(idx primitives.ValidatorIndex) bool {
	if !d.initialized {
		return false
	}
	return d.syncCurrentMap[idx]
}

func (d *dutyStoreData) IsNextSyncCommittee(idx primitives.ValidatorIndex) bool {
	if !d.initialized {
		return false
	}
	return d.syncNextMap[idx]
}

func (d *dutyStoreData) canPromote(nextEpoch primitives.Epoch, indices []primitives.ValidatorIndex) bool {
	if !d.initialized || d.epoch+1 != nextEpoch || d.missingNext != 0 {
		return false
	}
	// Both slices are kept sorted; differing length or any element mismatch
	// signals a validator-set drift (activation, exit, keymanager change) and
	// invalidates the cached duties for promotion.
	if len(d.indices) != len(indices) {
		return false
	}
	for i, idx := range d.indices {
		if idx != indices[i] {
			return false
		}
	}
	return true
}

func (d *dutyStoreData) ToContainer() *ethpb.ValidatorDutiesContainer {
	if !d.initialized {
		return &ethpb.ValidatorDutiesContainer{}
	}
	current := make([]*ethpb.ValidatorDuty, 0, len(d.currentDuties))
	for _, duty := range d.currentDuties {
		current = append(current, duty)
	}
	next := make([]*ethpb.ValidatorDuty, 0, len(d.nextDuties))
	for _, duty := range d.nextDuties {
		next = append(next, duty)
	}
	return &ethpb.ValidatorDutiesContainer{
		PrevDependentRoot:  d.prevDependentRoot,
		CurrDependentRoot:  d.currDependentRoot,
		CurrentEpochDuties: current,
		NextEpochDuties:    next,
	}
}

func (d *dutyStoreData) setFromContainer(container *ethpb.ValidatorDutiesContainer) {
	if container == nil {
		*d = dutyStoreData{}
		return
	}

	d.epoch = 0
	d.missingNext = 0
	d.proposerSlots = make(map[primitives.ValidatorIndex][]primitives.Slot)
	d.ptcSlots = make(map[primitives.ValidatorIndex][]primitives.Slot)
	d.syncCurrentMap = make(map[primitives.ValidatorIndex]bool)
	d.syncNextMap = make(map[primitives.ValidatorIndex]bool)

	d.currentDuties = make(map[pubkey]*ethpb.ValidatorDuty, len(container.CurrentEpochDuties))
	for _, duty := range container.CurrentEpochDuties {
		if duty == nil {
			continue
		}
		d.currentDuties[bytesutil.ToBytes48(duty.PublicKey)] = duty
		if len(duty.ProposerSlots) > 0 {
			d.proposerSlots[duty.ValidatorIndex] = duty.ProposerSlots
		}
		if duty.IsSyncCommittee {
			d.syncCurrentMap[duty.ValidatorIndex] = true
		}
		if len(duty.PtcSlots) > 0 {
			d.ptcSlots[duty.ValidatorIndex] = duty.PtcSlots
		}
	}

	d.nextDuties = make(map[pubkey]*ethpb.ValidatorDuty, len(container.NextEpochDuties))
	for _, duty := range container.NextEpochDuties {
		if duty == nil {
			continue
		}
		d.nextDuties[bytesutil.ToBytes48(duty.PublicKey)] = duty
		if duty.IsSyncCommittee {
			d.syncNextMap[duty.ValidatorIndex] = true
		}
	}

	d.prevDependentRoot = container.PrevDependentRoot
	d.currDependentRoot = container.CurrDependentRoot
	d.initialized = true
}

// dutyStore is the concurrency-safe wrapper around dutyStoreData. All exported
// methods acquire mu internally. Compound reads should use Snapshot to get a
// coherent view without holding the lock across long operations.
type dutyStore struct {
	mu   sync.RWMutex
	data dutyStoreData
}

// roDutySnapshot is a read-only view of dutyStore. Getters return defensive
// copies, so mutating the returned values can't affect the live store.
type roDutySnapshot struct {
	d dutyStoreData
}

func (s roDutySnapshot) IsInitialized() bool { return s.d.IsInitialized() }

func (s roDutySnapshot) PrevDependentRoot() []byte { return s.d.PrevDependentRoot() }

func (s roDutySnapshot) CurrDependentRoot() []byte { return s.d.CurrDependentRoot() }

func (s roDutySnapshot) CurrentDuty(pk pubkey) (*ethpb.ValidatorDuty, bool) {
	return s.d.CurrentDuty(pk)
}

func (s roDutySnapshot) ProposerSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	return s.d.ProposerSlots(idx)
}

func (s roDutySnapshot) PtcSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	return s.d.PtcSlots(idx)
}

func (s roDutySnapshot) IsSyncCommittee(idx primitives.ValidatorIndex) bool {
	return s.d.IsSyncCommittee(idx)
}

func (s roDutySnapshot) IsNextSyncCommittee(idx primitives.ValidatorIndex) bool {
	return s.d.IsNextSyncCommittee(idx)
}

// CurrentDuties yields cloned current-epoch duties. The iterator is re-rangeable.
func (s roDutySnapshot) CurrentDuties() iter.Seq2[pubkey, *ethpb.ValidatorDuty] {
	return func(yield func(pubkey, *ethpb.ValidatorDuty) bool) {
		if !s.d.initialized {
			return
		}
		for pk, duty := range s.d.currentDuties {
			if !yield(pk, cloneValidatorDuty(duty)) {
				return
			}
		}
	}
}

// NextDuties yields cloned next-epoch duties. The iterator is re-rangeable.
func (s roDutySnapshot) NextDuties() iter.Seq2[pubkey, *ethpb.ValidatorDuty] {
	return func(yield func(pubkey, *ethpb.ValidatorDuty) bool) {
		if !s.d.initialized {
			return
		}
		for pk, duty := range s.d.nextDuties {
			if !yield(pk, cloneValidatorDuty(duty)) {
				return
			}
		}
	}
}

func (s roDutySnapshot) CurrentDutyCount() int {
	if !s.d.initialized {
		return 0
	}
	return len(s.d.currentDuties)
}

func (s roDutySnapshot) NextDutyCount() int {
	if !s.d.initialized {
		return 0
	}
	return len(s.d.nextDuties)
}

// Snapshot returns a coherent read-only view of the store. The returned value
// can be inspected without holding any lock; maps and slices alias internal
// state but are never mutated in place (setFromContainer replaces them).
func (ds *dutyStore) Snapshot() roDutySnapshot {
	if ds == nil {
		return roDutySnapshot{}
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return roDutySnapshot{d: ds.data}
}

func (ds *dutyStore) Reset() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.data.setFromContainer(nil)
}

func (ds *dutyStore) IsInitialized() bool {
	if ds == nil {
		return false
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.IsInitialized()
}

func (ds *dutyStore) canPromote(nextEpoch primitives.Epoch, indices []primitives.ValidatorIndex) bool {
	if ds == nil {
		return false
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.canPromote(nextEpoch, indices)
}

func (ds *dutyStore) PrevDependentRoot() []byte {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.PrevDependentRoot()
}

func (ds *dutyStore) CurrDependentRoot() []byte {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.CurrDependentRoot()
}

// DependentRoots returns both dependent roots. Retained for compatibility
// with callers that want them in a single call; see PrevDependentRoot and
// CurrDependentRoot for naming semantics.
func (ds *dutyStore) DependentRoots() (prev, curr []byte) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.PrevDependentRoot(), ds.data.CurrDependentRoot()
}

func (ds *dutyStore) CurrentDuty(pk pubkey) (*ethpb.ValidatorDuty, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.CurrentDuty(pk)
}

func (ds *dutyStore) ProposerSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.ProposerSlots(idx)
}

func (ds *dutyStore) PtcSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.PtcSlots(idx)
}

func (ds *dutyStore) IsSyncCommittee(idx primitives.ValidatorIndex) bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.IsSyncCommittee(idx)
}

func (ds *dutyStore) IsNextSyncCommittee(idx primitives.ValidatorIndex) bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.IsNextSyncCommittee(idx)
}

func (ds *dutyStore) ToContainer() *ethpb.ValidatorDutiesContainer {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.ToContainer()
}

// Write atomically replaces the store's state with the given data.
func (ds *dutyStore) Write(data dutyStoreData) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.data = data
}
