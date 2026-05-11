package client

import (
	"sync"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

type pubkey = [fieldparams.BLSPubkeyLength]byte

// dutyStoreData holds duty state with no synchronization. Methods on this
// type never lock; the surrounding dutyStore is responsible for serializing
// access. A value copy of dutyStoreData also serves as a read-only snapshot
// (see dutyStore.Snapshot) — maps and slices are aliased rather than deep
// copied, which is safe because writers always replace them wholesale.
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
}

func (d *dutyStoreData) IsInitialized() bool { return d.initialized }

func (d *dutyStoreData) PrevDependentRoot() []byte {
	if !d.initialized {
		return nil
	}
	return d.prevDependentRoot
}

func (d *dutyStoreData) CurrDependentRoot() []byte {
	if !d.initialized {
		return nil
	}
	return d.currDependentRoot
}

func (d *dutyStoreData) CurrentEpochDuties() map[pubkey]*ethpb.ValidatorDuty {
	if !d.initialized {
		return nil
	}
	return d.currentDuties
}

func (d *dutyStoreData) NextEpochDuties() map[pubkey]*ethpb.ValidatorDuty {
	if !d.initialized {
		return nil
	}
	return d.nextDuties
}

func (d *dutyStoreData) CurrentDuty(pk pubkey) (*ethpb.ValidatorDuty, bool) {
	if !d.initialized {
		return nil, false
	}
	v, ok := d.currentDuties[pk]
	return v, ok
}

func (d *dutyStoreData) ProposerSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	if !d.initialized {
		return nil
	}
	return d.proposerSlots[idx]
}

func (d *dutyStoreData) PtcSlots(idx primitives.ValidatorIndex) []primitives.Slot {
	if !d.initialized {
		return nil
	}
	return d.ptcSlots[idx]
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

func (d *dutyStoreData) canPromote(nextEpoch primitives.Epoch) bool {
	return d.initialized && d.epoch+1 == nextEpoch && d.missingNext == 0
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

// Snapshot returns a coherent read-only view of the store. The returned value
// can be inspected without holding any lock; maps and slices alias internal
// state but are never mutated in place (setFromContainer replaces them).
func (ds *dutyStore) Snapshot() dutyStoreData {
	if ds == nil {
		return dutyStoreData{}
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data
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

func (ds *dutyStore) canPromote(nextEpoch primitives.Epoch) bool {
	if ds == nil {
		return false
	}
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.canPromote(nextEpoch)
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

func (ds *dutyStore) CurrentEpochDuties() map[pubkey]*ethpb.ValidatorDuty {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.CurrentEpochDuties()
}

func (ds *dutyStore) NextEpochDuties() map[pubkey]*ethpb.ValidatorDuty {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.data.NextEpochDuties()
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
