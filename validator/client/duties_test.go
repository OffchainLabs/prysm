package client

import (
	"errors"
	"testing"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
)

func TestFetchSyncDuties_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.EpochsPerSyncCommitteePeriod = 256
	params.OverrideBeaconConfig(cfg)

	epoch := primitives.Epoch(10)
	currentPeriod := uint64(epoch) / uint64(cfg.EpochsPerSyncCommitteePeriod)
	indices := []primitives.ValidatorIndex{1}

	// Pre-populate dutyStore with cached sync data.
	ds := &dutyStore{initialized: true}
	ds.syncCurrentResp = &ethpb.SyncCommitteeDutiesResponse{Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 1}}}
	ds.syncNextResp = &ethpb.SyncCommitteeDutiesResponse{}
	ds.syncPeriod = currentPeriod

	v := &validator{
		validatorClient: client,
		duties:          ds,
	}

	// No RPC calls expected — cache hit returns nils.
	period, current, next, err := v.fetchSyncDuties(t.Context(), epoch, indices)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), period)
	assert.Equal(t, (*ethpb.SyncCommitteeDutiesResponse)(nil), current)
	assert.Equal(t, (*ethpb.SyncCommitteeDutiesResponse)(nil), next)
}

func TestFetchSyncDuties_PreAltair(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 100
	params.OverrideBeaconConfig(cfg)

	v := &validator{
		validatorClient: client,
		duties:          &dutyStore{initialized: true},
	}

	// No RPC calls expected — pre-Altair returns zero values.
	period, current, next, err := v.fetchSyncDuties(t.Context(), 50, []primitives.ValidatorIndex{1})
	require.NoError(t, err)
	assert.Equal(t, uint64(0), period)
	assert.Equal(t, (*ethpb.SyncCommitteeDutiesResponse)(nil), current)
	assert.Equal(t, (*ethpb.SyncCommitteeDutiesResponse)(nil), next)
}

func TestUpdateDutiesSplit_SyncFailureNonFatal(t *testing.T) {
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.EpochsPerSyncCommitteePeriod = 256
	params.OverrideBeaconConfig(cfg)

	kp := randKeypair(t)
	epoch := primitives.Epoch(10)
	idx := primitives.ValidatorIndex(42)

	v := &validator{
		validatorClient: client,
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			kp.pub: {index: idx, status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}},
		},
		duties: &dutyStore{},
	}

	depRoot := make([]byte, 32)

	// Proposer succeeds.
	client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(
		&ethpb.ProposerDutiesResponse{DependentRoot: depRoot}, nil,
	)

	// Attester: no cache → straight to parallel fetch (current + next).
	client.EXPECT().AttesterDuties(gomock.Any(), epoch, gomock.Any()).Return(
		&ethpb.AttesterDutiesResponse{
			DependentRoot: depRoot,
			Duties:        []*ethpb.AttesterDuty{{ValidatorIndex: idx, Slot: 320}},
		}, nil,
	)
	client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(
		&ethpb.AttesterDutiesResponse{Duties: []*ethpb.AttesterDuty{{ValidatorIndex: idx, Slot: 352}}}, nil,
	)

	// Sync committee: current fails.
	client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch, gomock.Any()).Return(
		nil, errors.New("sync rpc failed"),
	)
	// Next epoch sync may or may not be called (parallel), allow it.
	client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch+1, gomock.Any()).Return(
		&ethpb.SyncCommitteeDutiesResponse{}, nil,
	).AnyTimes()

	filteredKeys := [][fieldparams.BLSPubkeyLength]byte{kp.pub}
	err := v.updateDutiesSplit(t.Context(), epoch, filteredKeys)

	// Should succeed despite sync failure.
	require.NoError(t, err)
	assert.Equal(t, true, v.duties.IsInitialized())
	assert.LogsContain(t, hook, "Error getting sync committee duties, reusing cached data")
}

