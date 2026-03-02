package client

import (
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestDutyStore_IsInitialized(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		var ds *dutyStore
		assert.Equal(t, false, ds.IsInitialized())
	})
	t.Run("empty store", func(t *testing.T) {
		ds := &dutyStore{}
		assert.Equal(t, false, ds.IsInitialized())
	})
	t.Run("legacy initialized", func(t *testing.T) {
		ds := &dutyStore{}
		ds.SetLegacy(&ethpb.ValidatorDutiesContainer{})
		assert.Equal(t, true, ds.IsInitialized())
	})
	t.Run("initialized", func(t *testing.T) {
		ds := &dutyStore{initialized: true}
		assert.Equal(t, true, ds.IsInitialized())
	})
}

func TestDutyStore_DependentRoots_Legacy(t *testing.T) {
	ds := &dutyStore{}
	ds.SetLegacy(&ethpb.ValidatorDutiesContainer{
		PrevDependentRoot: []byte("prev"),
		CurrDependentRoot: []byte("curr"),
	})
	prev, curr := ds.DependentRoots()
	assert.DeepEqual(t, []byte("prev"), prev)
	assert.DeepEqual(t, []byte("curr"), curr)
}

func TestDutyStore_DependentRoots_Split(t *testing.T) {
	ds := &dutyStore{
		attesterDependentRoot: []byte("att-root"),
		proposerDependentRoot: []byte("prop-root"),
		initialized:           true,
	}
	prev, curr := ds.DependentRoots()
	assert.DeepEqual(t, []byte("att-root"), prev)
	assert.DeepEqual(t, []byte("prop-root"), curr)
}

func testPubkey(b byte) [fieldparams.BLSPubkeyLength]byte {
	var pk [fieldparams.BLSPubkeyLength]byte
	pk[0] = b
	return pk
}

func TestDutyStore_CurrentEpochDuties_Legacy(t *testing.T) {
	pk1 := testPubkey(1)
	pk2 := testPubkey(2)
	ds := &dutyStore{}
	ds.SetLegacy(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{
				PublicKey:       pk1[:],
				ValidatorIndex:  10,
				AttesterSlot:    5,
				CommitteeIndex:  1,
				ProposerSlots:   []primitives.Slot{3},
				Status:          ethpb.ValidatorStatus_ACTIVE,
				IsSyncCommittee: true,
			},
			{
				PublicKey:      pk2[:],
				ValidatorIndex: 20,
				AttesterSlot:   6,
				Status:         ethpb.ValidatorStatus_EXITING,
			},
		},
	})
	duties := ds.CurrentEpochDuties()
	require.Equal(t, 2, len(duties))
	require.NotNil(t, duties[pk1])
	assert.Equal(t, primitives.ValidatorIndex(10), duties[pk1].ValidatorIndex)
	assert.Equal(t, primitives.Slot(5), duties[pk1].Slot)
	assert.Equal(t, true, ds.IsSyncCommittee(10))
	assert.DeepEqual(t, []primitives.Slot{3}, ds.ProposerSlots(10))

	require.NotNil(t, duties[pk2])
	assert.Equal(t, primitives.ValidatorIndex(20), duties[pk2].ValidatorIndex)
}

func TestDutyStore_CurrentEpochDuties_Split(t *testing.T) {
	pk1 := testPubkey(1)
	pk2 := testPubkey(2)
	ds := &dutyStore{
		currentDuties: attesterMap(&ethpb.AttesterDutiesResponse{
			Duties: []*ethpb.AttesterDuty{
				{Pubkey: pk1[:], ValidatorIndex: 10, Slot: 5, CommitteeIndex: 1, CommitteeLength: 128},
				{Pubkey: pk2[:], ValidatorIndex: 20, Slot: 6, CommitteeIndex: 2},
			},
		}),
		proposerSlots:  proposerSlotsMap(&ethpb.ProposerDutiesResponse{Duties: []*ethpb.ProposerDutyV2{{ValidatorIndex: 10, Slot: 3}}}),
		syncCurrentMap: syncMap(&ethpb.SyncCommitteeDutiesResponse{Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 10}}}),
		syncNextMap:    syncMap(nil),
		initialized:    true,
	}

	duties := ds.CurrentEpochDuties()
	require.Equal(t, 2, len(duties))

	// Validator 10: attester + proposer + sync.
	require.NotNil(t, duties[pk1])
	assert.Equal(t, primitives.ValidatorIndex(10), duties[pk1].ValidatorIndex)
	assert.Equal(t, primitives.Slot(5), duties[pk1].Slot)
	assert.DeepEqual(t, []primitives.Slot{3}, ds.ProposerSlots(10))
	assert.Equal(t, true, ds.IsSyncCommittee(10))

	// Validator 20: attester only.
	require.NotNil(t, duties[pk2])
	assert.Equal(t, primitives.ValidatorIndex(20), duties[pk2].ValidatorIndex)
	assert.Equal(t, false, ds.IsSyncCommittee(20))
}

