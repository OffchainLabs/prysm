package client

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
)

func testDutyStore(current ...*ethpb.ValidatorDuty) *dutyStore {
	ds := &dutyStore{
		currentDuties:  make(map[pubkey]*ethpb.ValidatorDuty),
		nextDuties:     make(map[pubkey]*ethpb.ValidatorDuty),
		proposerSlots:  make(map[primitives.ValidatorIndex][]primitives.Slot),
		ptcSlots:       make(map[primitives.ValidatorIndex][]primitives.Slot),
		syncCurrentMap: make(map[primitives.ValidatorIndex]bool),
		syncNextMap:    make(map[primitives.ValidatorIndex]bool),
		initialized:    true,
	}
	for _, d := range current {
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
	return ds
}

func TestDutyStore_Uninitialized(t *testing.T) {
	ds := &dutyStore{}
	assert.Equal(t, false, ds.IsInitialized())
	assert.Equal(t, true, ds.CurrentEpochDuties() == nil)
	assert.Equal(t, true, ds.NextEpochDuties() == nil)

	prev, curr := ds.DependentRoots()
	assert.Equal(t, true, prev == nil)
	assert.Equal(t, true, curr == nil)

	d, ok := ds.CurrentDuty(pubkey{})
	assert.Equal(t, false, ok)
	assert.Equal(t, (*ethpb.ValidatorDuty)(nil), d)

	assert.Equal(t, true, ds.ProposerSlots(0) == nil)
	assert.Equal(t, true, ds.PtcSlots(0) == nil)
	assert.Equal(t, false, ds.IsSyncCommittee(0))
	assert.Equal(t, false, ds.IsNextSyncCommittee(0))
}

func TestDutyStore_ZeroValueIsNotInitialized(t *testing.T) {
	ds := &dutyStore{}
	assert.Equal(t, false, ds.IsInitialized())
}

func TestDutyStore_SetFromCombinedDutiesResponse(t *testing.T) {
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
	ds.SetFromCombinedDutiesResponse(container)

	assert.Equal(t, true, ds.IsInitialized())

	// Current duties.
	d, ok := ds.CurrentDuty(pk1)
	assert.Equal(t, true, ok)
	assert.Equal(t, primitives.ValidatorIndex(10), d.ValidatorIndex)

	_, ok = ds.CurrentDuty(pk2)
	assert.Equal(t, false, ok)

	// Next duties.
	next := ds.NextEpochDuties()
	assert.Equal(t, 1, len(next))
	assert.Equal(t, primitives.ValidatorIndex(20), next[pk2].ValidatorIndex)

	// Dependent roots.
	prev, curr := ds.DependentRoots()
	assert.DeepEqual(t, []byte("prev"), prev)
	assert.DeepEqual(t, []byte("curr"), curr)

	// Proposer slots.
	assert.DeepEqual(t, []primitives.Slot{3, 7}, ds.ProposerSlots(10))
	assert.Equal(t, true, ds.ProposerSlots(20) == nil)

	// PTC slots.
	assert.DeepEqual(t, []primitives.Slot{4, 6}, ds.PtcSlots(10))
	assert.Equal(t, true, ds.PtcSlots(20) == nil)

	// Sync committee.
	assert.Equal(t, true, ds.IsSyncCommittee(10))
	assert.Equal(t, false, ds.IsSyncCommittee(20))
	assert.Equal(t, false, ds.IsNextSyncCommittee(10))
	assert.Equal(t, true, ds.IsNextSyncCommittee(20))
}

func TestDutyStore_Reset(t *testing.T) {
	ds := testDutyStore(&ethpb.ValidatorDuty{PublicKey: make([]byte, 48)})
	ds.prevDependentRoot = []byte("prev")
	ds.currDependentRoot = []byte("curr")
	assert.Equal(t, true, ds.IsInitialized())

	ds.Reset()
	assert.Equal(t, false, ds.IsInitialized())
	assert.Equal(t, true, ds.CurrentEpochDuties() == nil)
}

func TestDutyStore_SetFromCombinedDutiesResponseNilResets(t *testing.T) {
	ds := testDutyStore(&ethpb.ValidatorDuty{PublicKey: make([]byte, 48)})
	assert.Equal(t, true, ds.IsInitialized())

	ds.SetFromCombinedDutiesResponse(nil)
	assert.Equal(t, false, ds.IsInitialized())
}

func TestDutyStore_SetFromCombinedDutiesResponseSkipsNilDuties(t *testing.T) {
	ds := &dutyStore{}
	ds.SetFromCombinedDutiesResponse(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{nil, {PublicKey: make([]byte, 48), ValidatorIndex: 1}},
		NextEpochDuties:    []*ethpb.ValidatorDuty{nil},
	})
	assert.Equal(t, 1, len(ds.CurrentEpochDuties()))
	assert.Equal(t, 0, len(ds.NextEpochDuties()))
}