func TestUpdateDutiesSplit_AttesterFailureFatal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.EpochsPerSyncCommitteePeriod = 256
	params.OverrideBeaconConfig(cfg)

	kp := randKeypair(t)
	epoch := primitives.Epoch(10)
	idx := primitives.ValidatorIndex(42)

	v := &validator{
		validatorClient: client,
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			kp.pub: {index: idx, status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}},
		},
		duties: &dutyStore{initialized: true},
	}

	// Proposer succeeds.
	client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(
		&ethpb.ProposerDutiesResponse{}, nil,
	)

	// Attester current epoch fails.
	client.EXPECT().AttesterDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		nil, errors.New("attester rpc failed"),
	).AnyTimes()

	// Sync may or may not be called (parallel).
	client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&ethpb.SyncCommitteeDutiesResponse{}, nil,
	).AnyTimes()

	filteredKeys := [][fieldparams.BLSPubkeyLength]byte{kp.pub}
	err := v.updateDutiesSplit(t.Context(), epoch, filteredKeys)

	require.ErrorContains(t, "attester rpc failed", err)
	// Duties should be cleared.
	assert.Equal(t, false, v.duties.IsInitialized())
}

func TestFetchProposerDuties_PostFulu(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 5
	params.OverrideBeaconConfig(cfg)

	epoch := primitives.Epoch(10)

	v := &validator{
		validatorClient: client,
		duties:          &dutyStore{},
	}

	currentResp := &ethpb.ProposerDutiesResponse{
		DependentRoot: make([]byte, 32),
		Duties:        []*ethpb.ProposerDutyV2{{ValidatorIndex: 1, Slot: 320}},
	}
	nextResp := &ethpb.ProposerDutiesResponse{
		DependentRoot: make([]byte, 32),
		Duties:        []*ethpb.ProposerDutyV2{{ValidatorIndex: 2, Slot: 352}},
	}

	client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(currentResp, nil)
	client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(nextResp, nil)

	current, next, err := v.fetchProposerDuties(t.Context(), epoch)
	require.NoError(t, err)
	assert.Equal(t, currentResp, current)
	assert.Equal(t, nextResp, next)
}

func TestFetchProposerDuties_PreFulu(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 100
	params.OverrideBeaconConfig(cfg)

	epoch := primitives.Epoch(10)

	v := &validator{
		validatorClient: client,
		duties:          &dutyStore{},
	}

	resp := &ethpb.ProposerDutiesResponse{
		DependentRoot: make([]byte, 32),
		Duties:        []*ethpb.ProposerDutyV2{{ValidatorIndex: 1, Slot: 320}},
	}

	// Only current epoch fetched, no next.
	client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(resp, nil)

	current, next, err := v.fetchProposerDuties(t.Context(), epoch)
	require.NoError(t, err)
	assert.Equal(t, resp, current)
	assert.Equal(t, (*ethpb.ProposerDutiesResponse)(nil), next)
}

func TestFetchProposerDuties_PostFulu_NextEpochFailureNonFatal(t *testing.T) {
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.FuluForkEpoch = 5
	params.OverrideBeaconConfig(cfg)

	epoch := primitives.Epoch(10)

	v := &validator{
		validatorClient: client,
		duties:          &dutyStore{},
	}

	currentResp := &ethpb.ProposerDutiesResponse{
		DependentRoot: make([]byte, 32),
		Duties:        []*ethpb.ProposerDutyV2{{ValidatorIndex: 1, Slot: 320}},
	}

	client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(currentResp, nil)
	client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(nil, errors.New("next epoch failed"))

	current, next, err := v.fetchProposerDuties(t.Context(), epoch)
	require.NoError(t, err)
	assert.Equal(t, currentResp, current)
	assert.Equal(t, (*ethpb.ProposerDutiesResponse)(nil), next)
	assert.LogsContain(t, hook, "Could not get next epoch proposer duties")
}

func TestFetchSyncDuties_CacheMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.EpochsPerSyncCommitteePeriod = 256
	params.OverrideBeaconConfig(cfg)

	epoch := primitives.Epoch(10)
	indices := []primitives.ValidatorIndex{1, 2}

	v := &validator{
		validatorClient: client,
		duties:          &dutyStore{},
	}

	currentResp := &ethpb.SyncCommitteeDutiesResponse{Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 1}}}
	nextResp := &ethpb.SyncCommitteeDutiesResponse{Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 2}}}

	client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch, indices).Return(currentResp, nil)
	client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch+1, indices).Return(nextResp, nil)

	period, current, next, err := v.fetchSyncDuties(t.Context(), epoch, indices)
	require.NoError(t, err)
	assert.Equal(t, currentResp, current)
	assert.Equal(t, nextResp, next)
	assert.Equal(t, uint64(0), period)
}