func TestDutyStore_CurrentAttesterDuty_Legacy(t *testing.T) {
	pk1 := testPubkey(1)
	pk2 := testPubkey(2)
	ds := &dutyStore{}
	ds.SetLegacy(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{PublicKey: pk1[:], ValidatorIndex: 10, AttesterSlot: 5},
			{PublicKey: pk2[:], ValidatorIndex: 20, AttesterSlot: 6},
		},
	})

	duty, ok := ds.CurrentAttesterDuty(pk1)
	require.Equal(t, true, ok)
	assert.Equal(t, primitives.ValidatorIndex(10), duty.ValidatorIndex)

	duty, ok = ds.CurrentAttesterDuty(pk2)
	require.Equal(t, true, ok)
	assert.Equal(t, primitives.ValidatorIndex(20), duty.ValidatorIndex)

	_, ok = ds.CurrentAttesterDuty(testPubkey(99))
	assert.Equal(t, false, ok)
}

func TestDutyStore_CurrentAttesterDuty_Split(t *testing.T) {
	pk1 := testPubkey(1)
	ds := &dutyStore{
		currentDuties: attesterMap(&ethpb.AttesterDutiesResponse{
			Duties: []*ethpb.AttesterDuty{
				{Pubkey: pk1[:], ValidatorIndex: 10, Slot: 5},
			},
		}),
		initialized: true,
	}
	duty, ok := ds.CurrentAttesterDuty(pk1)
	require.Equal(t, true, ok)
	assert.Equal(t, primitives.ValidatorIndex(10), duty.ValidatorIndex)

	_, ok = ds.CurrentAttesterDuty(testPubkey(99))
	assert.Equal(t, false, ok)
}

func TestDutyStore_ProposerSlots_Legacy(t *testing.T) {
	ds := &dutyStore{}
	ds.SetLegacy(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{ValidatorIndex: 10, ProposerSlots: []primitives.Slot{3, 7}},
			{ValidatorIndex: 20},
		},
	})
	assert.DeepEqual(t, []primitives.Slot{3, 7}, ds.ProposerSlots(10))
	assert.Equal(t, 0, len(ds.ProposerSlots(20)))
	assert.Equal(t, 0, len(ds.ProposerSlots(99)))
}

func TestDutyStore_ProposerSlots_Split(t *testing.T) {
	ds := &dutyStore{
		proposerSlots: proposerSlotsMap(&ethpb.ProposerDutiesResponse{
			Duties: []*ethpb.ProposerDutyV2{{ValidatorIndex: 10, Slot: 3}},
		}),
		initialized: true,
	}
	assert.DeepEqual(t, []primitives.Slot{3}, ds.ProposerSlots(10))
	assert.Equal(t, 0, len(ds.ProposerSlots(99)))
}

func TestDutyStore_IsSyncCommittee_Legacy(t *testing.T) {
	ds := &dutyStore{}
	ds.SetLegacy(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{ValidatorIndex: 10, IsSyncCommittee: true},
			{ValidatorIndex: 20, IsSyncCommittee: false},
		},
		NextEpochDuties: []*ethpb.ValidatorDuty{
			{ValidatorIndex: 10, IsSyncCommittee: false},
			{ValidatorIndex: 20, IsSyncCommittee: true},
		},
	})
	assert.Equal(t, true, ds.IsSyncCommittee(10))
	assert.Equal(t, false, ds.IsSyncCommittee(20))
	assert.Equal(t, false, ds.IsNextSyncCommittee(10))
	assert.Equal(t, true, ds.IsNextSyncCommittee(20))
}

