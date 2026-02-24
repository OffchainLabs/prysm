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
		ds.SetLegacy(&ethpb.ValidatorDutiesContainer{}, nil)
		assert.Equal(t, true, ds.IsInitialized())
	})
	t.Run("split initialized", func(t *testing.T) {
		ds := &dutyStore{}
		ds.SetSplit(
			&attesterDutiesCacheEntry{current: &ethpb.AttesterDutiesResponse{}},
			nil, nil, nil, nil,
		)
		assert.Equal(t, true, ds.IsInitialized())
	})
	t.Run("split without attester", func(t *testing.T) {
		ds := &dutyStore{}
		ds.SetSplit(nil, nil, nil, nil, nil)
		assert.Equal(t, false, ds.IsInitialized())
	})
}

func TestDutyStore_DependentRoots_Legacy(t *testing.T) {
	ds := &dutyStore{}
	ds.SetLegacy(&ethpb.ValidatorDutiesContainer{
		PrevDependentRoot: []byte("prev"),
		CurrDependentRoot: []byte("curr"),
	}, nil)
	prev, curr := ds.DependentRoots()
	assert.DeepEqual(t, []byte("prev"), prev)
	assert.DeepEqual(t, []byte("curr"), curr)
}

func TestDutyStore_DependentRoots_Split(t *testing.T) {
	ds := &dutyStore{}
	ds.SetSplit(
		&attesterDutiesCacheEntry{
			current: &ethpb.AttesterDutiesResponse{DependentRoot: []byte("att-root")},
		},
		&proposerDutiesCacheEntry{
			current: &ethpb.ProposerDutiesResponse{DependentRoot: []byte("prop-root")},
		},
		nil, nil, nil,
	)
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
	}, nil)
	duties := ds.CurrentEpochDuties()
	require.Equal(t, 2, len(duties))
	assert.Equal(t, primitives.ValidatorIndex(10), duties[0].ValidatorIndex)
	assert.Equal(t, primitives.Slot(5), duties[0].Slot)
	assert.Equal(t, true, duties[0].IsSyncCommittee)
	assert.DeepEqual(t, []primitives.Slot{3}, duties[0].ProposerSlots)
	assert.Equal(t, ethpb.ValidatorStatus_ACTIVE, duties[0].Status)

	assert.Equal(t, primitives.ValidatorIndex(20), duties[1].ValidatorIndex)
	assert.Equal(t, ethpb.ValidatorStatus_EXITING, duties[1].Status)
}

func TestDutyStore_CurrentEpochDuties_Split(t *testing.T) {
	pk1 := testPubkey(1)
	pk2 := testPubkey(2)
	ds := &dutyStore{}
	ds.SetSplit(
		&attesterDutiesCacheEntry{
			current: &ethpb.AttesterDutiesResponse{
				Duties: []*ethpb.AttesterDuty{
					{ValidatorIndex: 10, Slot: 5, CommitteeIndex: 1, CommitteeLength: 128},
					{ValidatorIndex: 20, Slot: 6, CommitteeIndex: 2},
				},
			},
		},
		&proposerDutiesCacheEntry{
			current: &ethpb.ProposerDutiesResponse{
				Duties: []*ethpb.ProposerDutyV2{
					{ValidatorIndex: 10, Slot: 3},
				},
			},
		},
		&syncDutiesCacheEntry{
			current: &ethpb.SyncCommitteeDutiesResponse{
				Duties: []*ethpb.SyncCommitteeDuty{
					{ValidatorIndex: 10},
				},
			},
		},
		map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{
			10: pk1,
			20: pk2,
		},
		map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			pk1: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}},
			pk2: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_EXITING}},
		},
	)

	duties := ds.CurrentEpochDuties()
	require.Equal(t, 2, len(duties))

	// Validator 10: attester + proposer + sync.
	assert.Equal(t, pk1, duties[0].Pubkey)
	assert.Equal(t, primitives.ValidatorIndex(10), duties[0].ValidatorIndex)
	assert.Equal(t, primitives.Slot(5), duties[0].Slot)
	assert.DeepEqual(t, []primitives.Slot{3}, duties[0].ProposerSlots)
	assert.Equal(t, true, duties[0].IsSyncCommittee)
	assert.Equal(t, ethpb.ValidatorStatus_ACTIVE, duties[0].Status)

	// Validator 20: attester only.
	assert.Equal(t, pk2, duties[1].Pubkey)
	assert.Equal(t, primitives.ValidatorIndex(20), duties[1].ValidatorIndex)
	assert.Equal(t, false, duties[1].IsSyncCommittee)
	assert.Equal(t, ethpb.ValidatorStatus_EXITING, duties[1].Status)
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
	}, nil)

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
	ds := &dutyStore{}
	ds.SetSplit(
		&attesterDutiesCacheEntry{
			current: &ethpb.AttesterDutiesResponse{
				Duties: []*ethpb.AttesterDuty{
					{ValidatorIndex: 10, Slot: 5},
				},
			},
		},
		nil, nil,
		map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{10: pk1},
		nil,
	)
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
	}, nil)
	assert.DeepEqual(t, []primitives.Slot{3, 7}, ds.ProposerSlots(10))
	assert.Equal(t, 0, len(ds.ProposerSlots(20)))
	assert.Equal(t, 0, len(ds.ProposerSlots(99)))
}