func TestFetchSyncDuties_NextEpochFailureNonFatal(t *testing.T) {
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.EpochsPerSyncCommitteePeriod = 256
	params.OverrideBeaconConfig(cfg)

	epoch := primitives.Epoch(10)
	indices := []primitives.ValidatorIndex{1}

	v := &validator{
		validatorClient: client,
		duties:          &dutyStore{},
	}

	currentResp := &ethpb.SyncCommitteeDutiesResponse{Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 1}}}
	client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch, indices).Return(currentResp, nil)
	client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch+1, indices).Return(nil, errors.New("next sync failed"))

	period, current, next, err := v.fetchSyncDuties(t.Context(), epoch, indices)
	require.NoError(t, err)
	assert.Equal(t, currentResp, current)
	assert.Equal(t, (*ethpb.SyncCommitteeDutiesResponse)(nil), next)
	assert.Equal(t, uint64(0), period)
	assert.LogsContain(t, hook, "Could not get next epoch sync committee duties")
}

func TestUpdateDutiesSplit_ProposerFailureFatal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.EpochsPerSyncCommitteePeriod = 256
	params.OverrideBeaconConfig(cfg)

	kp := randKeypair(t)
	epoch := primitives.Epoch(10)
	idx := primitives.ValidatorIndex(42)

	v := &validator{
		validatorClient: client,
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			kp.pub: {index: idx, status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}},
		},
		duties: &dutyStore{initialized: true},
	}

	// Proposer fails.
	client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(nil, errors.New("proposer rpc failed"))

	// Attester may or may not be called (parallel).
	client.EXPECT().AttesterDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&ethpb.AttesterDutiesResponse{DependentRoot: make([]byte, 32)}, nil,
	).AnyTimes()

	// Sync may or may not be called (parallel).
	client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&ethpb.SyncCommitteeDutiesResponse{}, nil,
	).AnyTimes()

	err := v.updateDutiesSplit(t.Context(), epoch, [][fieldparams.BLSPubkeyLength]byte{kp.pub})
	require.ErrorContains(t, "proposer rpc failed", err)
	assert.Equal(t, false, v.duties.IsInitialized())
}

func TestDependentRootChangeReason_NoChange(t *testing.T) {
	prevRoot := []byte("prev-root-xxxxxxxxxxxxxxxxxxxxxxx")
	currRoot := []byte("curr-root-xxxxxxxxxxxxxxxxxxxxxxx")

	v := &validator{
		duties: newDutyStoreFromLegacy(&ethpb.ValidatorDutiesContainer{
			PrevDependentRoot: prevRoot,
			CurrDependentRoot: currRoot,
		}),
	}
	assert.Equal(t, "", v.dependentRootChangeReason(prevRoot, currRoot))
}

func TestDependentRootChangeReason_PreviousChanged(t *testing.T) {
	prevRoot := []byte("prev-root-xxxxxxxxxxxxxxxxxxxxxxx")
	currRoot := []byte("curr-root-xxxxxxxxxxxxxxxxxxxxxxx")
	newPrev := []byte("new-prev-xxxxxxxxxxxxxxxxxxxxxxxx")

	v := &validator{
		duties: newDutyStoreFromLegacy(&ethpb.ValidatorDutiesContainer{
			PrevDependentRoot: prevRoot,
			CurrDependentRoot: currRoot,
		}),
	}
	assert.Equal(t, "previous", v.dependentRootChangeReason(newPrev, currRoot))
}

func TestDependentRootChangeReason_CurrentChanged(t *testing.T) {
	prevRoot := []byte("prev-root-xxxxxxxxxxxxxxxxxxxxxxx")
	currRoot := []byte("curr-root-xxxxxxxxxxxxxxxxxxxxxxx")
	newCurr := []byte("new-curr-xxxxxxxxxxxxxxxxxxxxxxxx")

	v := &validator{
		duties: newDutyStoreFromLegacy(&ethpb.ValidatorDutiesContainer{
			PrevDependentRoot: prevRoot,
			CurrDependentRoot: currRoot,
		}),
	}
	assert.Equal(t, "current", v.dependentRootChangeReason(prevRoot, newCurr))
}

func TestDependentRootChangeReason_Uninitialized(t *testing.T) {
	v := &validator{duties: &dutyStore{}}
	assert.Equal(t, "previous", v.dependentRootChangeReason([]byte("a"), []byte("b")))
}

