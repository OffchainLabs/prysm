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
	assert.Equal(t, true, v.duties.isInitialized(), "Existing assignments should be preserved across transient errors")
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

	snap := v.duties.snapshot()
	require.Equal(t, 1, snap.currentDutyCount(), "Expected one duty")
	var gotDuty *ethpb.ValidatorDuty
	for _, d := range snap.currentDuties() {
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
	{
		var data dutyStoreData
		data.setFromContainer(dutiesContainer)
		ds.write(data)
	}
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
		require.DeepEqual(t, curr, v.duties.currDependentRoot())
		require.NoError(t, v.checkDependentRoots(ctx, head))
	})
}

// TestValidator_CheckDependentRoots_UnknownCurrentRootSkips asserts that when
// the cached current dependent root is unknown (nil) — e.g. after a soft
// next-epoch attester failure — a head event does NOT trigger a full
// UpdateDuties. Recovery is owned by the epoch boundary and per-slot retry.
func TestValidator_CheckDependentRoots_UnknownCurrentRootSkips(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := validatormock.NewMockValidatorClient(ctrl)

	ds := &dutyStore{}
	{
		var data dutyStoreData
		// PrevDependentRoot set so the prev check passes; CurrDependentRoot left nil.
		data.setFromContainer(&ethpb.ValidatorDutiesContainer{
			PrevDependentRoot: bytesutil.PadTo([]byte{0x01, 0x02, 0x03}, fieldparams.RootLength),
		})
		ds.write(data)
	}
	require.Equal(t, true, ds.isInitialized())
	require.Equal(t, true, ds.currDependentRoot() == nil)

	v := &validator{
		km:              newMockKeymanager(t, randKeypair(t)),
		validatorClient: client,
		duties:          ds,
		genesisTime:     time.Now(),
	}

	head := &structs.HeadEvent{
		Slot:                      "1",
		PreviousDutyDependentRoot: "0x0102030000000000000000000000000000000000000000000000000000000000",
		CurrentDutyDependentRoot:  "0xe3f7a1b2c489d56f03a6b8d9c7e1fa2456bb09f3de42a67c8910fc3e7a5d4b12",
	}
	// No Duties/AttesterDuties expectations: a triggered UpdateDuties would fail the test.
	require.NoError(t, v.checkDependentRoots(t.Context(), head))
}