func TestDutyStore_ProposerSlots_Split(t *testing.T) {
	pk1 := testPubkey(1)
	ds := &dutyStore{}
	ds.SetSplit(
		&attesterDutiesCacheEntry{current: &ethpb.AttesterDutiesResponse{}},
		&proposerDutiesCacheEntry{
			current: &ethpb.ProposerDutiesResponse{
				Duties: []*ethpb.ProposerDutyV2{
					{ValidatorIndex: 10, Slot: 3},
				},
			},
		},
		nil,
		map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{10: pk1},
		nil,
	)
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
	}, nil)
	assert.Equal(t, true, ds.IsSyncCommittee(10))
	assert.Equal(t, false, ds.IsSyncCommittee(20))
	assert.Equal(t, false, ds.IsNextSyncCommittee(10))
	assert.Equal(t, true, ds.IsNextSyncCommittee(20))
}

func TestDutyStore_IsSyncCommittee_Split(t *testing.T) {
	ds := &dutyStore{}
	ds.SetSplit(
		&attesterDutiesCacheEntry{current: &ethpb.AttesterDutiesResponse{}},
		nil,
		&syncDutiesCacheEntry{
			current: &ethpb.SyncCommitteeDutiesResponse{
				Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 10}},
			},
			next: &ethpb.SyncCommitteeDutiesResponse{
				Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 20}},
			},
		},
		nil, nil,
	)
	assert.Equal(t, true, ds.IsSyncCommittee(10))
	assert.Equal(t, false, ds.IsSyncCommittee(20))
	assert.Equal(t, false, ds.IsNextSyncCommittee(10))
	assert.Equal(t, true, ds.IsNextSyncCommittee(20))
}

func TestDutyStore_AllCurrentExitedCount(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		pk1 := testPubkey(1)
		pk2 := testPubkey(2)
		pk3 := testPubkey(3)
		ds := &dutyStore{}
		ds.SetLegacy(&ethpb.ValidatorDutiesContainer{
			CurrentEpochDuties: []*ethpb.ValidatorDuty{
				{PublicKey: pk1[:], Status: ethpb.ValidatorStatus_EXITED},
				{PublicKey: pk2[:], Status: ethpb.ValidatorStatus_EXITED},
				{PublicKey: pk3[:], Status: ethpb.ValidatorStatus_ACTIVE},
			},
		}, nil)
		exited, total := ds.AllCurrentExitedCount()
		assert.Equal(t, 2, exited)
		assert.Equal(t, 3, total)
	})
	t.Run("split", func(t *testing.T) {
		pk1 := testPubkey(1)
		pk2 := testPubkey(2)
		ds := &dutyStore{}
		ds.SetSplit(
			&attesterDutiesCacheEntry{
				current: &ethpb.AttesterDutiesResponse{
					Duties: []*ethpb.AttesterDuty{
						{ValidatorIndex: 10},
						{ValidatorIndex: 20},
					},
				},
			},
			nil, nil,
			map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{10: pk1, 20: pk2},
			map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
				pk1: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_EXITED}},
				pk2: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}},
			},
		)
		exited, total := ds.AllCurrentExitedCount()
		assert.Equal(t, 1, exited)
		assert.Equal(t, 2, total)
	})
}

