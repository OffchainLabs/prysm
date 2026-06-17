package validator

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	mockp2p "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	prysmTime "github.com/OffchainLabs/prysm/v7/time"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestProposeAttestation(t *testing.T) {
	chainService := &mock.ChainService{}
	attesterServer := &Server{
		HeadFetcher:             chainService,
		P2P:                     &mockp2p.MockBroadcaster{},
		AttPool:                 attestations.NewPool(),
		OperationNotifier:       (&mock.ChainService{}).OperationNotifier(),
		TimeFetcher:             chainService,
		AttestationStateFetcher: chainService,
		SyncChecker:             &mockSync.Sync{IsSyncing: false},
	}
	head := util.NewBeaconBlock()
	head.Block.Slot = 999
	head.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	root, err := head.Block.HashTreeRoot()
	require.NoError(t, err)

	validators := make([]*ethpb.Validator, 64)
	for i := range validators {
		validators[i] = &ethpb.Validator{
			PublicKey:             make([]byte, 48),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
		}
	}

	sk, err := bls.RandKey()
	require.NoError(t, err)
	sig := sk.Sign([]byte("dummy_test_data"))

	t.Run("Phase 0", func(t *testing.T) {
		state, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, state.SetSlot(params.BeaconConfig().SlotsPerEpoch+1))
		require.NoError(t, state.SetValidators(validators))

		req := &ethpb.Attestation{
			Signature: sig.Marshal(),
			Data: &ethpb.AttestationData{
				BeaconBlockRoot: root[:],
				Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
				Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			},
		}
		_, err = attesterServer.ProposeAttestation(t.Context(), req)
		assert.NoError(t, err)
	})
	t.Run("Phase 0 post electra", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		config := params.BeaconConfig()
		config.ElectraForkEpoch = 0
		params.OverrideBeaconConfig(config)

		state, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, state.SetSlot(params.BeaconConfig().SlotsPerEpoch+1))
		require.NoError(t, state.SetValidators(validators))

		req := &ethpb.Attestation{
			Signature: sig.Marshal(),
			Data: &ethpb.AttestationData{
				BeaconBlockRoot: root[:],
				Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
				Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			},
		}
		_, err = attesterServer.ProposeAttestation(t.Context(), req)
		assert.ErrorContains(t, "old attestation format", err)
	})
	t.Run("Electra", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		config := params.BeaconConfig()
		config.ElectraForkEpoch = 0
		params.OverrideBeaconConfig(config)

		state, err := util.NewBeaconStateElectra()
		require.NoError(t, err)
		require.NoError(t, state.SetSlot(params.BeaconConfig().SlotsPerEpoch+1))
		require.NoError(t, state.SetValidators(validators))
		chainService.State = state

		req := &ethpb.SingleAttestation{
			Signature: sig.Marshal(),
			Data: &ethpb.AttestationData{
				BeaconBlockRoot: root[:],
				Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
				Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			},
		}
		_, err = attesterServer.ProposeAttestationElectra(t.Context(), req)
		assert.NoError(t, err)
	})
	t.Run("Electra att too early", func(t *testing.T) {
		req := &ethpb.SingleAttestation{
			Signature: sig.Marshal(),
			Data: &ethpb.AttestationData{
				BeaconBlockRoot: root[:],
				Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
				Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			},
		}
		_, err = attesterServer.ProposeAttestationElectra(t.Context(), req)
		assert.ErrorContains(t, "ProposeAttestationElectra not supported yet", err)
	})
	t.Run("Gloas rejects committee index >= 2", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		config := params.BeaconConfig().Copy()
		config.ElectraForkEpoch = 0
		config.GloasForkEpoch = 0
		params.OverrideBeaconConfig(config)

		req := &ethpb.SingleAttestation{
			Signature: sig.Marshal(),
			Data: &ethpb.AttestationData{
				Slot:            params.BeaconConfig().SlotsPerEpoch + 1,
				CommitteeIndex:  2,
				BeaconBlockRoot: root[:],
				Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
				Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			},
		}
		_, err = attesterServer.ProposeAttestationElectra(t.Context(), req)
		assert.ErrorContains(t, "index must be < 2 post-Gloas", err)
	})
	t.Run("Gloas rejects index 1 for same slot", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		config := params.BeaconConfig().Copy()
		config.ElectraForkEpoch = 0
		config.GloasForkEpoch = 0
		params.OverrideBeaconConfig(config)

		attSlot := params.BeaconConfig().SlotsPerEpoch + 1
		server := &Server{
			HeadFetcher:             chainService,
			P2P:                     &mockp2p.MockBroadcaster{},
			AttPool:                 attestations.NewPool(),
			OperationNotifier:       (&mock.ChainService{}).OperationNotifier(),
			TimeFetcher:             chainService,
			AttestationStateFetcher: chainService,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
			ForkchoiceFetcher:       &mock.ChainService{BlockSlot: attSlot},
		}
		req := &ethpb.SingleAttestation{
			Signature: sig.Marshal(),
			Data: &ethpb.AttestationData{
				Slot:            attSlot,
				CommitteeIndex:  1,
				BeaconBlockRoot: root[:],
				Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
				Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			},
		}
		_, err = server.ProposeAttestationElectra(t.Context(), req)
		assert.ErrorContains(t, "same slot attestations must use index 0 post-Gloas", err)
	})
	t.Run("Gloas allows index 1 for prior slot", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		config := params.BeaconConfig().Copy()
		config.ElectraForkEpoch = 0
		config.GloasForkEpoch = 0
		params.OverrideBeaconConfig(config)

		attSlot := params.BeaconConfig().SlotsPerEpoch + 1
		state, err := util.NewBeaconStateElectra()
		require.NoError(t, err)
		require.NoError(t, state.SetSlot(attSlot))
		require.NoError(t, state.SetValidators(validators))
		cs := &mock.ChainService{State: state, BlockSlot: attSlot - 1}
		server := &Server{
			HeadFetcher:             cs,
			P2P:                     &mockp2p.MockBroadcaster{},
			AttPool:                 attestations.NewPool(),
			OperationNotifier:       (&mock.ChainService{}).OperationNotifier(),
			TimeFetcher:             cs,
			AttestationStateFetcher: cs,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
			ForkchoiceFetcher:       cs,
		}
		req := &ethpb.SingleAttestation{
			Signature: sig.Marshal(),
			Data: &ethpb.AttestationData{
				Slot:            attSlot,
				CommitteeIndex:  1,
				BeaconBlockRoot: root[:],
				Source:          &ethpb.Checkpoint{Root: make([]byte, 32)},
				Target:          &ethpb.Checkpoint{Root: make([]byte, 32)},
			},
		}
		_, err = server.ProposeAttestationElectra(t.Context(), req)
		assert.NoError(t, err)
	})
}

