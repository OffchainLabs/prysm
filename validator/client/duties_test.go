package client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	validatormock "github.com/OffchainLabs/prysm/v7/testing/validator-mock"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestUpdateDuties_DoesNothingWhenNotEpochStart_AlreadyExistingAssignments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	v := validator{
		km:              newMockKeymanager(t, randKeypair(t)),
		validatorClient: client,
		duties: func() *dutyStore {
			ds := testDutyStore(&ethpb.ValidatorDuty{AttesterSlot: 10, CommitteeIndex: 20})
			ds.nextDuties[pubkey{}] = &ethpb.ValidatorDuty{AttesterSlot: 10, CommitteeIndex: 20}
			return ds
		}(),
	}
	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Times(1)

	assert.NoError(t, v.UpdateDuties(t.Context()), "Could not update assignments")
}

func TestUpdateDuties_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	v := validator{
		validatorClient: client,
		km:              newMockKeymanager(t, randKeypair(t)),
		duties:          testDutyStore(&ethpb.ValidatorDuty{CommitteeIndex: 1}),
	}

	expected := errors.New("bad")

	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Return(nil, expected)

	assert.ErrorContains(t, expected.Error(), v.UpdateDuties(t.Context()))
	assert.Equal(t, false, v.duties.IsInitialized(), "Assignments should have been cleared on failure")
}

func TestUpdateDuties_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	resp := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{
				AttesterSlot:    params.BeaconConfig().SlotsPerEpoch,
				ValidatorIndex:  200,
				CommitteeIndex:  100,
				CommitteeLength: 4,
				PublicKey:       []byte("testPubKey_1"),
				ProposerSlots:   []primitives.Slot{params.BeaconConfig().SlotsPerEpoch + 1},
			},
		},
	}
	v := validator{
		km:              newMockKeymanager(t, randKeypair(t)),
		validatorClient: client,
		duties:          &dutyStore{},
	}
	v.aggSelector = testLocalSelector(t, &v)
	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Return(resp, nil)

	var wg sync.WaitGroup
	wg.Add(1)

	client.EXPECT().SubscribeCommitteeSubnets(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*emptypb.Empty, error) {
		wg.Done()
		return nil, nil
	})

	require.NoError(t, v.UpdateDuties(t.Context()), "Could not update assignments")

	util.WaitTimeout(&wg, 2*time.Second)

	duties := v.duties.CurrentEpochDuties()
	require.Equal(t, 1, len(duties), "Expected one duty")
	var gotDuty *ethpb.ValidatorDuty
	for _, d := range duties {
		gotDuty = d
	}
	assert.Equal(t, params.BeaconConfig().SlotsPerEpoch+1, gotDuty.ProposerSlots[0], "Unexpected validator assignments")
	assert.Equal(t, params.BeaconConfig().SlotsPerEpoch, gotDuty.AttesterSlot, "Unexpected validator assignments")
	assert.Equal(t, resp.CurrentEpochDuties[0].CommitteeIndex, gotDuty.CommitteeIndex, "Unexpected validator assignments")
	assert.Equal(t, resp.CurrentEpochDuties[0].ValidatorIndex, gotDuty.ValidatorIndex, "Unexpected validator assignments")
}

func TestUpdateDuties_OK_FilterBlacklistedPublicKeys(t *testing.T) {
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	numValidators := 10
	km := genMockKeymanager(t, numValidators)
	blacklistedPublicKeys := make(map[[fieldparams.BLSPubkeyLength]byte]bool)
	for _, k := range km.keys {
		blacklistedPublicKeys[k] = true
	}
	v := validator{
		km:                 km,
		validatorClient:    client,
		blacklistedPubkeys: blacklistedPublicKeys,
		duties:             &dutyStore{},
	}
	v.aggSelector = testLocalSelector(t, &v)

	resp := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{},
	}
	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Return(resp, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	client.EXPECT().SubscribeCommitteeSubnets(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*emptypb.Empty, error) {
		wg.Done()
		return nil, nil
	})

	require.NoError(t, v.UpdateDuties(t.Context()), "Could not update assignments")

	util.WaitTimeout(&wg, 2*time.Second)

	for range blacklistedPublicKeys {
		assert.LogsContain(t, hook, "Not including slashable public key")
	}
}

