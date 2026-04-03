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

func TestUpdateDuties_PartialNewKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	existingPair := randKeypair(t)
	newPair := randKeypair(t)

	existingResp := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{
				PublicKey:      existingPair.pub[:],
				ValidatorIndex: 10,
				AttesterSlot:   5,
				ProposerSlots:  []primitives.Slot{3},
			},
		},
	}
	newKeyResp := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{
				PublicKey:      newPair.pub[:],
				ValidatorIndex: 20,
				AttesterSlot:   6,
				ProposerSlots:  []primitives.Slot{7},
			},
		},
	}

	km := newMockKeymanager(t, existingPair)
	v := validator{
		km:              km,
		validatorClient: client,
		duties:          &dutyStore{},
	}
	v.aggSelector = testLocalSelector(t, &v)

	// First call: full refresh sets up existing duties.
	var wg sync.WaitGroup
	wg.Add(1)
	client.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(existingResp, nil)
	client.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*emptypb.Empty, error) {
			wg.Done()
			return nil, nil
		})
	require.NoError(t, v.UpdateDuties(t.Context()))
	util.WaitTimeout(&wg, 2*time.Second)
	assert.Equal(t, 1, len(v.duties.CurrentEpochDuties()))
	assert.Equal(t, true, v.lastDutiesPubkeys[existingPair.pub])

	// Second call: partial refresh with newPair merges without replacing.
	wg.Add(1)
	client.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(newKeyResp, nil)
	client.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*emptypb.Empty, error) {
			wg.Done()
			return nil, nil
		})
	require.NoError(t, v.UpdateDuties(t.Context(), [][fieldparams.BLSPubkeyLength]byte{newPair.pub}))
	util.WaitTimeout(&wg, 2*time.Second)

	// Both validators should now have duties.
	assert.Equal(t, 2, len(v.duties.CurrentEpochDuties()))
	d, ok := v.duties.CurrentDuty(existingPair.pub)
	assert.Equal(t, true, ok)
	assert.Equal(t, primitives.ValidatorIndex(10), d.ValidatorIndex)
	d, ok = v.duties.CurrentDuty(newPair.pub)
	assert.Equal(t, true, ok)
	assert.Equal(t, primitives.ValidatorIndex(20), d.ValidatorIndex)
	assert.Equal(t, true, v.lastDutiesPubkeys[newPair.pub])
}

func TestUpdateDuties_PartialNewKeys_FilterBlacklisted(t *testing.T) {
	hook := logTest.NewGlobal()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	goodPair := randKeypair(t)
	badPair := randKeypair(t)

	km := newMockKeymanager(t, goodPair, badPair)
	v := validator{
		km:              km,
		validatorClient: client,
		duties:          testDutyStore(),
		lastDutiesPubkeys: map[[fieldparams.BLSPubkeyLength]byte]bool{
			goodPair.pub: true,
		},
		blacklistedPubkeys: map[[fieldparams.BLSPubkeyLength]byte]bool{
			badPair.pub: true,
		},
	}
	v.aggSelector = testLocalSelector(t, &v)

	resp := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{
			{PublicKey: goodPair.pub[:], ValidatorIndex: 10, AttesterSlot: 5},
		},
	}
	var wg sync.WaitGroup
	wg.Add(1)
	client.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(resp, nil)
	client.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ *ethpb.CommitteeSubnetsSubscribeRequest, _ []*ethpb.ValidatorDuty) (*emptypb.Empty, error) {
			wg.Done()
			return nil, nil
		})

	require.NoError(t, v.UpdateDuties(t.Context(), [][fieldparams.BLSPubkeyLength]byte{goodPair.pub, badPair.pub}))
	util.WaitTimeout(&wg, 2*time.Second)
	assert.LogsContain(t, hook, "Not including slashable public key")
}

func TestUpdateDuties_PartialError_DoesNotResetStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	pair := randKeypair(t)
	v := validator{
		km:              newMockKeymanager(t, pair),
		validatorClient: client,
		duties:          testDutyStore(&ethpb.ValidatorDuty{PublicKey: pair.pub[:], ValidatorIndex: 10}),
		lastDutiesPubkeys: map[[fieldparams.BLSPubkeyLength]byte]bool{
			pair.pub: true,
		},
	}

	newPair := randKeypair(t)
	client.EXPECT().Duties(gomock.Any(), gomock.Any()).Return(nil, errors.New("rpc error"))

	err := v.UpdateDuties(t.Context(), [][fieldparams.BLSPubkeyLength]byte{newPair.pub})
	require.ErrorContains(t, "rpc error", err)
	// Existing duties should not be cleared on partial failure.
	assert.Equal(t, true, v.duties.IsInitialized())
	assert.Equal(t, 1, len(v.duties.CurrentEpochDuties()))
}

func TestNewValidatorKeys(t *testing.T) {
	pair1 := randKeypair(t)
	pair2 := randKeypair(t)
	pair3 := randKeypair(t)

	v := validator{
		duties: testDutyStore(),
		lastDutiesPubkeys: map[[fieldparams.BLSPubkeyLength]byte]bool{
			pair1.pub: true,
			pair2.pub: true,
		},
	}

	// No new keys when all are tracked.
	newKeys := v.newValidatorKeys([][fieldparams.BLSPubkeyLength]byte{pair1.pub, pair2.pub})
	assert.Equal(t, 0, len(newKeys))

	// pair3 is new.
	newKeys = v.newValidatorKeys([][fieldparams.BLSPubkeyLength]byte{pair1.pub, pair3.pub})
	assert.Equal(t, 1, len(newKeys))
	assert.Equal(t, pair3.pub, newKeys[0])
}

func TestNewValidatorKeys_NilLastDuties(t *testing.T) {
	pair := randKeypair(t)
	v := validator{
		duties: testDutyStore(),
	}
	// Should return nil when lastDutiesPubkeys hasn't been initialized.
	newKeys := v.newValidatorKeys([][fieldparams.BLSPubkeyLength]byte{pair.pub})
	assert.Equal(t, 0, len(newKeys))
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
			keys.pub: {publicKey: keys.pub[:], index: 200},
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
