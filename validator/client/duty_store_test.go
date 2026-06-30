package client

import (
	"reflect"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

func testDutyStore(current ...*ethpb.ValidatorDuty) *dutyStore {
	ds := &dutyStore{}
	ds.data = dutyStoreData{
		currentDuties:  make(map[pubkey]*ethpb.ValidatorDuty),
		nextDuties:     make(map[pubkey]*ethpb.ValidatorDuty),
		proposerSlots:  make(map[primitives.ValidatorIndex][]primitives.Slot),
		ptcSlots:       make(map[primitives.ValidatorIndex][]primitives.Slot),
		syncCurrentMap: make(map[primitives.ValidatorIndex]bool),
		syncNextMap:    make(map[primitives.ValidatorIndex]bool),
		initialized:    true,
	}
	for _, d := range current {
		ds.data.currentDuties[bytesutil.ToBytes48(d.PublicKey)] = d
		if len(d.ProposerSlots) > 0 {
			ds.data.proposerSlots[d.ValidatorIndex] = d.ProposerSlots
		}
		if d.IsSyncCommittee {
			ds.data.syncCurrentMap[d.ValidatorIndex] = true
		}
		if len(d.PtcSlots) > 0 {
			ds.data.ptcSlots[d.ValidatorIndex] = d.PtcSlots
		}
	}
	return ds
}

func TestDutyStore_Uninitialized(t *testing.T) {
	ds := &dutyStore{}
	assert.Equal(t, false, ds.isInitialized())
	snap := ds.snapshot()
	assert.Equal(t, 0, snap.currentDutyCount())
	assert.Equal(t, 0, snap.nextDutyCount())

	assert.Equal(t, true, ds.prevDependentRoot() == nil)
	assert.Equal(t, true, ds.currDependentRoot() == nil)

	d, ok := ds.currentDuty(pubkey{})
	assert.Equal(t, false, ok)
	assert.Equal(t, (*ethpb.ValidatorDuty)(nil), d)

	assert.Equal(t, true, ds.proposerSlots(0) == nil)
	assert.Equal(t, true, ds.ptcSlots(0) == nil)
	assert.Equal(t, false, ds.isSyncCommittee(0))
	assert.Equal(t, false, ds.isNextSyncCommittee(0))
}

func TestDutyStore_ZeroValueIsNotInitialized(t *testing.T) {
	ds := &dutyStore{}
	assert.Equal(t, false, ds.isInitialized())
}

func TestDutyStore_Write(t *testing.T) {
	pk1 := bytesutil.ToBytes48([]byte{1})
	pk2 := bytesutil.ToBytes48([]byte{2})

	container := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{
				PublicKey:       pk1[:],
				ValidatorIndex:  10,
				AttesterSlot:    5,
				ProposerSlots:   []primitives.Slot{3, 7},
				PtcSlots:        []primitives.Slot{4, 6},
				IsSyncCommittee: true,
			},
		},
		NextEpochDuties: []*ethpb.ValidatorDuty{
			{
				PublicKey:       pk2[:],
				ValidatorIndex:  20,
				AttesterSlot:    12,
				IsSyncCommittee: true,
			},
		},
		PrevDependentRoot: []byte("prev"),
		CurrDependentRoot: []byte("curr"),
	}

	ds := &dutyStore{}
	{
		var data dutyStoreData
		data.setFromContainer(container)
		ds.write(data)
	}

	assert.Equal(t, true, ds.isInitialized())

	// Current duties.
	d, ok := ds.currentDuty(pk1)
	assert.Equal(t, true, ok)
	assert.Equal(t, primitives.ValidatorIndex(10), d.ValidatorIndex)

	_, ok = ds.currentDuty(pk2)
	assert.Equal(t, false, ok)

	// Next duties.
	snap := ds.snapshot()
	assert.Equal(t, 1, snap.nextDutyCount())
	for pk, duty := range snap.nextDuties() {
		assert.Equal(t, pk2, pk)
		assert.Equal(t, primitives.ValidatorIndex(20), duty.ValidatorIndex)
	}

	// Dependent roots.
	assert.DeepEqual(t, []byte("prev"), ds.prevDependentRoot())
	assert.DeepEqual(t, []byte("curr"), ds.currDependentRoot())

	// Proposer slots.
	assert.DeepEqual(t, []primitives.Slot{3, 7}, ds.proposerSlots(10))
	assert.Equal(t, true, ds.proposerSlots(20) == nil)

	// PTC slots.
	assert.DeepEqual(t, []primitives.Slot{4, 6}, ds.ptcSlots(10))
	assert.Equal(t, true, ds.ptcSlots(20) == nil)

	// Sync committee.
	assert.Equal(t, true, ds.isSyncCommittee(10))
	assert.Equal(t, false, ds.isSyncCommittee(20))
	assert.Equal(t, false, ds.isNextSyncCommittee(10))
	assert.Equal(t, true, ds.isNextSyncCommittee(20))
}