func TestUpdateDuties_AllValidatorsExited(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	resp := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{
				AttesterSlot:    params.BeaconConfig().SlotsPerEpoch,
				ValidatorIndex:  200,
				CommitteeIndex:  100,
				CommitteeLength: 4,
				PublicKey:       []byte("testPubKey_1"),
				ProposerSlots:   []primitives.Slot{params.BeaconConfig().SlotsPerEpoch + 1},
				Status:          ethpb.ValidatorStatus_EXITED,
			},
			{
				AttesterSlot:    params.BeaconConfig().SlotsPerEpoch,
				ValidatorIndex:  201,
				CommitteeIndex:  101,
				CommitteeLength: 4,
				PublicKey:       []byte("testPubKey_2"),
				ProposerSlots:   []primitives.Slot{params.BeaconConfig().SlotsPerEpoch + 1},
				Status:          ethpb.ValidatorStatus_EXITED,
			},
		},
	}
	v := validator{
		km:              newMockKeymanager(t, randKeypair(t)),
		validatorClient: client,
		duties:          &dutyStore{},
	}
	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Return(resp, nil)

	err := v.UpdateDuties(t.Context())
	require.ErrorContains(t, ErrValidatorsAllExited.Error(), err)

}

func TestUpdateDuties_Distributed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	// Start of third epoch.
	slot := 2 * params.BeaconConfig().SlotsPerEpoch
	keys := randKeypair(t)
	resp := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{
				AttesterSlot:   slot, // First slot in epoch.
				ValidatorIndex: 200,
				CommitteeIndex: 100,
				PublicKey:      keys.pub[:],
				Status:         ethpb.ValidatorStatus_ACTIVE,
			},
		},
		NextEpochDuties: []*ethpb.ValidatorDuty{
			{
				AttesterSlot:   slot + params.BeaconConfig().SlotsPerEpoch, // First slot in next epoch.
				ValidatorIndex: 200,
				CommitteeIndex: 100,
				PublicKey:      keys.pub[:],
				Status:         ethpb.ValidatorStatus_ACTIVE,
			},
		},
	}

	secsPerSlot := params.BeaconConfig().SecondsPerSlot
	genesis := time.Now().Add(-time.Duration(uint64(slot)*secsPerSlot) * time.Second)

	v := validator{
		km:              newMockKeymanager(t, keys),
		validatorClient: client,
		distributed:     true,
		duties:          &dutyStore{},
		genesisTime:     genesis,
		pubkeyToStatus: map[[fieldparams.BLSPubkeyLength]byte]*validatorStatus{
			keys.pub: {
				publicKey: keys.pub[:],
				status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
				index:     200,
			},
		},
	}
	v.aggSelector = newDistributedSelector(&v)

	sigDomain := make([]byte, 32)

	client.EXPECT().Duties(
		gomock.Any(),
		gomock.Any(),
	).Return(resp, nil)

	client.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Return(
		&ethpb.DomainResponse{SignatureDomain: sigDomain},
		nil, /*err*/
	)

	client.EXPECT().AggregatedSelections(
		gomock.Any(),
		gomock.Any(),
	).Return(
		[]iface.BeaconCommitteeSelection{
			{
				SelectionProof: make([]byte, 32),
				Slot:           slot,
				ValidatorIndex: 200,
			},
		},
		nil,
	)

	var wg sync.WaitGroup
	wg.Add(1)

	client.EXPECT().SubscribeCommitteeSubnets(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*emptypb.Empty, error) {
		wg.Done()
		return nil, nil
	})

	require.NoError(t, v.UpdateDuties(t.Context()), "Could not update assignments")
	util.WaitTimeout(&wg, 2*time.Second)
	dvProvider, ok := v.aggSelector.(*distributedSelector)
	require.Equal(t, true, ok)
	require.Equal(t, 1, len(dvProvider.attSelections))
}