func TestProposeAttestation_IncorrectSignature(t *testing.T) {
	attesterServer := &Server{
		HeadFetcher:       &mock.ChainService{},
		P2P:               &mockp2p.MockBroadcaster{},
		AttPool:           attestations.NewPool(),
		OperationNotifier: (&mock.ChainService{}).OperationNotifier(),
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
	}

	req := util.HydrateAttestation(&ethpb.Attestation{})
	wanted := "Incorrect attestation signature"
	_, err := attesterServer.ProposeAttestation(t.Context(), req)
	assert.ErrorContains(t, wanted, err)
}

func TestProposeAttestation_Syncing(t *testing.T) {
	attesterServer := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}

	req := util.HydrateAttestation(&ethpb.Attestation{})
	_, err := attesterServer.ProposeAttestation(t.Context(), req)
	assert.ErrorContains(t, "Syncing to latest head", err)
	s, ok := status.FromError(err)
	require.Equal(t, true, ok)
	assert.Equal(t, codes.Unavailable, s.Code())
}

func TestProposeAttestationElectra_Syncing(t *testing.T) {
	attesterServer := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}

	req := &ethpb.SingleAttestation{
		Data: &ethpb.AttestationData{
			Source: &ethpb.Checkpoint{Root: make([]byte, 32)},
			Target: &ethpb.Checkpoint{Root: make([]byte, 32)},
		},
	}
	_, err := attesterServer.ProposeAttestationElectra(t.Context(), req)
	assert.ErrorContains(t, "Syncing to latest head", err)
	s, ok := status.FromError(err)
	require.Equal(t, true, ok)
	assert.Equal(t, codes.Unavailable, s.Code())
}