func TestDutyStore_NextEpochDuties_Split(t *testing.T) {
	t.Run("without next proposer", func(t *testing.T) {
		pk1 := testPubkey(1)
		ds := &dutyStore{}
		ds.SetSplit(
			&attesterDutiesCacheEntry{
				current: &ethpb.AttesterDutiesResponse{},
				next: &ethpb.AttesterDutiesResponse{
					Duties: []*ethpb.AttesterDuty{
						{ValidatorIndex: 10, Slot: 40},
					},
				},
			},
			nil,
			&syncDutiesCacheEntry{
				next: &ethpb.SyncCommitteeDutiesResponse{
					Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 10}},
				},
			},
			map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{10: pk1},
			nil,
		)
		duties := ds.NextEpochDuties()
		require.Equal(t, 1, len(duties))
		assert.Equal(t, primitives.Slot(40), duties[0].Slot)
		assert.Equal(t, true, duties[0].IsSyncCommittee)
		assert.Equal(t, 0, len(duties[0].ProposerSlots))
	})

	t.Run("with next proposer (post-Fulu)", func(t *testing.T) {
		pk1 := testPubkey(1)
		ds := &dutyStore{}
		ds.SetSplit(
			&attesterDutiesCacheEntry{
				current: &ethpb.AttesterDutiesResponse{},
				next: &ethpb.AttesterDutiesResponse{
					Duties: []*ethpb.AttesterDuty{
						{ValidatorIndex: 10, Slot: 40},
					},
				},
			},
			&proposerDutiesCacheEntry{
				current: &ethpb.ProposerDutiesResponse{},
				next: &ethpb.ProposerDutiesResponse{
					Duties: []*ethpb.ProposerDutyV2{
						{ValidatorIndex: 10, Slot: 42},
					},
				},
			},
			nil,
			map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{10: pk1},
			nil,
		)
		duties := ds.NextEpochDuties()
		require.Equal(t, 1, len(duties))
		assert.Equal(t, primitives.Slot(40), duties[0].Slot)
		assert.DeepEqual(t, []primitives.Slot{42}, duties[0].ProposerSlots)
	})
}

func TestDutyStore_SetLegacy_ClearsSplit(t *testing.T) {
	ds := &dutyStore{}
	ds.SetSplit(
		&attesterDutiesCacheEntry{current: &ethpb.AttesterDutiesResponse{}},
		&proposerDutiesCacheEntry{},
		&syncDutiesCacheEntry{},
		nil, nil,
	)
	assert.Equal(t, true, ds.IsInitialized())
	assert.NotNil(t, ds.AttesterDutiesCache())

	ds.SetLegacy(&ethpb.ValidatorDutiesContainer{}, nil)
	assert.Equal(t, true, ds.IsInitialized())
	assert.Equal(t, (*attesterDutiesCacheEntry)(nil), ds.AttesterDutiesCache())
	assert.Equal(t, (*proposerDutiesCacheEntry)(nil), ds.ProposerDutiesCache())
	assert.Equal(t, (*syncDutiesCacheEntry)(nil), ds.SyncDutiesCache())
}

func TestDutyStore_SetSplit_ClearsLegacy(t *testing.T) {
	ds := &dutyStore{}
	pk := testPubkey(1)
	ds.SetLegacy(&ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{{PublicKey: pk[:]}},
	}, nil)
	assert.Equal(t, true, ds.IsInitialized())
	assert.Equal(t, 1, len(ds.CurrentEpochDuties()))

	ds.SetSplit(
		&attesterDutiesCacheEntry{current: &ethpb.AttesterDutiesResponse{}},
		nil, nil, nil, nil,
	)
	assert.Equal(t, true, ds.IsInitialized())
	assert.Equal(t, 0, len(ds.CurrentEpochDuties()))
	assert.NotNil(t, ds.AttesterDutiesCache())
}

func TestDutyStore_ProposerOnlyInSplit(t *testing.T) {
	pk1 := testPubkey(1)
	pk2 := testPubkey(2)
	ds := &dutyStore{}
	ds.SetSplit(
		&attesterDutiesCacheEntry{
			current: &ethpb.AttesterDutiesResponse{
				Duties: []*ethpb.AttesterDuty{
					{ValidatorIndex: 10, Slot: 5},
				},
			},
		},
		&proposerDutiesCacheEntry{
			current: &ethpb.ProposerDutiesResponse{
				Duties: []*ethpb.ProposerDutyV2{
					{ValidatorIndex: 20, Slot: 3}, // proposer-only validator
				},
			},
		},
		nil,
		map[primitives.ValidatorIndex][fieldparams.BLSPubkeyLength]byte{10: pk1, 20: pk2},
		nil,
	)
	duties := ds.CurrentEpochDuties()
	require.Equal(t, 2, len(duties))

	// Check the proposer-only duty exists.
	found := false
	for _, d := range duties {
		if d.ValidatorIndex == 20 {
			found = true
			assert.DeepEqual(t, []primitives.Slot{3}, d.ProposerSlots)
		}
	}
	assert.Equal(t, true, found)
}