func TestValidator_CheckDependentRoots(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()
	client := validatormock.NewMockValidatorClient(ctrl)

	dutiesContainer := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{
				AttesterSlot:    params.BeaconConfig().SlotsPerEpoch,
				ValidatorIndex:  200,
				CommitteeIndex:  100,
				CommitteeLength: 4,
				PublicKey:       []byte("testPubKey_1"),
				ProposerSlots:   []primitives.Slot{params.BeaconConfig().SlotsPerEpoch + 1},
			},
		},
		PrevDependentRoot: bytesutil.PadTo([]byte{0x01, 0x02, 0x03}, fieldparams.RootLength),
		CurrDependentRoot: bytesutil.PadTo([]byte{0x04, 0x05, 0x06}, fieldparams.RootLength),
	}
	ds := &dutyStore{}
	ds.SetFromCombinedDutiesResponse(dutiesContainer)
	v := &validator{
		km:              newMockKeymanager(t, randKeypair(t)),
		validatorClient: client,
		duties:          ds,
	}
	v.aggSelector = testLocalSelector(t, v)

	t.Run("nil head event", func(t *testing.T) {
		err := v.checkDependentRoots(ctx, nil)
		require.ErrorContains(t, "received empty head event", err)
	})

	t.Run("invalid previous duty dependent root", func(t *testing.T) {
		head := &structs.HeadEvent{
			Slot:                      "0",
			PreviousDutyDependentRoot: "invalid_hex",
		}
		err := v.checkDependentRoots(ctx, head)
		require.ErrorContains(t, "failed to decode previous duty dependent root", err)
	})

	t.Run("invalid current duty dependent root", func(t *testing.T) {
		head := &structs.HeadEvent{
			Slot:                      "0",
			PreviousDutyDependentRoot: "0x0102030000000000000000000000000000000000000000000000000000000000",
			CurrentDutyDependentRoot:  "invalid_hex",
		}
		err := v.checkDependentRoots(ctx, head)
		require.ErrorContains(t, "failed to decode current duty dependent root", err)
	})

	t.Run("update duties for previous root mismatch", func(t *testing.T) {
		head := &structs.HeadEvent{
			Slot:                      "1",
			PreviousDutyDependentRoot: "0xe3f7a1b2c489d56f03a6b8d9c7e1fa2456bb09f3de42a67c8910fc3e7a5d4b12",
			CurrentDutyDependentRoot:  "0xe3f7a1b2c489d56f03a6b8d9c7e1fa2456bb09f3de42a67c8910fc3e7a5d4b12",
		}
		client.EXPECT().SubscribeCommitteeSubnets(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*emptypb.Empty, error) {
			return nil, nil
		}).AnyTimes()
		client.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(dutiesContainer, nil)
		err := v.checkDependentRoots(ctx, head)
		require.NoError(t, err)
	})

	t.Run("update duties for current root mismatch", func(t *testing.T) {
		head := &structs.HeadEvent{
			Slot:                      "1",
			PreviousDutyDependentRoot: "0x0102030000000000000000000000000000000000000000000000000000000000",
			CurrentDutyDependentRoot:  "0xe3f7a1b2c489d56f03a6b8d9c7e1fa2456bb09f3de42a67c8910fc3e7a5d4b12",
		}
		client.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(dutiesContainer, nil)
		var wg sync.WaitGroup
		wg.Add(1)

		client.EXPECT().SubscribeCommitteeSubnets(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
		).DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*emptypb.Empty, error) {
			wg.Done()
			return nil, nil
		}).AnyTimes()
		err := v.checkDependentRoots(ctx, head)
		require.NoError(t, err)
		util.WaitTimeout(&wg, 2*time.Second)
	})
	t.Run("no updates needed", func(t *testing.T) {
		head := &structs.HeadEvent{
			Slot:                      "0",
			PreviousDutyDependentRoot: "0x0102030000000000000000000000000000000000000000000000000000000000",
			CurrentDutyDependentRoot:  "0x0405060000000000000000000000000000000000000000000000000000000000",
		}
		curr, err := bytesutil.DecodeHexWithLength(head.CurrentDutyDependentRoot, fieldparams.RootLength)
		require.NoError(t, err)
		_, storedCurr := v.duties.DependentRoots()
		require.DeepEqual(t, curr, storedCurr)
		require.NoError(t, v.checkDependentRoots(ctx, head))
	})
}

