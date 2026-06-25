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

func (d *dutyStoreData) isInitialized() bool { return d.initialized }

func (d *dutyStoreData) currentDuty(pk pubkey) (*ethpb.ValidatorDuty, bool) {
	if !d.initialized {
		return nil, false
	}
	v, ok := d.currentDuties[pk]
	if !ok {
		return nil, false
	}
	return cloneValidatorDuty(v), true
}

func (d *dutyStoreData) isSyncCommittee(idx primitives.ValidatorIndex) bool {
	if !d.initialized {
		return false
	}
	return d.syncCurrentMap[idx]
}

func (d *dutyStoreData) isNextSyncCommittee(idx primitives.ValidatorIndex) bool {
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

func (d *dutyStoreData) toContainer() *ethpb.ValidatorDutiesContainer {
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

// reset returns d to its zero value. Clearing every field is what keeps stale
// state (notably indices, which canPromote keys on) from surviving a rebuild.
func (d *dutyStoreData) reset() {
	*d = dutyStoreData{}
}

func (d *dutyStoreData) setFromContainer(container *ethpb.ValidatorDutiesContainer) {
	// Rebuild from scratch so no field can leak from a prior fetch, even if this
	// is ever called on an already-populated struct.
	d.reset()
	if container == nil {
		return
	}

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

// dutyStore is the concurrency-safe wrapper around dutyStoreData. All methods
// acquire mu internally. Compound reads should use snapshot to get a coherent
// view without holding the lock across long operations.
type dutyStore struct {
	mu   sync.RWMutex
	data dutyStoreData
	// revision bumps on every mutation. snapshot captures it so an async retry can
	// detect that the store changed under it and drop its now-stale write.
	revision uint64
}

// roDutySnapshot is a read-only view of dutyStore. Getters return copies; the
// duty iterators yield aliases that callers must not mutate.
type roDutySnapshot struct {
	d        dutyStoreData
	revision uint64
}

func (s roDutySnapshot) isInitialized() bool { return s.d.isInitialized() }

func (s roDutySnapshot) prevDependentRoot() []byte {
	if !s.d.initialized {
		return nil
	}
	return bytes.Clone(s.d.prevDependentRoot)
}

func (s roDutySnapshot) currDependentRoot() []byte {
	if !s.d.initialized {
		return nil
	}
	return bytes.Clone(s.d.currDependentRoot)
}

func (s roDutySnapshot) currentDuty(pk pubkey) (*ethpb.ValidatorDuty, bool) {
	return s.d.currentDuty(pk)
}

func (s roDutySnapshot) proposerSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	if !s.d.initialized {
		return nil
	}
	return slices.Clone(s.d.proposerSlots[idx])
}

func (s roDutySnapshot) ptcSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	if !s.d.initialized {
		return nil
	}
	return slices.Clone(s.d.ptcSlots[idx])
}

func (s roDutySnapshot) isSyncCommittee(idx primitives.ValidatorIndex) bool {
	return s.d.isSyncCommittee(idx)
}

func (s roDutySnapshot) isNextSyncCommittee(idx primitives.ValidatorIndex) bool {
	return s.d.isNextSyncCommittee(idx)
}

// currentDuties yields read-only current-epoch duty aliases. Re-rangeable.
func (s roDutySnapshot) currentDuties() iter.Seq2[pubkey, *ethpb.ValidatorDuty] {
	return func(yield func(pubkey, *ethpb.ValidatorDuty) bool) {
		if !s.d.initialized {
			return
		}
		for pk, duty := range s.d.currentDuties {
			if !yield(pk, duty) {
				return
			}
		}
	}
}

// nextDuties yields read-only next-epoch duty aliases. Re-rangeable.
func (s roDutySnapshot) nextDuties() iter.Seq2[pubkey, *ethpb.ValidatorDuty] {
	return func(yield func(pubkey, *ethpb.ValidatorDuty) bool) {
		if !s.d.initialized {
			return
		}
		for pk, duty := range s.d.nextDuties {
			if !yield(pk, duty) {
				return
			}
		}
	}
}

func (s roDutySnapshot) missingNext() missingNextDuties {
	if !s.d.initialized {
		return 0
	}
	return s.d.missingNext
}

func (s roDutySnapshot) epoch() primitives.Epoch {
	return s.d.epoch
}

func (s roDutySnapshot) indices() []primitives.ValidatorIndex {
	if !s.d.initialized {
		return nil
	}
	return slices.Clone(s.d.indices)
}

func (s roDutySnapshot) currentDutyCount() int {
	if !s.d.initialized {
		return 0
	}
	return len(s.d.currentDuties)
}

func (s roDutySnapshot) nextDutyCount() int {
	if !s.d.initialized {
		return 0
	}
	return len(s.d.nextDuties)
}

// snapshot returns a coherent read-only view of the store. The returned value
// can be inspected without holding any lock; maps and slices alias internal
// state but are never mutated in place (setFromContainer replaces them).
func (ds *dutyStore) snapshot() roDutySnapshot {
	if ds == nil {
		return roDutySnapshot{}
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return roDutySnapshot{d: ds.data, revision: ds.revision}
}

// needsNextRetry reports whether a per-type next-epoch retry has work to do —
// a cheap check so callers can avoid spawning a retry that would just no-op.
func (ds *dutyStore) needsNextRetry() bool {
	if ds == nil {
		return false
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.initialized && ds.data.missingNext != 0 && len(ds.data.indices) > 0
}

func (ds *dutyStore) reset() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.data.reset()
	ds.revision++
}

func (ds *dutyStore) isInitialized() bool {
	if ds == nil {
		return false
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.isInitialized()
}

func (ds *dutyStore) canPromote(nextEpoch primitives.Epoch, indices []primitives.ValidatorIndex) bool {
	if ds == nil {
		return false
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.canPromote(nextEpoch, indices)
}

func (ds *dutyStore) prevDependentRoot() []byte {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	if !ds.data.initialized {
		return nil
	}
	return bytes.Clone(ds.data.prevDependentRoot)
}

func (ds *dutyStore) currDependentRoot() []byte {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	if !ds.data.initialized {
		return nil
	}
	return bytes.Clone(ds.data.currDependentRoot)
}

// dependentRoots returns both dependent roots. Retained for compatibility
// with callers that want them in a single call; see prevDependentRoot and
// currDependentRoot for naming semantics.
func (ds *dutyStore) dependentRoots() (prev, curr []byte) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	if !ds.data.initialized {
		return nil, nil
	}
	return bytes.Clone(ds.data.prevDependentRoot), bytes.Clone(ds.data.currDependentRoot)
}

func (ds *dutyStore) currentDuty(pk pubkey) (*ethpb.ValidatorDuty, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.currentDuty(pk)
}

func (ds *dutyStore) proposerSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	if !ds.data.initialized {
		return nil
	}
	return slices.Clone(ds.data.proposerSlots[idx])
}

func (ds *dutyStore) ptcSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	if !ds.data.initialized {
		return nil
	}
	return slices.Clone(ds.data.ptcSlots[idx])
}

func (ds *dutyStore) isSyncCommittee(idx primitives.ValidatorIndex) bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.isSyncCommittee(idx)
}