// TestValidator_CheckDependentRoots_NoEmptyWindowDuringRefetch asserts that
// concurrent readers of the duty store never observe an empty store while
// checkDependentRoots is refetching. A previous implementation called
// clearDuties() before UpdateDuties(), leaving a window in which other
// goroutines would fail with "no duties for validators".
func TestValidator_CheckDependentRoots_NoEmptyWindowDuringRefetch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ctx := t.Context()
	client := validatormock.NewMockValidatorClient(ctrl)

	oldContainer := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: []*ethpb.ValidatorDuty{{
			AttesterSlot:    params.BeaconConfig().SlotsPerEpoch,
			ValidatorIndex:  200,
			CommitteeIndex:  100,
			CommitteeLength: 4,
			PublicKey:       []byte("testPubKey_1"),
		}},
		PrevDependentRoot: bytesutil.PadTo([]byte{0x01, 0x02, 0x03}, fieldparams.RootLength),
		CurrDependentRoot: bytesutil.PadTo([]byte{0x04, 0x05, 0x06}, fieldparams.RootLength),
	}
	newContainer := &ethpb.ValidatorDutiesContainer{
		CurrentEpochDuties: oldContainer.CurrentEpochDuties,
		PrevDependentRoot:  bytesutil.PadTo([]byte{0xaa, 0xbb, 0xcc}, fieldparams.RootLength),
		CurrDependentRoot:  bytesutil.PadTo([]byte{0xdd, 0xee, 0xff}, fieldparams.RootLength),
	}
	ds := &dutyStore{}
	{
		var data dutyStoreData
		data.setFromContainer(oldContainer)
		ds.write(data)
	}
	v := &validator{
		km:              newMockKeymanager(t, randKeypair(t)),
		validatorClient: client,
		duties:          ds,
	}
	v.aggSelector = testLocalSelector(t, v)

	// Block the RPC inside UpdateDuties until we release it, and signal when
	// the goroutine is actually inside the call so we can probe store state.
	entered := make(chan struct{})
	release := make(chan struct{})
	client.EXPECT().Duties(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *ethpb.DutiesRequest) (*ethpb.ValidatorDutiesContainer, error) {
			close(entered)
			<-release
			return newContainer, nil
		},
	)
	client.EXPECT().SubscribeCommitteeSubnets(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(&emptypb.Empty{}, nil).AnyTimes()

	// Head event with a prev root that differs from stored — triggers
	// needsPrevUpdate.
	head := &structs.HeadEvent{
		Slot:                      "1",
		PreviousDutyDependentRoot: "0xe3f7a1b2c489d56f03a6b8d9c7e1fa2456bb09f3de42a67c8910fc3e7a5d4b12",
		CurrentDutyDependentRoot:  "0xe3f7a1b2c489d56f03a6b8d9c7e1fa2456bb09f3de42a67c8910fc3e7a5d4b12",
	}

	done := make(chan error, 1)
	go func() { done <- v.checkDependentRoots(ctx, head) }()

	<-entered // refetch is in flight

	// The bug: with clearDuties() before UpdateDuties(), the dependent roots
	// would be (nil, nil) here. The fix keeps the OLD values visible until
	// the atomic swap at the end of updateDuties.
	prev := v.duties.prevDependentRoot()
	curr := v.duties.currDependentRoot()
	require.NotNil(t, prev, "duty store was cleared mid-refetch (prev)")
	require.NotNil(t, curr, "duty store was cleared mid-refetch (curr)")
	require.DeepEqual(t, oldContainer.PrevDependentRoot, prev)
	require.DeepEqual(t, oldContainer.CurrDependentRoot, curr)
	require.Equal(t, true, v.duties.isInitialized())

	close(release)
	require.NoError(t, <-done)

	// After completion, the new roots must be in place.
	require.DeepEqual(t, newContainer.PrevDependentRoot, v.duties.prevDependentRoot())
	require.DeepEqual(t, newContainer.CurrDependentRoot, v.duties.currDependentRoot())
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

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, []primitives.ValidatorIndex{42}))

		snap := v.duties.snapshot()
		// Current epoch: attester + proposer + sync + PTC.
		require.Equal(t, 1, snap.currentDutyCount())
		for _, d := range snap.currentDuties() {
			assert.Equal(t, primitives.Slot(epoch)*spe+3, d.AttesterSlot)
			require.Equal(t, 1, len(d.ProposerSlots))
			assert.Equal(t, primitives.Slot(epoch)*spe+1, d.ProposerSlots[0])
			assert.Equal(t, true, d.IsSyncCommittee)
			require.Equal(t, 1, len(d.PtcSlots))
			assert.Equal(t, primitives.Slot(epoch)*spe+5, d.PtcSlots[0])
		}

		// Next epoch: attester + PTC look-ahead.
		require.Equal(t, 1, snap.nextDutyCount())
		for _, d := range snap.nextDuties() {
			assert.Equal(t, primitives.Slot(epoch+1)*spe+7, d.AttesterSlot)
			require.Equal(t, 1, len(d.PtcSlots))
			assert.Equal(t, primitives.Slot(epoch+1)*spe+2, d.PtcSlots[0])
			assert.Equal(t, false, d.IsSyncCommittee)
		}

		// Duty store accessors.
		assert.DeepEqual(t, []primitives.Slot{primitives.Slot(epoch)*spe + 1}, v.duties.proposerSlots(42))
		assert.DeepEqual(t, []primitives.Slot{primitives.Slot(epoch)*spe + 5}, v.duties.ptcSlots(42))
		assert.Equal(t, true, v.duties.isSyncCommittee(42))
		assert.Equal(t, false, v.duties.isNextSyncCommittee(42))
	})

	t.Run("attester error preserves existing duties", func(t *testing.T) {
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch
		seedDuty := &ethpb.ValidatorDuty{
			PublicKey: keys.pub[:], ValidatorIndex: 42,
			AttesterSlot: primitives.Slot(epoch)*spe + 3, Status: ethpb.ValidatorStatus_UNKNOWN_STATUS,
		}
		{
			var data dutyStoreData
			data.setFromContainer(&ethpb.ValidatorDutiesContainer{
				CurrentEpochDuties: []*ethpb.ValidatorDuty{seedDuty},
			})
			v.duties.write(data)
		}

		client.EXPECT().AttesterDuties(gomock.Any(), epoch, gomock.Any()).Return(nil, errors.New("attester fail"))
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(nil, nil).AnyTimes()
		client.EXPECT().ProposerDuties(gomock.Any(), gomock.Any()).Return(&ethpb.ProposerDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).AnyTimes()

		err := v.updateDutiesSplit(t.Context(), epoch, []primitives.ValidatorIndex{42})
		require.ErrorContains(t, "attester fail", err)
		assert.Equal(t, true, v.duties.isInitialized())
		assert.Equal(t, 1, v.duties.snapshot().currentDutyCount())
	})

	t.Run("proposer error preserves existing duties", func(t *testing.T) {
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch
		seedDuty := &ethpb.ValidatorDuty{
			PublicKey: keys.pub[:], ValidatorIndex: 42,
			AttesterSlot: primitives.Slot(epoch)*spe + 3, Status: ethpb.ValidatorStatus_UNKNOWN_STATUS,
		}
		{
			var data dutyStoreData
			data.setFromContainer(&ethpb.ValidatorDutiesContainer{
				CurrentEpochDuties: []*ethpb.ValidatorDuty{seedDuty},
			})
			v.duties.write(data)
		}

		client.EXPECT().AttesterDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.AttesterDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(nil, errors.New("proposer fail"))
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(nil, nil).AnyTimes()
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).AnyTimes()

		err := v.updateDutiesSplit(t.Context(), epoch, []primitives.ValidatorIndex{42})
		require.ErrorContains(t, "proposer fail", err)
		assert.Equal(t, true, v.duties.isInitialized())
		assert.Equal(t, 1, v.duties.snapshot().currentDutyCount())
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

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, []primitives.ValidatorIndex{42}))
		assert.Equal(t, true, v.duties.isInitialized())
		assert.Equal(t, 0, len(v.duties.ptcSlots(42)))
	})

	t.Run("next epoch attester error is non-fatal and defers promotion", func(t *testing.T) {
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch

		client.EXPECT().AttesterDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: make([]byte, 32),
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch)*spe + 3, CommitteeIndex: 1, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(nil, errors.New("next attester fail"))
		client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(&ethpb.ProposerDutiesResponse{DependentRoot: make([]byte, 32)}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).AnyTimes()

		// Current epoch succeeds despite the next-epoch attester failure.
		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, []primitives.ValidatorIndex{42}))
		assert.Equal(t, true, v.duties.isInitialized())
		assert.Equal(t, 1, v.duties.snapshot().currentDutyCount())
		// Missing next-epoch attester keeps promotion ineligible, forcing a retry.
		assert.Equal(t, false, v.duties.canPromote(epoch+1, []primitives.ValidatorIndex{42}))
	})

	t.Run("no known indices clears existing duties", func(t *testing.T) {
		v, _, keys := setup(t)
		v.pubkeyToStatus = map[pubkey]*validatorStatus{}

		// Seed the store with prior duties so the test verifies they're cleared
		// (rather than passing tautologically against an empty store).
		{
			var data dutyStoreData
			data.setFromContainer(&ethpb.ValidatorDutiesContainer{
				CurrentEpochDuties: []*ethpb.ValidatorDuty{{
					PublicKey: keys.pub[:], ValidatorIndex: 42,
					Status: ethpb.ValidatorStatus_ACTIVE,
				}},
			})
			v.duties.write(data)
			require.Equal(t, true, v.duties.isInitialized())
		}

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, nil))
		assert.Equal(t, false, v.duties.isInitialized())
	})

	t.Run("promote-path dependent root divergence falls back to full refetch", func(t *testing.T) {
		hook := logTest.NewGlobal()
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch

		// Seed the store so canPromote is true (epoch-1 cached, next-epoch
		// duties present, init flag set).
		{
			var data dutyStoreData
			data.setFromContainer(&ethpb.ValidatorDutiesContainer{
				NextEpochDuties: []*ethpb.ValidatorDuty{{
					PublicKey: keys.pub[:], ValidatorIndex: 42,
					AttesterSlot: primitives.Slot(epoch)*spe + 3,
					Status:       ethpb.ValidatorStatus_ACTIVE,
				}},
			})
			v.duties.write(data)
		}
		v.duties.data.epoch = epoch - 1
		v.duties.data.currDependentRoot = bytesutil.PadTo([]byte{0xaa}, 32)
		v.duties.data.indices = []primitives.ValidatorIndex{42}

		rootA := bytesutil.PadTo([]byte{0x01}, 32)
		rootB := bytesutil.PadTo([]byte{0x02}, 32)
		rootC := bytesutil.PadTo([]byte{0x03}, 32)

		// Promote path: mismatched roots.
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: rootA,
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch+1)*spe + 7, CommitteeIndex: 2, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(&ethpb.ProposerDutiesResponse{DependentRoot: rootB}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil)
		client.EXPECT().PTCDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil)

		// Refetch path: aligned roots, full set of RPCs.
		client.EXPECT().AttesterDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: bytesutil.PadTo([]byte{0x10}, 32),
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch)*spe + 3, CommitteeIndex: 1, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: rootC,
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch+1)*spe + 7, CommitteeIndex: 2, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(&ethpb.ProposerDutiesResponse{DependentRoot: bytesutil.PadTo([]byte{0x11}, 32)}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(&ethpb.ProposerDutiesResponse{DependentRoot: rootC}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil)
		client.EXPECT().PTCDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil)
		client.EXPECT().PTCDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil)

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, []primitives.ValidatorIndex{42}))
		assert.LogsContain(t, hook, "diverged on promotion")

		// Refetch's currDepRoot is the next-epoch attester root.
		require.DeepEqual(t, rootC, v.duties.currDependentRoot())
		assert.Equal(t, epoch, v.duties.data.epoch)
	})

	t.Run("incomplete cache forces full refetch instead of promote", func(t *testing.T) {
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch

		// First iteration at epoch: next-epoch proposer soft-fails. All other RPCs succeed.
		// fetchProposerDuties logs nextErr at Debug and returns next=nil, so propErr is nil.
		client.EXPECT().AttesterDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: make([]byte, 32),
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch) * spe, CommitteeIndex: 1, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch+1) * spe, CommitteeIndex: 2, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(nil, errors.New("next proposer fail"))
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).Times(2)
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).Times(2)

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, []primitives.ValidatorIndex{42}))
		require.Equal(t, missingNextProposer, v.duties.data.missingNext&missingNextProposer)

		// Second iteration at epoch+1. v.duties.epoch+1 == epoch+1 would normally trigger
		// the promote path (only 4 next-epoch RPCs). The dirty mask must force a full fetch,
		// so we expect all 8 RPCs (current+next for each duty type).
		nextEpoch := epoch + 1
		client.EXPECT().AttesterDuties(gomock.Any(), nextEpoch, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: make([]byte, 32),
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(nextEpoch) * spe, CommitteeIndex: 1, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().AttesterDuties(gomock.Any(), nextEpoch+1, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(nextEpoch+1) * spe, CommitteeIndex: 2, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), nextEpoch).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), nextEpoch+1).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).Times(2)
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).Times(2)

		require.NoError(t, v.updateDutiesSplit(t.Context(), nextEpoch, []primitives.ValidatorIndex{42}))
		require.Equal(t, missingNextDuties(0), v.duties.data.missingNext)
	})

	t.Run("unfilled next duties force full current+next fetch at boundary", func(t *testing.T) {
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch

		// End of epoch E with the gap never closed (mid-epoch retries kept
		// failing): missingNextPtc still set, and a cached next-epoch attester
		// slot sentinel (+99) that must NOT survive into the current epoch.
		{
			var data dutyStoreData
			data.setFromContainer(&ethpb.ValidatorDutiesContainer{
				CurrentEpochDuties: []*ethpb.ValidatorDuty{{
					PublicKey: keys.pub[:], ValidatorIndex: 42, AttesterSlot: primitives.Slot(epoch) * spe,
				}},
				NextEpochDuties: []*ethpb.ValidatorDuty{{
					PublicKey: keys.pub[:], ValidatorIndex: 42, AttesterSlot: primitives.Slot(epoch+1)*spe + 99,
				}},
			})
			data.epoch = epoch
			data.indices = []primitives.ValidatorIndex{42}
			data.missingNext = missingNextPtc
			v.duties.write(data)
		}

		// Gap still open => promotion refused at the boundary.
		require.Equal(t, false, v.duties.canPromote(epoch+1, []primitives.ValidatorIndex{42}))

		next := epoch + 1
		// A full fetch calls AttesterDuties for current (next) AND next (next+1);
		// a promote would only fetch next+1. Current returns a fresh slot (+3).
		client.EXPECT().AttesterDuties(gomock.Any(), next, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: make([]byte, 32),
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(next)*spe + 3, CommitteeIndex: 1, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().AttesterDuties(gomock.Any(), next+1, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), next).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), next+1).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).Times(2)
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).Times(2)

		require.NoError(t, v.updateDutiesSplit(t.Context(), next, []primitives.ValidatorIndex{42}))

		// Full fetch: current came from the fresh AttesterDuties(next)=+3, not the
		// promoted cached sentinel (+99); and the gap is now closed.
		require.Equal(t, missingNextDuties(0), v.duties.data.missingNext)
		cur, ok := v.duties.currentDuty(keys.pub)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(next)*spe+3, cur.AttesterSlot)
	})

	t.Run("validator set drift forces full refetch instead of promote", func(t *testing.T) {
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch

		// Seed the store with indices=[42] and a complete next-epoch cache so
		// that, ignoring drift, canPromote would otherwise return true.
		{
			var data dutyStoreData
			data.setFromContainer(&ethpb.ValidatorDutiesContainer{
				NextEpochDuties: []*ethpb.ValidatorDuty{{
					PublicKey: keys.pub[:], ValidatorIndex: 42,
					Status: ethpb.ValidatorStatus_ACTIVE,
				}},
			})
			data.epoch = epoch - 1
			data.indices = []primitives.ValidatorIndex{42}
			v.duties.write(data)
		}

		// Caller now presents a different (larger) index set; canPromote must
		// reject promotion and fall through to fetchAllDuties.
		client.EXPECT().AttesterDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: make([]byte, 32),
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch) * spe, CommitteeIndex: 1, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).Times(2)
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).Times(2)

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, []primitives.ValidatorIndex{42, 99}))
		require.DeepEqual(t, []primitives.ValidatorIndex{42, 99}, v.duties.data.indices)
	})

	t.Run("combined-endpoint cache cannot promote into split", func(t *testing.T) {
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch

		// Simulate what updateDutiesCombined leaves behind: a populated next-
		// epoch cache, missingNext=missingNextPtc, and indices empty (combined
		// path doesn't track them). The first split call must refetch.
		{
			var data dutyStoreData
			data.setFromContainer(&ethpb.ValidatorDutiesContainer{
				NextEpochDuties: []*ethpb.ValidatorDuty{{
					PublicKey: keys.pub[:], ValidatorIndex: 42,
					Status: ethpb.ValidatorStatus_ACTIVE,
				}},
			})
			data.missingNext = missingNextPtc
			v.duties.write(data)
		}

		// Expect full-fetch RPC pattern (8 endpoints), not promote (4).
		client.EXPECT().AttesterDuties(gomock.Any(), epoch, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: make([]byte, 32),
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch) * spe, CommitteeIndex: 1, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(&ethpb.ProposerDutiesResponse{}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).Times(2)
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).Times(2)

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, []primitives.ValidatorIndex{42}))
		// After a full fetch, missingNext is reset.
		require.Equal(t, missingNextDuties(0), v.duties.data.missingNext)
	})

	t.Run("promote refreshes Status from pubkeyToStatus", func(t *testing.T) {
		v, client, keys := setup(t)
		spe := params.BeaconConfig().SlotsPerEpoch

		// Seed the store as if the prior fetch saw the validator as PENDING
		// (activation epoch reached, so it was admitted into the duty set).
		{
			var data dutyStoreData
			data.setFromContainer(&ethpb.ValidatorDutiesContainer{
				NextEpochDuties: []*ethpb.ValidatorDuty{{
					PublicKey: keys.pub[:], ValidatorIndex: 42,
					AttesterSlot: primitives.Slot(epoch)*spe + 3,
					Status:       ethpb.ValidatorStatus_PENDING,
				}},
				CurrDependentRoot: bytesutil.PadTo([]byte{0xaa}, 32),
			})
			data.epoch = epoch - 1
			data.indices = []primitives.ValidatorIndex{42}
			v.duties.write(data)
		}

		root := bytesutil.PadTo([]byte{0x01}, 32)
		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.AttesterDutiesResponse{
			DependentRoot: root,
			Duties: []*ethpb.AttesterDuty{{
				Pubkey: keys.pub[:], ValidatorIndex: 42,
				Slot: primitives.Slot(epoch+1)*spe + 7, CommitteeIndex: 2, CommitteeLength: 64, CommitteesAtSlot: 4,
			}},
		}, nil)
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(&ethpb.ProposerDutiesResponse{DependentRoot: root}, nil)
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil)
		client.EXPECT().PTCDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil)

		require.NoError(t, v.updateDutiesSplit(t.Context(), epoch, []primitives.ValidatorIndex{42}))

		snap := v.duties.snapshot()
		require.Equal(t, 1, snap.currentDutyCount())
		for _, d := range snap.currentDuties() {
			assert.Equal(t, ethpb.ValidatorStatus_ACTIVE, d.Status)
		}
	})
}