func TestUpdateDutiesSplit(t *testing.T) {
	epoch := primitives.Epoch(5)

	setup := func(t *testing.T) (*validator, *validatormock.MockValidatorClient, keypair) {
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.AltairForkEpoch = 0
		cfg.FuluForkEpoch = 0
		cfg.GloasForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		ctrl := gomock.NewController(t)
		client := validatormock.NewMockValidatorClient(ctrl)
		keys := randKeypair(t)
		v := &validator{
			validatorClient: client,
			duties:          &dutyStore{},
			pubkeyToStatus: map[pubkey]*validatorStatus{
				keys.pub: {
					publicKey: keys.pub[:],
					status:    &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE},
					index:     42,
				},
			},
		}
		return v, client, keys
	}

	t.Run("OK", func(t *testing.T) {
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch

		client.EXPECT().AttesterDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: make([]byte, 32),
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch)*spe + 3, CommitteeIndex: 1, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch+1)*spe + 7, CommitteeIndex: 2, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(&ethpb.ProposerDutiesResponse{
			DependentRoot: make([]byte, 32),
			Duties:        []*ethpb.ProposerDutyV2{{Pubkey: keys.pub[:], ValidatorIndex: 42, Slot: primitives.Slot(epoch)*spe + 1}},
		}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{
			Duties: []*ethpb.SyncCommitteeDuty{{Pubkey: keys.pub[:], ValidatorIndex: 42}},
		}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil)
		client.EXPECT().PTCDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.PTCDutiesResponse{
			Duties: []*ethpb.PTCDuty{{Pubkey: keys.pub[:], ValidatorIndex: 42, Slot: primitives.Slot(epoch)*spe + 5}},
		}, nil)
		client.EXPECT().PTCDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.PTCDutiesResponse{
			Duties: []*ethpb.PTCDuty{{Pubkey: keys.pub[:], ValidatorIndex: 42, Slot: primitives.Slot(epoch+1)*spe + 2}},
		}, nil)

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, [][fieldparams.BLSPubkeyLength]byte{keys.pub}))

		// Current epoch: attester + proposer + sync + PTC.
		current := v.duties.CurrentEpochDuties()
		require.Equal(t, 1, len(current))
		for _, d := range current {
			assert.Equal(t, primitives.Slot(epoch)*spe+3, d.AttesterSlot)
			require.Equal(t, 1, len(d.ProposerSlots))
			assert.Equal(t, primitives.Slot(epoch)*spe+1, d.ProposerSlots[0])
			assert.Equal(t, true, d.IsSyncCommittee)
			require.Equal(t, 1, len(d.PtcSlots))
			assert.Equal(t, primitives.Slot(epoch)*spe+5, d.PtcSlots[0])
		}

		// Next epoch: attester + PTC look-ahead.
		next := v.duties.NextEpochDuties()
		require.Equal(t, 1, len(next))
		for _, d := range next {
			assert.Equal(t, primitives.Slot(epoch+1)*spe+7, d.AttesterSlot)
			require.Equal(t, 1, len(d.PtcSlots))
			assert.Equal(t, primitives.Slot(epoch+1)*spe+2, d.PtcSlots[0])
			assert.Equal(t, false, d.IsSyncCommittee)
		}

		// Duty store accessors.
		assert.DeepEqual(t, []primitives.Slot{primitives.Slot(epoch)*spe + 1}, v.duties.ProposerSlots(42))
		assert.DeepEqual(t, []primitives.Slot{primitives.Slot(epoch)*spe + 5}, v.duties.PtcSlots(42))
		assert.Equal(t, true, v.duties.IsSyncCommittee(42))
		assert.Equal(t, false, v.duties.IsNextSyncCommittee(42))
	})

	t.Run("attester error clears duties", func(t *testing.T) {
		v, client, keys := setup(t)

		client.EXPECT().AttesterDuties(gomock.Any(), epoch, gomock.Any()).Return(nil, errors.New("attester fail"))
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(nil, nil).AnyTimes()
		client.EXPECT().ProposerDuties(gomock.Any(), gomock.Any()).Return(&ethpb.ProposerDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).AnyTimes()

		err := v.updateDutiesSplit(t.Context(), epoch, [][fieldparams.BLSPubkeyLength]byte{keys.pub})
		require.ErrorContains(t, "attester fail", err)
		assert.Equal(t, false, v.duties.IsInitialized())
	})

	t.Run("proposer error clears duties", func(t *testing.T) {
		v, client, keys := setup(t)

		client.EXPECT().AttesterDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.AttesterDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(nil, errors.New("proposer fail"))
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(nil, nil).AnyTimes()
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).AnyTimes()

		err := v.updateDutiesSplit(t.Context(), epoch, [][fieldparams.BLSPubkeyLength]byte{keys.pub})
		require.ErrorContains(t, "proposer fail", err)
		assert.Equal(t, false, v.duties.IsInitialized())
	})

	t.Run("PTC error is non-fatal", func(t *testing.T) {
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch

		client.EXPECT().AttesterDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: make([]byte, 32),
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch)*spe + 3, CommitteeIndex: 1, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(&ethpb.ProposerDutiesResponse{DependentRoot: make([]byte, 32)}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().PTCDuties(gomock.Any(), epoch, gomock.Any()).Return(nil, errors.New("ptc fail"))
		client.EXPECT().PTCDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil)

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, [][fieldparams.BLSPubkeyLength]byte{keys.pub}))
		assert.Equal(t, true, v.duties.IsInitialized())
		assert.Equal(t, 0, len(v.duties.PtcSlots(42)))
	})

	t.Run("no known indices clears duties", func(t *testing.T) {
		v, _, keys := setup(t)
		v.pubkeyToStatus = map[pubkey]*validatorStatus{}

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, [][fieldparams.BLSPubkeyLength]byte{keys.pub}))
		assert.Equal(t, false, v.duties.IsInitialized())
	})
}