func TestGetAttestationData_OK(t *testing.T) {
	block := util.NewBeaconBlock()
	block.Block.Slot = 3*params.BeaconConfig().SlotsPerEpoch + 1
	targetBlock := util.NewBeaconBlock()
	targetBlock.Block.Slot = 1 * params.BeaconConfig().SlotsPerEpoch
	targetRoot, err := targetBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for target block")

	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot = 2 * params.BeaconConfig().SlotsPerEpoch
	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not hash beacon block")
	justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for justified block")
	slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(slot))
	justifiedCheckpoint := &ethpb.Checkpoint{
		Epoch: 2,
		Root:  justifiedRoot[:],
	}
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(justifiedCheckpoint))
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			HeadFetcher: &mock.ChainService{TargetRoot: targetRoot, Root: blockRoot[:], State: beaconState},
			GenesisTimeFetcher: &mock.ChainService{
				Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second),
			},
			FinalizedFetcher:      &mock.ChainService{CurrentJustifiedCheckPoint: justifiedCheckpoint},
			AttestationCache:      cache.NewAttestationDataCache(),
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		},
	}

	req := &ethpb.AttestationDataRequest{
		CommitteeIndex: 0,
		Slot:           3*params.BeaconConfig().SlotsPerEpoch + 1,
	}
	res, err := attesterServer.GetAttestationData(t.Context(), req)
	require.NoError(t, err, "Could not get attestation info at slot")

	expectedInfo := &ethpb.AttestationData{
		Slot:            3*params.BeaconConfig().SlotsPerEpoch + 1,
		BeaconBlockRoot: blockRoot[:],
		Source: &ethpb.Checkpoint{
			Epoch: 2,
			Root:  justifiedRoot[:],
		},
		Target: &ethpb.Checkpoint{
			Epoch: 3,
			Root:  targetRoot[:],
		},
	}

	if !proto.Equal(res, expectedInfo) {
		t.Errorf("Expected attestation info to match, received %v, wanted %v", res, expectedInfo)
	}
}

func BenchmarkGetAttestationDataConcurrent(b *testing.B) {
	block := util.NewBeaconBlock()
	block.Block.Slot = 3*params.BeaconConfig().SlotsPerEpoch + 1
	targetBlock := util.NewBeaconBlock()
	targetBlock.Block.Slot = 1 * params.BeaconConfig().SlotsPerEpoch
	targetRoot, err := targetBlock.Block.HashTreeRoot()
	require.NoError(b, err, "Could not get signing root for target block")

	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot = 2 * params.BeaconConfig().SlotsPerEpoch
	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(b, err, "Could not hash beacon block")
	justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(b, err, "Could not get signing root for justified block")
	slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
	justifiedCheckpoint := &ethpb.Checkpoint{
		Epoch: 2,
		Root:  justifiedRoot[:],
	}
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			AttestationCache: cache.NewAttestationDataCache(),
			HeadFetcher:      &mock.ChainService{TargetRoot: targetRoot, Root: blockRoot[:]},
			GenesisTimeFetcher: &mock.ChainService{
				Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second),
			},
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
			FinalizedFetcher:      &mock.ChainService{CurrentJustifiedCheckPoint: justifiedCheckpoint},
		},
	}

	req := &ethpb.AttestationDataRequest{
		CommitteeIndex: 0,
		Slot:           3*params.BeaconConfig().SlotsPerEpoch + 1,
	}

	for b.Loop() {
		var wg sync.WaitGroup

		for range 5000 {
			wg.Go(func() {
				_, err := attesterServer.GetAttestationData(b.Context(), req)
				require.NoError(b, err, "Could not get attestation info at slot")
			})
		}
		wg.Wait() // Wait for all goroutines to finish
	}

	b.Log("Elapsed time:", b.Elapsed())
}