func TestDutyStore_IsSyncCommittee_Split(t *testing.T) {
	ds := &dutyStore{
		syncCurrentMap: syncMap(&ethpb.SyncCommitteeDutiesResponse{
			Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 10}},
		}),
		syncNextMap: syncMap(&ethpb.SyncCommitteeDutiesResponse{
			Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 20}},
		}),
		initialized: true,
	}
	assert.Equal(t, true, ds.IsSyncCommittee(10))
	assert.Equal(t, false, ds.IsSyncCommittee(20))
	assert.Equal(t, false, ds.IsNextSyncCommittee(10))
	assert.Equal(t, true, ds.IsNextSyncCommittee(20))
}

func TestDutyStore_NextEpochDuties_Split(t *testing.T) {
	t.Run("without next proposer", func(t *testing.T) {
		pk := testPubkey(10)
		ds := &dutyStore{
			currentDuties: attesterMap(&ethpb.AttesterDutiesResponse{}),
			nextDuties: attesterMap(&ethpb.AttesterDutiesResponse{
				Duties: []*ethpb.AttesterDuty{
					{Pubkey: pk[:], ValidatorIndex: 10, Slot: 40},
				},
			}),
			syncNextMap: syncMap(&ethpb.SyncCommitteeDutiesResponse{
				Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 10}},
			}),
			initialized: true,
		}
		duties := ds.NextEpochDuties()
		require.Equal(t, 1, len(duties))
		require.NotNil(t, duties[pk])
		assert.Equal(t, primitives.Slot(40), duties[pk].Slot)
		assert.Equal(t, true, ds.IsNextSyncCommittee(10))
	})

	t.Run("with next proposer (post-Fulu)", func(t *testing.T) {
		propSlots := proposerSlotsMap(&ethpb.ProposerDutiesResponse{})
		for _, d := range []*ethpb.ProposerDutyV2{{ValidatorIndex: 10, Slot: 42}} {
			propSlots[d.ValidatorIndex] = append(propSlots[d.ValidatorIndex], d.Slot)
		}
		ds := &dutyStore{
			proposerSlots: propSlots,
			initialized:   true,
		}
		assert.DeepEqual(t, []primitives.Slot{42}, ds.ProposerSlots(10))
	})
}

func TestDutyStore_SetLegacy_OverwritesExisting(t *testing.T) {
	ds := &dutyStore{
		currentDuties: attesterMap(&ethpb.AttesterDutiesResponse{}),
		initialized:   true,
	}
	assert.Equal(t, true, ds.IsInitialized())

	ds.SetLegacy(&ethpb.ValidatorDutiesContainer{})
	assert.Equal(t, true, ds.IsInitialized())
}

func TestDutyStore_ProposerOnlyInSplit(t *testing.T) {
	pk := testPubkey(10)
	ds := &dutyStore{
		currentDuties: attesterMap(&ethpb.AttesterDutiesResponse{
			Duties: []*ethpb.AttesterDuty{
				{Pubkey: pk[:], ValidatorIndex: 10, Slot: 5},
			},
		}),
		proposerSlots: proposerSlotsMap(&ethpb.ProposerDutiesResponse{
			Duties: []*ethpb.ProposerDutyV2{
				{ValidatorIndex: 20, Slot: 3}, // proposer-only validator
			},
		}),
		initialized: true,
	}
	duties := ds.CurrentEpochDuties()
	require.Equal(t, 1, len(duties))
	require.NotNil(t, duties[pk])
	assert.Equal(t, primitives.ValidatorIndex(10), duties[pk].ValidatorIndex)

	// Proposer-only validator 20 is not in attester list, but has proposer slots.
	assert.DeepEqual(t, []primitives.Slot{3}, ds.ProposerSlots(20))
}

func TestDutyStore_SyncCacheValidity(t *testing.T) {
	ds := &dutyStore{
		syncCurrentResp: &ethpb.SyncCommitteeDutiesResponse{},
		syncPeriod:      5,
		initialized:     true,
	}

	assert.Equal(t, true, ds.SyncCacheValid(5))
	assert.Equal(t, false, ds.SyncCacheValid(6))
}