func TestDutyStore_Reset(t *testing.T) {
	ds := testDutyStore(&ethpb.ValidatorDuty{PublicKey: make([]byte, 48)})
	ds.data.prevDependentRoot = []byte("prev")
	ds.data.currDependentRoot = []byte("curr")
	assert.Equal(t, true, ds.isInitialized())

	ds.reset()
	assert.Equal(t, false, ds.isInitialized())
	assert.Equal(t, 0, ds.snapshot().currentDutyCount())
}

func TestDutyStoreData_Reset(t *testing.T) {
	populated := func() dutyStoreData {
		return dutyStoreData{
			initialized:       true,
			epoch:             9,
			missingNext:       missingNextPtc,
			indices:           []primitives.ValidatorIndex{1, 5, 7},
			currentDuties:     map[pubkey]*ethpb.ValidatorDuty{{}: {}},
			prevDependentRoot: []byte("prev"),
		}
	}

	t.Run("reset zeroes every field", func(t *testing.T) {
		d := populated()
		d.reset()
		// Covers every field, including any added later: IsZero reports whether
		// the whole struct equals its zero value.
		assert.Equal(t, true, reflect.ValueOf(d).IsZero())
	})

	t.Run("setFromContainer drops stale indices on a populated struct", func(t *testing.T) {
		d := populated()
		d.setFromContainer(&ethpb.ValidatorDutiesContainer{
			CurrentEpochDuties: []*ethpb.ValidatorDuty{{PublicKey: make([]byte, 48), ValidatorIndex: 2}},
		})
		assert.Equal(t, true, d.indices == nil)
		// With indices cleared, a stale validator set can't satisfy canPromote
		// even when the epoch lines up.
		d.epoch = 9
		assert.Equal(t, false, d.canPromote(10, []primitives.ValidatorIndex{1, 5, 7}))
	})
}

func TestDutyStore_WriteNilResets(t *testing.T) {
	ds := testDutyStore(&ethpb.ValidatorDuty{PublicKey: make([]byte, 48)})
	assert.Equal(t, true, ds.isInitialized())

	{
		var data dutyStoreData
		data.setFromContainer(nil)
		ds.write(data)
	}
	assert.Equal(t, false, ds.isInitialized())
}

func TestDutyStore_WriteSkipsNilDuties(t *testing.T) {
	ds := &dutyStore{}
	{
		var data dutyStoreData
		data.setFromContainer(&ethpb.ValidatorDutiesContainer{
			CurrentEpochDuties: []*ethpb.ValidatorDuty{nil, {PublicKey: make([]byte, 48), ValidatorIndex: 1}},
			NextEpochDuties:    []*ethpb.ValidatorDuty{nil},
		})
		ds.write(data)
	}
	snap := ds.snapshot()
	assert.Equal(t, 1, snap.currentDutyCount())
	assert.Equal(t, 0, snap.nextDutyCount())
}