func TestGetAttestationData_SyncNotReady(t *testing.T) {
	as := Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}
	_, err := as.GetAttestationData(t.Context(), &ethpb.AttestationDataRequest{})
	assert.ErrorContains(t, "Syncing to latest head", err)
}

func TestGetAttestationData_Optimistic(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.BellatrixForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	as := &Server{
		SyncChecker:           &mockSync.Sync{},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: true},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		CoreService: &core.Service{
			GenesisTimeFetcher:    &mock.ChainService{Genesis: time.Now()},
			HeadFetcher:           &mock.ChainService{},
			AttestationCache:      cache.NewAttestationDataCache(),
			OptimisticModeFetcher: &mock.ChainService{Optimistic: true},
		},
	}
	_, err := as.GetAttestationData(t.Context(), &ethpb.AttestationDataRequest{})
	s, ok := status.FromError(err)
	require.Equal(t, true, ok)
	require.DeepEqual(t, codes.Unavailable, s.Code())
	require.ErrorContains(t, errOptimisticMode.Error(), err)

	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	as = &Server{
		SyncChecker:           &mockSync.Sync{},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		CoreService: &core.Service{
			AttestationCache:      cache.NewAttestationDataCache(),
			GenesisTimeFetcher:    &mock.ChainService{Genesis: time.Now()},
			HeadFetcher:           &mock.ChainService{Optimistic: false, State: beaconState},
			FinalizedFetcher:      &mock.ChainService{CurrentJustifiedCheckPoint: &ethpb.Checkpoint{}},
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		},
	}
	_, err = as.GetAttestationData(t.Context(), &ethpb.AttestationDataRequest{})
	require.NoError(t, err)
}

func TestServer_GetAttestationData_InvalidRequestSlot(t *testing.T) {
	ctx := t.Context()

	slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			GenesisTimeFetcher:    &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		},
	}

	req := &ethpb.AttestationDataRequest{
		Slot: 1000000000000,
	}
	_, err := attesterServer.GetAttestationData(ctx, req)
	assert.ErrorContains(t, "invalid request", err)
}

func TestServer_GetAttestationData_RequestSlotIsDifferentThanCurrentSlot(t *testing.T) {
	ctx := t.Context()
	db := dbutil.SetupDB(t)

	slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
	block := util.NewBeaconBlock()
	block.Block.Slot = slot
	block2 := util.NewBeaconBlock()
	block2.Block.Slot = slot - 1
	targetBlock := util.NewBeaconBlock()
	targetBlock.Block.Slot = 1 * params.BeaconConfig().SlotsPerEpoch
	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot = 2 * params.BeaconConfig().SlotsPerEpoch
	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not hash beacon block")
	blockRoot2, err := block2.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, db, block2)
	justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for justified block")
	justifiedCheckpoint := &ethpb.Checkpoint{
		Epoch: 2,
		Root:  justifiedRoot[:],
	}
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			HeadFetcher:           &mock.ChainService{TargetRoot: blockRoot2, Root: blockRoot[:]},
			GenesisTimeFetcher:    &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
			StateGen:              stategen.New(db, doublylinkedtree.New()),
			FinalizedFetcher:      &mock.ChainService{CurrentJustifiedCheckPoint: justifiedCheckpoint},
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		},
	}
	util.SaveBlock(t, ctx, db, block)

	req := &ethpb.AttestationDataRequest{
		CommitteeIndex: 0,
		Slot:           slot - 1,
	}
	_, err = attesterServer.GetAttestationData(ctx, req)
	require.ErrorContains(t, "invalid request: slot 24 is not the current slot 25", err)
}