func TestDependentRootChangeReason_ZeroCurrentRoot(t *testing.T) {
	prevRoot := []byte("prev-root-xxxxxxxxxxxxxxxxxxxxxxx")
	currRoot := []byte("curr-root-xxxxxxxxxxxxxxxxxxxxxxx")

	v := &validator{
		duties: newDutyStoreFromLegacy(&ethpb.ValidatorDutiesContainer{
			PrevDependentRoot: prevRoot,
			CurrDependentRoot: currRoot,
		}),
	}
	// Zero hash current root should return "" (no change).
	assert.Equal(t, "", v.dependentRootChangeReason(prevRoot, params.BeaconConfig().ZeroHash[:]))
}

func TestUpdateDutiesSplit_NoIndices(t *testing.T) {
	v := &validator{
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{},
		duties:         &dutyStore{},
	}
	// No matching indices → returns nil immediately without RPCs.
	kp := randKeypair(t)
	err := v.updateDutiesSplit(t.Context(), 10, [][fieldparams.BLSPubkeyLength]byte{kp.pub})
	require.NoError(t, err)
}

func TestUpdateDutiesLegacy_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	kp := randKeypair(t)
	epoch := primitives.Epoch(5)

	v := &validator{
		validatorClient: client,
		duties:          &dutyStore{},
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			kp.pub: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}},
		},
	}

	resp := &ethpb.ValidatorDutiesContainer{
		PrevDependentRoot: make([]byte, 32),
		CurrDependentRoot: make([]byte, 32),
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{
				PublicKey:      kp.pub[:],
				ValidatorIndex: 42,
				AttesterSlot:   160,
				CommitteeIndex: 1,
			},
		},
	}
	client.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(resp, nil)

	err := v.updateDutiesLegacy(t.Context(), epoch, [][fieldparams.BLSPubkeyLength]byte{kp.pub})
	require.NoError(t, err)
	assert.Equal(t, true, v.duties.IsInitialized())
	duties := v.duties.CurrentEpochDuties()
	require.Equal(t, 1, len(duties))
	require.NotNil(t, duties[kp.pub])
	assert.Equal(t, primitives.ValidatorIndex(42), duties[kp.pub].ValidatorIndex)
}

func TestUpdateDutiesLegacy_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	v := &validator{
		validatorClient: client,
		duties:          &dutyStore{initialized: true},
	}

	client.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(nil, errors.New("rpc failed"))

	err := v.updateDutiesLegacy(t.Context(), 5, [][fieldparams.BLSPubkeyLength]byte{})
	require.ErrorContains(t, "rpc failed", err)
	assert.Equal(t, false, v.duties.IsInitialized())
}

func TestUpdateDutiesLegacy_NilResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	v := &validator{
		validatorClient: client,
		duties:          &dutyStore{initialized: true},
	}

	client.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(nil, nil)

	err := v.updateDutiesLegacy(t.Context(), 5, [][fieldparams.BLSPubkeyLength]byte{})
	require.NoError(t, err)
	assert.Equal(t, false, v.duties.IsInitialized())
}

func TestOnDutiesUpdated_AllExited(t *testing.T) {
	kp := randKeypair(t)
	v := &validator{
		duties: newDutyStoreFromLegacy(&ethpb.ValidatorDutiesContainer{
			CurrentEpochDuties: []*ethpb.ValidatorDuty{
				{PublicKey: kp.pub[:], Status: ethpb.ValidatorStatus_EXITED},
			},
		}),
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			kp.pub: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_EXITED}},
		},
	}
	err := v.onDutiesUpdated(t.Context())
	require.ErrorIs(t, err, ErrValidatorsAllExited)
}

func TestOnDutiesUpdated_NotAllExited(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	kp := randKeypair(t)
	v := &validator{
		validatorClient: client,
		duties:          newDutyStoreFromLegacy(&ethpb.ValidatorDutiesContainer{}),
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			kp.pub: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_PENDING}},
		},
	}

	client.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	err := v.onDutiesUpdated(t.Context())
	require.NoError(t, err)
}

func TestClearDuties(t *testing.T) {
	v := &validator{
		duties: newDutyStoreFromLegacy(&ethpb.ValidatorDutiesContainer{
			CurrentEpochDuties: []*ethpb.ValidatorDuty{{ValidatorIndex: 1}},
		}),
	}
	assert.Equal(t, true, v.duties.IsInitialized())
	v.clearDuties()
	assert.Equal(t, false, v.duties.IsInitialized())
}

func TestClearDuties_NilStore(t *testing.T) {
	v := &validator{}
	v.clearDuties()
	assert.NotNil(t, v.duties)
	assert.Equal(t, false, v.duties.IsInitialized())
}