func TestDutyStoreData_CanPromote(t *testing.T) {
	base := func() dutyStoreData {
		return dutyStoreData{
			initialized: true,
			epoch:       9,
			indices:     []primitives.ValidatorIndex{1, 5, 7},
		}
	}

	t.Run("happy path: matching epoch + indices + zero missing", func(t *testing.T) {
		d := base()
		assert.Equal(t, true, d.canPromote(10, []primitives.ValidatorIndex{1, 5, 7}))
	})

	t.Run("uninitialized cannot promote", func(t *testing.T) {
		d := base()
		d.initialized = false
		assert.Equal(t, false, d.canPromote(10, []primitives.ValidatorIndex{1, 5, 7}))
	})

	t.Run("non-adjacent epoch cannot promote", func(t *testing.T) {
		d := base()
		assert.Equal(t, false, d.canPromote(11, []primitives.ValidatorIndex{1, 5, 7}))
		assert.Equal(t, false, d.canPromote(9, []primitives.ValidatorIndex{1, 5, 7}))
	})

	t.Run("non-zero missingNext blocks promote", func(t *testing.T) {
		d := base()
		d.missingNext = missingNextPtc
		assert.Equal(t, false, d.canPromote(10, []primitives.ValidatorIndex{1, 5, 7}))
	})

	t.Run("drift: added index blocks promote", func(t *testing.T) {
		d := base()
		assert.Equal(t, false, d.canPromote(10, []primitives.ValidatorIndex{1, 5, 7, 9}))
	})

	t.Run("drift: removed index blocks promote", func(t *testing.T) {
		d := base()
		assert.Equal(t, false, d.canPromote(10, []primitives.ValidatorIndex{1, 7}))
	})

	t.Run("drift: substituted index blocks promote", func(t *testing.T) {
		d := base()
		assert.Equal(t, false, d.canPromote(10, []primitives.ValidatorIndex{1, 5, 8}))
	})

	t.Run("nil current indices treated as empty: blocks promote when stored is non-empty", func(t *testing.T) {
		d := base()
		assert.Equal(t, false, d.canPromote(10, nil))
	})

	t.Run("both empty index sets is a no-op promote", func(t *testing.T) {
		d := dutyStoreData{initialized: true, epoch: 9}
		assert.Equal(t, true, d.canPromote(10, nil))
	})
}

func TestDutyStore_needsNextRetry(t *testing.T) {
	var nilStore *dutyStore
	assert.Equal(t, false, nilStore.needsNextRetry())

	ds := &dutyStore{}
	assert.Equal(t, false, ds.needsNextRetry()) // uninitialized

	pk := pubkey{0x1}
	build := func(missing missingNextDuties, indices []primitives.ValidatorIndex) dutyStoreData {
		var d dutyStoreData
		d.setFromContainer(&ethpb.ValidatorDutiesContainer{
			CurrentEpochDuties: []*ethpb.ValidatorDuty{{PublicKey: pk[:], ValidatorIndex: 7}},
		})
		d.missingNext = missing
		d.indices = indices
		return d
	}

	ds.write(build(0, []primitives.ValidatorIndex{7}))
	assert.Equal(t, false, ds.needsNextRetry()) // nothing missing

	ds.write(build(missingNextPtc, nil))
	assert.Equal(t, false, ds.needsNextRetry()) // missing but no indices (combined path)

	ds.write(build(missingNextPtc, []primitives.ValidatorIndex{7}))
	assert.Equal(t, true, ds.needsNextRetry()) // missing + indices

	ds.write(build(missingNextAttester, []primitives.ValidatorIndex{7}))
	assert.Equal(t, true, ds.needsNextRetry()) // attester is retried like any other missing type
}

func TestDutyStore_replaceNextDuties_revisionGuard(t *testing.T) {
	pk := pubkey{0x2}
	ds := &dutyStore{}

	// Uninitialized store: nothing to replace.
	assert.Equal(t, false, ds.replaceNextDuties(0, nil, 0, nil))

	var d dutyStoreData
	d.setFromContainer(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{{PublicKey: pk[:], ValidatorIndex: 7}},
		NextEpochDuties:    []*ethpb.ValidatorDuty{{PublicKey: pk[:], ValidatorIndex: 7, AttesterSlot: 100}},
	})
	d.epoch = 5
	d.indices = []primitives.ValidatorIndex{7}
	d.missingNext = missingNextPtc
	ds.write(d)

	staleRev := ds.snapshot().revision
	ds.write(d) // a concurrent write advances the revision under the stale reader

	newNext := []*ethpb.ValidatorDuty{{PublicKey: pk[:], ValidatorIndex: 7, AttesterSlot: 200}}

	// Stale revision -> dropped, store left unchanged.
	assert.Equal(t, false, ds.replaceNextDuties(staleRev, newNext, 0, nil))
	for _, nd := range ds.snapshot().nextDuties() {
		assert.Equal(t, primitives.Slot(100), nd.AttesterSlot)
	}

	// Current revision -> applied.
	assert.Equal(t, true, ds.replaceNextDuties(ds.snapshot().revision, newNext, 0, nil))
	for _, nd := range ds.snapshot().nextDuties() {
		assert.Equal(t, primitives.Slot(200), nd.AttesterSlot)
	}
	assert.Equal(t, true, ds.snapshot().missingNext() == 0)
}