func TestGetAttestationData_SucceedsInFirstEpoch(t *testing.T) {
	slot := primitives.Slot(5)
	block := util.NewBeaconBlock()
	block.Block.Slot = slot
	targetBlock := util.NewBeaconBlock()
	targetBlock.Block.Slot = 0
	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot = 0
	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not hash beacon block")
	justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for justified block")
	targetRoot, err := targetBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for target block")

	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(slot))
	justifiedCheckpoint := &ethpb.Checkpoint{
		Epoch: 0,
		Root:  justifiedRoot[:],
	}
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(justifiedCheckpoint))

	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: prysmTime.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			AttestationCache: cache.NewAttestationDataCache(),
			HeadFetcher: &mock.ChainService{
				TargetRoot: targetRoot, Root: blockRoot[:], State: beaconState,
			},
			GenesisTimeFetcher:    &mock.ChainService{Genesis: prysmTime.Now().Add(time.Duration(-1*offset) * time.Second)},
			FinalizedFetcher:      &mock.ChainService{CurrentJustifiedCheckPoint: justifiedCheckpoint},
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		},
	}

	req := &ethpb.AttestationDataRequest{
		CommitteeIndex: 0,
		Slot:           5,
	}
	res, err := attesterServer.GetAttestationData(t.Context(), req)
	require.NoError(t, err, "Could not get attestation info at slot")

	expectedInfo := &ethpb.AttestationData{
		Slot:            slot,
		BeaconBlockRoot: blockRoot[:],
		Source: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  justifiedRoot[:],
		},
		Target: &ethpb.Checkpoint{
			Epoch: 0,
			Root:  targetRoot[:],
		},
	}

	if !proto.Equal(res, expectedInfo) {
		t.Errorf("Expected attestation info to match, received %v, wanted %v", res, expectedInfo)
	}
}

func TestGetAttestationData_CommitteeIndexIsZeroPostElectra(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	block := util.NewBeaconBlock()
	block.Block.Slot = 3*params.BeaconConfig().SlotsPerEpoch + 1
	targetBlock := util.NewBeaconBlock()
	targetBlock.Block.Slot = params.BeaconConfig().SlotsPerEpoch
	targetRoot, err := targetBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot = 2 * params.BeaconConfig().SlotsPerEpoch
	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err)
	justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(slot))
	justifiedCheckpoint := &ethpb.Checkpoint{
		Epoch: 2,
		Root:  justifiedRoot[:],
	}
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(justifiedCheckpoint))
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			HeadFetcher: &mock.ChainService{TargetRoot: targetRoot, Root: blockRoot[:], State: beaconState},
			GenesisTimeFetcher: &mock.ChainService{
				Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second),
			},
			FinalizedFetcher:      &mock.ChainService{CurrentJustifiedCheckPoint: justifiedCheckpoint},
			AttestationCache:      cache.NewAttestationDataCache(),
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		},
	}

	req := &ethpb.AttestationDataRequest{
		CommitteeIndex: 123, // set non-zero committee index
		Slot:           3*params.BeaconConfig().SlotsPerEpoch + 1,
	}
	res, err := attesterServer.GetAttestationData(t.Context(), req)
	require.NoError(t, err)

	expected := &ethpb.AttestationData{
		Slot:            3*params.BeaconConfig().SlotsPerEpoch + 1,
		CommitteeIndex:  0,
		BeaconBlockRoot: blockRoot[:],
		Source: &ethpb.Checkpoint{
			Epoch: 2,
			Root:  justifiedRoot[:],
		},
		Target: &ethpb.Checkpoint{
			Epoch: 3,
			Root:  targetRoot[:],
		},
	}

	assert.DeepEqual(t, expected, res)
}