func TestRetryMissingNextDuties(t *testing.T) {
	epoch := primitives.Epoch(5)
	spe := params.BeaconConfig().SlotsPerEpoch

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
			// UNKNOWN status keeps the async subnet subscription from signing for
			// these duties, which would need a keymanager this test doesn't wire up.
			pubkeyToStatus: map[pubkey]*validatorStatus{
				keys.pub: {publicKey: keys.pub[:], status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_UNKNOWN_STATUS}, index: 42},
			},
		}
		v.aggSelector = testLocalSelector(t, v)
		client.EXPECT().SubscribeCommitteeSubnets(gomock.Any(), gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil).AnyTimes()
		return v, client, keys
	}

	// seed writes current + next duties with the given missing mask.
	seed := func(v *validator, keys keypair, missing missingNextDuties) {
		var data dutyStoreData
		data.setFromContainer(&ethpb.ValidatorDutiesContainer{
			CurrentEpochDuties: []*ethpb.ValidatorDuty{{
				PublicKey: keys.pub[:], ValidatorIndex: 42,
				AttesterSlot: primitives.Slot(epoch)*spe + 3, Status: ethpb.ValidatorStatus_UNKNOWN_STATUS,
			}},
			NextEpochDuties: []*ethpb.ValidatorDuty{{
				PublicKey: keys.pub[:], ValidatorIndex: 42,
				AttesterSlot: primitives.Slot(epoch+1)*spe + 7, Status: ethpb.ValidatorStatus_UNKNOWN_STATUS,
			}},
		})
		data.epoch = epoch
		data.indices = []primitives.ValidatorIndex{42}
		data.missingNext = missing
		v.duties.write(data)
	}

	t.Run("overlay fills the missing type without refetching the spine", func(t *testing.T) {
		v, client, keys := setup(t)
		seed(v, keys, missingNextPtc)

		// Only PTC is re-fetched. No attester/proposer/sync expectations: if the
		// targeted retry fetched them, gomock would fail the test.
		client.EXPECT().PTCDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.PTCDutiesResponse{
			Duties: []*ethpb.PTCDuty{{Pubkey: keys.pub[:], ValidatorIndex: 42, Slot: primitives.Slot(epoch+1)*spe + 2}},
		}, nil)

		require.NoError(t, v.RetryMissingNextDuties(t.Context()))

		snap := v.duties.snapshot()
		require.Equal(t, 1, snap.nextDutyCount())
		for _, d := range snap.nextDuties() {
			require.Equal(t, 1, len(d.PtcSlots))
			assert.Equal(t, primitives.Slot(epoch+1)*spe+2, d.PtcSlots[0])
			// Attester spine preserved from the existing duties (not re-fetched).
			assert.Equal(t, primitives.Slot(epoch+1)*spe+7, d.AttesterSlot)
		}
		assert.Equal(t, true, v.duties.canPromote(epoch+1, []primitives.ValidatorIndex{42}))
	})

	t.Run("missing attester spine is rebuilt; failure retries next slot", func(t *testing.T) {
		v, client, keys := setup(t)
		// Spine missing -> rebuild path (re-fetch the whole epoch). The fetch fails,
		// so existing duties are left intact and promotion stays blocked; the bit
		// stays set so MaybeRetry will try again next slot.
		seed(v, keys, missingNextAttester)

		client.EXPECT().AttesterDuties(gomock.Any(), epoch+1, gomock.Any()).Return(nil, errors.New("att down"))
		client.EXPECT().ProposerDuties(gomock.Any(), gomock.Any()).Return(&ethpb.ProposerDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().SyncCommitteeDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.SyncCommitteeDutiesResponse{}, nil).AnyTimes()
		client.EXPECT().PTCDuties(gomock.Any(), gomock.Any(), gomock.Any()).Return(&ethpb.PTCDutiesResponse{}, nil).AnyTimes()

		assert.Equal(t, true, v.duties.needsNextRetry()) // attester is retried like any other type
		require.NoError(t, v.RetryMissingNextDuties(t.Context()))

		snap := v.duties.snapshot()
		require.Equal(t, 1, snap.nextDutyCount())
		for _, d := range snap.nextDuties() {
			assert.Equal(t, primitives.Slot(epoch+1)*spe+7, d.AttesterSlot)
		}
		assert.Equal(t, false, v.duties.canPromote(epoch+1, []primitives.ValidatorIndex{42}))
	})

	t.Run("no missing duties is a no-op", func(t *testing.T) {
		v, _, keys := setup(t)
		seed(v, keys, 0)
		// No client calls expected.
		require.NoError(t, v.RetryMissingNextDuties(t.Context()))
		assert.Equal(t, true, v.duties.canPromote(epoch+1, []primitives.ValidatorIndex{42}))
	})

	t.Run("no progress leaves the store untouched", func(t *testing.T) {
		v, client, keys := setup(t)
		seed(v, keys, missingNextPtc)

		// Only PTC is retried and it keeps failing, so the missing set is unchanged
		// and the no-progress guard must skip the write (and the re-subscribe).
		client.EXPECT().PTCDuties(gomock.Any(), epoch+1, gomock.Any()).Return(nil, errors.New("ptc still down"))

		require.NoError(t, v.RetryMissingNextDuties(t.Context()))

		snap := v.duties.snapshot()
		require.Equal(t, 1, snap.nextDutyCount())
		for _, d := range snap.nextDuties() {
			// Seeded spine preserved, PTC still empty -> store was not replaced.
			assert.Equal(t, primitives.Slot(epoch+1)*spe+7, d.AttesterSlot)
			assert.Equal(t, 0, len(d.PtcSlots))
		}
		assert.Equal(t, false, v.duties.canPromote(epoch+1, []primitives.ValidatorIndex{42}))
	})

	t.Run("partial progress clears only the filled type", func(t *testing.T) {
		v, client, keys := setup(t)
		seed(v, keys, missingNextProposer|missingNextPtc)

		// Only the two flagged types are re-fetched (attester/sync are not): proposer
		// succeeds, PTC still fails.
		client.EXPECT().ProposerDuties(gomock.Any(), epoch+1).Return(&ethpb.ProposerDutiesResponse{
			Duties: []*ethpb.ProposerDutyV2{{Pubkey: keys.pub[:], ValidatorIndex: 42, Slot: primitives.Slot(epoch+1)*spe + 1}},
		}, nil)
		client.EXPECT().PTCDuties(gomock.Any(), epoch+1, gomock.Any()).Return(nil, errors.New("ptc still down"))

		require.NoError(t, v.RetryMissingNextDuties(t.Context()))

		snap := v.duties.snapshot()
		for _, d := range snap.nextDuties() {
			require.Equal(t, 1, len(d.ProposerSlots))                       // proposer filled
			assert.Equal(t, 0, len(d.PtcSlots))                             // ptc still missing
			assert.Equal(t, primitives.Slot(epoch+1)*spe+7, d.AttesterSlot) // spine untouched
		}
		// PTC still missing keeps promotion blocked.
		assert.Equal(t, false, v.duties.canPromote(epoch+1, []primitives.ValidatorIndex{42}))
	})

	t.Run("self-gates with no network call when indices are empty", func(t *testing.T) {
		v, _, keys := setup(t)
		// Combined-path-like state: missing flagged but no indices recorded.
		var data dutyStoreData
		data.setFromContainer(&ethpb.ValidatorDutiesContainer{
			NextEpochDuties: []*ethpb.ValidatorDuty{{PublicKey: keys.pub[:], ValidatorIndex: 42}},
		})
		data.epoch = epoch
		data.missingNext = missingNextPtc // indices left nil
		v.duties.write(data)

		// No duty-fetch expectations: a call would fail the test.
		require.NoError(t, v.RetryMissingNextDuties(t.Context()))
	})

	t.Run("MaybeRetry is a no-op when nothing is missing", func(t *testing.T) {
		v, _, keys := setup(t)
		v.genesisTime = time.Now()
		seed(v, keys, 0)
		// needsNextRetry() is false: returns synchronously, no goroutine, flag untouched.
		v.MaybeRetryMissingNextDuties(t.Context(), 0)
		assert.Equal(t, false, v.retryInFlight.Load())
	})

	t.Run("MaybeRetry skips when a retry is already in flight", func(t *testing.T) {
		v, _, keys := setup(t)
		v.genesisTime = time.Now()
		seed(v, keys, missingNextPtc)
		v.retryInFlight.Store(true) // simulate one already running
		// CAS fails: no goroutine spawned, no duty fetches (none are mocked).
		v.MaybeRetryMissingNextDuties(t.Context(), 0)
		assert.Equal(t, true, v.retryInFlight.Load())
	})

	t.Run("MaybeRetry spawns a retry that fills missing duties", func(t *testing.T) {
		v, client, keys := setup(t)
		v.genesisTime = time.Now()
		seed(v, keys, missingNextPtc)

		// Targeted overlay: only the flagged PTC type is re-fetched.
		client.EXPECT().PTCDuties(gomock.Any(), epoch+1, gomock.Any()).Return(&ethpb.PTCDutiesResponse{
			Duties: []*ethpb.PTCDuty{{Pubkey: keys.pub[:], ValidatorIndex: 42, Slot: primitives.Slot(epoch+1)*spe + 2}},
		}, nil).AnyTimes()

		v.MaybeRetryMissingNextDuties(t.Context(), 0)

		// Poll until the spawned goroutine finishes (in-flight flag reset). The flag
		// is cleared last, so by then the duties are filled too.
		deadline := time.Now().Add(2 * time.Second)
		for v.retryInFlight.Load() && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		assert.Equal(t, false, v.retryInFlight.Load()) // flag released for the next retry
		assert.Equal(t, true, v.duties.canPromote(epoch+1, []primitives.ValidatorIndex{42}))
	})
}

