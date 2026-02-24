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

func TestFetchAttesterDuties_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	depRoot := []byte("dependent-root-xxxxxxxxxxxxxxxx")
	indices := []primitives.ValidatorIndex{1, 2, 3}
	epoch := primitives.Epoch(10)

	cachedCurrent := &ethpb.AttesterDutiesResponse{DependentRoot: depRoot, Duties: []*ethpb.AttesterDuty{{ValidatorIndex: 1}}}
	cachedNext := &ethpb.AttesterDutiesResponse{Duties: []*ethpb.AttesterDuty{{ValidatorIndex: 1}}}

	v := &validator{
		validatorClient: client,
		duties: &dutyStore{
			attester: &attesterDutiesCacheEntry{
				current: cachedCurrent,
				next:    cachedNext,
				epoch:   epoch,
			},
			initialized: true,
		},
	}

	// Probe returns matching dependent root → cache hit.
	client.EXPECT().AttesterDuties(gomock.Any(), epoch, indices[:1]).Return(
		&ethpb.AttesterDutiesResponse{DependentRoot: depRoot}, nil,
	)

	result, err := v.fetchAttesterDuties(t.Context(), epoch, indices)
	require.NoError(t, err)
	assert.Equal(t, cachedCurrent, result.current)
	assert.Equal(t, cachedNext, result.next)
	assert.Equal(t, epoch, result.epoch)
}

func TestFetchAttesterDuties_CacheMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	oldRoot := []byte("old-root-xxxxxxxxxxxxxxxxxxxxxxx")
	newRoot := []byte("new-root-xxxxxxxxxxxxxxxxxxxxxxx")
	indices := []primitives.ValidatorIndex{1, 2}
	epoch := primitives.Epoch(10)

	v := &validator{
		validatorClient: client,
		duties: &dutyStore{
			attester: &attesterDutiesCacheEntry{
				current: &ethpb.AttesterDutiesResponse{DependentRoot: oldRoot},
				epoch:   epoch,
			},
			initialized: true,
		},
	}

	// Probe returns different dependent root → cache miss.
	client.EXPECT().AttesterDuties(gomock.Any(), epoch, indices[:1]).Return(
		&ethpb.AttesterDutiesResponse{DependentRoot: newRoot}, nil,
	)

	currentResp := &ethpb.AttesterDutiesResponse{DependentRoot: newRoot, Duties: []*ethpb.AttesterDuty{{ValidatorIndex: 1}}}
	nextResp := &ethpb.AttesterDutiesResponse{Duties: []*ethpb.AttesterDuty{{ValidatorIndex: 2}}}

	// Full fetch: current + next epoch.
	client.EXPECT().AttesterDuties(gomock.Any(), epoch, indices).Return(currentResp, nil)
	client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, indices).Return(nextResp, nil)

	result, err := v.fetchAttesterDuties(t.Context(), epoch, indices)
	require.NoError(t, err)
	assert.Equal(t, currentResp, result.current)
	assert.Equal(t, nextResp, result.next)
	assert.Equal(t, epoch, result.epoch)
}

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

	cachedEntry := &syncDutiesCacheEntry{
		current: &ethpb.SyncCommitteeDutiesResponse{Duties: []*ethpb.SyncCommitteeDuty{{ValidatorIndex: 1}}},
		next:    &ethpb.SyncCommitteeDutiesResponse{},
		epoch:   epoch,
		period:  currentPeriod,
	}

	v := &validator{
		validatorClient: client,
		duties: &dutyStore{
			sync:        cachedEntry,
			initialized: true,
		},
	}

	// No RPC calls expected — cache hit.
	result, err := v.fetchSyncDuties(t.Context(), epoch, indices)
	require.NoError(t, err)
	assert.Equal(t, cachedEntry, result)
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

	// No RPC calls expected — pre-Altair returns nil, nil.
	result, err := v.fetchSyncDuties(t.Context(), 50, []primitives.ValidatorIndex{1})
	require.NoError(t, err)
	assert.Equal(t, (*syncDutiesCacheEntry)(nil), result)
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