func TestGetAttestationData_CommitteeIndexGloas(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ElectraForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	block := util.NewBeaconBlock()
	block.Block.Slot = 3*params.BeaconConfig().SlotsPerEpoch + 1
	targetBlock := util.NewBeaconBlock()
	targetBlock.Block.Slot = params.BeaconConfig().SlotsPerEpoch
	targetRoot, err := targetBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot = 2 * params.BeaconConfig().SlotsPerEpoch
	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err)
	justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(slot))
	justifiedCheckpoint := &ethpb.Checkpoint{
		Epoch: 2,
		Root:  justifiedRoot[:],
	}
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(justifiedCheckpoint))
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))

	t.Run("full payload returns index 1", func(t *testing.T) {
		headSlot := slot
		attesterServer := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
			TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
			CoreService: &core.Service{
				HeadFetcher: &mock.ChainService{
					TargetRoot:   targetRoot,
					Root:         blockRoot[:],
					State:        beaconState,
					MockHeadSlot: &headSlot,
				},
				ChainInfoFetcher: &mock.ChainService{
					MockCanonicalRoots: map[primitives.Slot][32]byte{slot: blockRoot},
					MockCanonicalFull:  map[primitives.Slot]bool{slot: true},
				},
				GenesisTimeFetcher: &mock.ChainService{
					Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second),
				},
				FinalizedFetcher:      &mock.ChainService{CurrentJustifiedCheckPoint: justifiedCheckpoint},
				AttestationCache:      cache.NewAttestationDataCache(),
				OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
			},
		}
		res, err := attesterServer.GetAttestationData(t.Context(), &ethpb.AttestationDataRequest{
			CommitteeIndex: 123,
			Slot:           slot,
		})
		require.NoError(t, err)
		assert.Equal(t, primitives.CommitteeIndex(1), res.CommitteeIndex)
	})

	t.Run("no full payload returns index 0", func(t *testing.T) {
		headSlot := slot
		attesterServer := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
			TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
			CoreService: &core.Service{
				HeadFetcher: &mock.ChainService{
					TargetRoot:   targetRoot,
					Root:         blockRoot[:],
					State:        beaconState,
					MockHeadSlot: &headSlot,
				},
				ChainInfoFetcher: &mock.ChainService{
					MockCanonicalRoots: map[primitives.Slot][32]byte{slot: blockRoot},
				},
				GenesisTimeFetcher: &mock.ChainService{
					Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second),
				},
				FinalizedFetcher:      &mock.ChainService{CurrentJustifiedCheckPoint: justifiedCheckpoint},
				AttestationCache:      cache.NewAttestationDataCache(),
				OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
			},
		}
		res, err := attesterServer.GetAttestationData(t.Context(), &ethpb.AttestationDataRequest{
			CommitteeIndex: 123,
			Slot:           slot,
		})
		require.NoError(t, err)
		assert.Equal(t, primitives.CommitteeIndex(0), res.CommitteeIndex)
	})
}

func TestServer_SubscribeCommitteeSubnets_InvalidRequest(t *testing.T) {
	tests := []struct {
		name string
		req  *ethpb.CommitteeSubnetsSubscribeRequest
		err  string
	}{
		{
			name: "no slots provided",
			req:  &ethpb.CommitteeSubnetsSubscribeRequest{},
			err:  "no attester slots provided",
		},
		{
			name: "fields not the same length",
			req: &ethpb.CommitteeSubnetsSubscribeRequest{
				Slots:        []primitives.Slot{1, 2},
				CommitteeIds: []primitives.CommitteeIndex{0},
				IsAggregator: []bool{false},
			},
			err: "request fields are not the same length",
		},
		{
			name: "validator_indices length does not match slots",
			req: &ethpb.CommitteeSubnetsSubscribeRequest{
				Slots:            []primitives.Slot{1, 2},
				CommitteeIds:     []primitives.CommitteeIndex{0, 0},
				IsAggregator:     []bool{false, false},
				ValidatorIndices: []primitives.ValidatorIndex{7},
			},
			err: "validator_indices length must match slots length when provided",
		},
		{
			name: "committees_at_slot length does not match slots",
			req: &ethpb.CommitteeSubnetsSubscribeRequest{
				Slots:            []primitives.Slot{1, 2},
				CommitteeIds:     []primitives.CommitteeIndex{0, 0},
				IsAggregator:     []bool{false, false},
				CommitteesAtSlot: []uint64{1},
			},
			err: "committees_at_slot length must match slots length when provided",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attesterServer := &Server{
				HeadFetcher:               &mock.ChainService{},
				P2P:                       &mockp2p.MockBroadcaster{},
				AttPool:                   attestations.NewPool(),
				OperationNotifier:         (&mock.ChainService{}).OperationNotifier(),
				SubscribedValidatorsCache: cache.NewSubscribedValidatorsCache(),
			}
			_, err := attesterServer.SubscribeCommitteeSubnets(t.Context(), tt.req)
			assert.ErrorContains(t, tt.err, err)
			assert.Equal(t, false, attesterServer.SubscribedValidatorsCache.Has(7))
		})
	}
}