func TestDutyStore_MergeDutiesResponse(t *testing.T) {
	pk1 := bytesutil.ToBytes48([]byte{1})
	pk2 := bytesutil.ToBytes48([]byte{2})
	pk3 := bytesutil.ToBytes48([]byte{3})

	// Start with an initialized store containing pk1.
	ds := testDutyStore(&ethpb.ValidatorDuty{
		PublicKey:      pk1[:],
		ValidatorIndex: 10,
		AttesterSlot:   5,
		ProposerSlots:  []primitives.Slot{3},
	})
	ds.nextDuties[pk1] = &ethpb.ValidatorDuty{
		PublicKey:      pk1[:],
		ValidatorIndex: 10,
		AttesterSlot:   40,
	}

	// Merge pk2 (current) and pk3 (next).
	ds.MergeDutiesResponse(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{
				PublicKey:       pk2[:],
				ValidatorIndex:  20,
				AttesterSlot:    6,
				ProposerSlots:   []primitives.Slot{7},
				IsSyncCommittee: true,
				PtcSlots:        []primitives.Slot{8},
			},
		},
		NextEpochDuties: []*ethpb.ValidatorDuty{
			{
				PublicKey:       pk3[:],
				ValidatorIndex:  30,
				AttesterSlot:    50,
				IsSyncCommittee: true,
			},
		},
	})

	// pk1 should be unchanged.
	d, ok := ds.CurrentDuty(pk1)
	assert.Equal(t, true, ok)
	assert.Equal(t, primitives.ValidatorIndex(10), d.ValidatorIndex)
	assert.DeepEqual(t, []primitives.Slot{3}, ds.ProposerSlots(10))
	next := ds.NextEpochDuties()
	assert.Equal(t, primitives.ValidatorIndex(10), next[pk1].ValidatorIndex)

	// pk2 should be merged into current.
	d, ok = ds.CurrentDuty(pk2)
	assert.Equal(t, true, ok)
	assert.Equal(t, primitives.ValidatorIndex(20), d.ValidatorIndex)
	assert.DeepEqual(t, []primitives.Slot{7}, ds.ProposerSlots(20))
	assert.Equal(t, true, ds.IsSyncCommittee(20))
	assert.DeepEqual(t, []primitives.Slot{8}, ds.PtcSlots(20))

	// pk3 should be merged into next.
	assert.Equal(t, primitives.ValidatorIndex(30), next[pk3].ValidatorIndex)
	assert.Equal(t, true, ds.IsNextSyncCommittee(30))
}

func TestDutyStore_MergeDutiesResponseNoopWhenUninitialized(t *testing.T) {
	ds := &dutyStore{}
	ds.MergeDutiesResponse(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{PublicKey: make([]byte, 48), ValidatorIndex: 1},
		},
	})
	assert.Equal(t, false, ds.IsInitialized())
}

func TestDutyStore_MergeDutiesResponseSkipsNil(t *testing.T) {
	ds := testDutyStore()
	ds.MergeDutiesResponse(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{nil},
		NextEpochDuties:    []*ethpb.ValidatorDuty{nil},
	})
	assert.Equal(t, 0, len(ds.CurrentEpochDuties()))
	assert.Equal(t, 0, len(ds.NextEpochDuties()))
}