func TestIsActiveForDuties(t *testing.T) {
	tests := []struct {
		name     string
		status   *ethpb.ValidatorStatusResponse
		epoch    primitives.Epoch
		expected bool
	}{
		{"nil", nil, 5, false},
		{"unknown", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_UNKNOWN_STATUS}, 5, false},
		{"deposited", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_DEPOSITED}, 5, false},
		{"pending before activation", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_PENDING, ActivationEpoch: 10}, 5, false},
		{"pending at activation", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_PENDING, ActivationEpoch: 5}, 5, true},
		{"pending after activation", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_PENDING, ActivationEpoch: 3}, 5, true},
		{"active", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}, 5, true},
		{"exiting", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_EXITING}, 5, true},
		{"slashing", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_SLASHING}, 5, false},
		{"exited", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_EXITED}, 5, false},
		{"invalid", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_INVALID}, 5, false},
		{"partially deposited", &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_PARTIALLY_DEPOSITED}, 5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isActiveForDuties(tt.status, tt.epoch))
		})
	}
}

func TestFilteredKeysAndIndices(t *testing.T) {
	pkActive := bytesutil.ToBytes48([]byte{1})
	pkPending := bytesutil.ToBytes48([]byte{2})
	pkExited := bytesutil.ToBytes48([]byte{3})
	pkUnknown := bytesutil.ToBytes48([]byte{4}) // not in pubkeyToStatus
	pkActive2 := bytesutil.ToBytes48([]byte{5})

	v := &validator{
		pubkeyToStatus: map[pubkey]*validatorStatus{
			pkActive:  {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}, index: 99},
			pkPending: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_PENDING, ActivationEpoch: 10}, index: 50},
			pkExited:  {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_EXITED}, index: 7},
			// pkActive2 has a smaller index than pkActive to verify sorting.
			pkActive2: {status: &ethpb.ValidatorStatusResponse{Status: ethpb.ValidatorStatus_ACTIVE}, index: 3},
		},
	}

	// At epoch 5, pkPending's activation epoch (10) hasn't been reached.
	keys, idx := v.filteredKeysAndIndices([][fieldparams.BLSPubkeyLength]byte{pkActive, pkPending, pkExited, pkUnknown, pkActive2}, 5)

	// Indices are sorted; pkActive2 (3) precedes pkActive (99).
	require.DeepEqual(t, []primitives.ValidatorIndex{3, 99}, idx)
	require.Equal(t, 2, len(keys))

	// At epoch 10, pkPending qualifies (activation epoch reached).
	keys, idx = v.filteredKeysAndIndices([][fieldparams.BLSPubkeyLength]byte{pkActive, pkPending, pkExited, pkUnknown, pkActive2}, 10)
	require.DeepEqual(t, []primitives.ValidatorIndex{3, 50, 99}, idx)
	require.Equal(t, 3, len(keys))
}