func TestServer_SubscribeCommitteeSubnets_TracksValidatorIndices(t *testing.T) {
	validators := make([]*ethpb.Validator, 64)
	for i := range validators {
		validators[i] = &ethpb.Validator{
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		}
	}
	state, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))

	attesterServer := &Server{
		HeadFetcher:               &mock.ChainService{State: state},
		P2P:                       &mockp2p.MockBroadcaster{},
		AttPool:                   attestations.NewPool(),
		OperationNotifier:         (&mock.ChainService{}).OperationNotifier(),
		SubscribedValidatorsCache: cache.NewSubscribedValidatorsCache(),
	}

	_, err = attesterServer.SubscribeCommitteeSubnets(t.Context(), &ethpb.CommitteeSubnetsSubscribeRequest{
		Slots:            []primitives.Slot{9000, 9001},
		CommitteeIds:     []primitives.CommitteeIndex{0, 1},
		IsAggregator:     []bool{false, true},
		ValidatorIndices: []primitives.ValidatorIndex{3, 11},
	})
	require.NoError(t, err)
	assert.Equal(t, true, attesterServer.SubscribedValidatorsCache.Has(3))
	assert.Equal(t, true, attesterServer.SubscribedValidatorsCache.Has(11))
}

func TestServer_SubscribeCommitteeSubnets_RejectsUnknownValidator(t *testing.T) {
	validators := make([]*ethpb.Validator, 64)
	for i := range validators {
		validators[i] = &ethpb.Validator{
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		}
	}
	state, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))

	attesterServer := &Server{
		HeadFetcher:               &mock.ChainService{State: state},
		P2P:                       &mockp2p.MockBroadcaster{},
		AttPool:                   attestations.NewPool(),
		OperationNotifier:         (&mock.ChainService{}).OperationNotifier(),
		SubscribedValidatorsCache: cache.NewSubscribedValidatorsCache(),
	}
	// Index 100 is out of bounds for the 64-validator head state.
	_, err = attesterServer.SubscribeCommitteeSubnets(t.Context(), &ethpb.CommitteeSubnetsSubscribeRequest{
		Slots:            []primitives.Slot{1},
		CommitteeIds:     []primitives.CommitteeIndex{0},
		IsAggregator:     []bool{false},
		ValidatorIndices: []primitives.ValidatorIndex{100},
	})
	require.ErrorContains(t, "Could not get validator", err)
	assert.Equal(t, false, attesterServer.SubscribedValidatorsCache.Has(100))
}

func TestServer_SubscribeCommitteeSubnets_MultipleSlots(t *testing.T) {
	// fixed seed
	s := rand.NewSource(10)
	randGen := rand.New(s)

	validators := make([]*ethpb.Validator, 64)
	for i := range validators {
		validators[i] = &ethpb.Validator{
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		}
	}

	state, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))

	attesterServer := &Server{
		HeadFetcher:       &mock.ChainService{State: state},
		P2P:               &mockp2p.MockBroadcaster{},
		AttPool:           attestations.NewPool(),
		OperationNotifier: (&mock.ChainService{}).OperationNotifier(),
	}

	var ss []primitives.Slot
	var comIdxs []primitives.CommitteeIndex
	var isAggregator []bool

	for i := primitives.Slot(100); i < 200; i++ {
		ss = append(ss, i)
		comIdxs = append(comIdxs, primitives.CommitteeIndex(randGen.Int63n(64)))
		boolVal := randGen.Uint64()%2 == 0
		isAggregator = append(isAggregator, boolVal)
	}

	_, err = attesterServer.SubscribeCommitteeSubnets(t.Context(), &ethpb.CommitteeSubnetsSubscribeRequest{
		Slots:        ss,
		CommitteeIds: comIdxs,
		IsAggregator: isAggregator,
	})
	require.NoError(t, err)
	for i := primitives.Slot(100); i < 200; i++ {
		subnets := cache.SubnetIDs.GetAttesterSubnetIDs(i)
		assert.Equal(t, 1, len(subnets))
		if isAggregator[i-100] {
			subnets = cache.SubnetIDs.GetAggregatorSubnetIDs(i)
			assert.Equal(t, 1, len(subnets))
		}
	}
}