func (ds *dutyStore) isNextSyncCommittee(idx primitives.ValidatorIndex) bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.isNextSyncCommittee(idx)
}

func (ds *dutyStore) toContainer() *ethpb.ValidatorDutiesContainer {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.toContainer()
}

// write atomically replaces the store's state with the given data.
func (ds *dutyStore) write(data dutyStoreData) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.data = data
	ds.revision++
}

// replaceNextDuties atomically swaps the next-epoch duties and missing mask,
// leaving current-epoch duties, epoch and indices intact. A non-nil currDepRoot
// also refreshes the current dependent root (the rebuilt next-epoch attester
// root); nil keeps the existing one. It applies only if the store hasn't changed
// since wantRevision (the revision the caller's fetch was based on), so a stale async
// retry whose epoch/duties advanced underneath it is dropped. Reports whether it
// applied.
func (ds *dutyStore) replaceNextDuties(wantRevision uint64, next []*ethpb.ValidatorDuty, missing missingNextDuties, currDepRoot []byte) bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if !ds.data.initialized || ds.revision != wantRevision {
		return false
	}
	container := ds.data.toContainer()
	container.NextEpochDuties = next
	if currDepRoot != nil {
		container.CurrDependentRoot = currDepRoot
	}
	epoch, indices := ds.data.epoch, ds.data.indices
	ds.data.setFromContainer(container)
	ds.data.epoch = epoch
	ds.data.indices = indices
	ds.data.missingNext = missing
	ds.revision++
	return true
}
