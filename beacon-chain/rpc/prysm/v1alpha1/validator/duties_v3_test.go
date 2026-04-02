package validator

import (
	"testing"
	"time"

	mockChain "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/gloas"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	dbutil "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/core"
	mockSync "github.com/OffchainLabs/prysm/v7/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetAttesterDuties_OK(t *testing.T) {
	genesis := util.NewBeaconBlock()
	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err)
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)

	db := dbutil.SetupDB(t)
	require.NoError(t, db.SaveGenesisBlockRoot(t.Context(), genesisRoot))

	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now(),
	}
	vs := &Server{
		HeadFetcher:           chain,
		TimeFetcher:           chain,
		OptimisticModeFetcher: chain,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		BeaconDB:              db,
		CoreService:           &core.Service{},
	}

	req := &ethpb.AttesterDutiesRequest{
		Epoch:            0,
		ValidatorIndices: []primitives.ValidatorIndex{0, 1},
	}
	res, err := vs.GetAttesterDuties(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, 2, len(res.Duties))
	assert.Equal(t, false, res.ExecutionOptimistic)
	assert.NotNil(t, res.DependentRoot)
	for _, d := range res.Duties {
		assert.NotNil(t, d.Pubkey)
		assert.Equal(t, true, d.Slot < params.BeaconConfig().SlotsPerEpoch)
	}
}

func TestGetAttesterDuties_Syncing(t *testing.T) {
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}
	_, err := vs.GetAttesterDuties(t.Context(), &ethpb.AttesterDutiesRequest{})
	assert.ErrorContains(t, "Syncing to latest head", err)
}

func TestGetAttesterDuties_EpochOutOfBound(t *testing.T) {
	chain := &mockChain.ChainService{Genesis: time.Now()}
	vs := &Server{
		TimeFetcher: chain,
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}
	currentEpoch := primitives.Epoch(chain.CurrentSlot() / params.BeaconConfig().SlotsPerEpoch)
	req := &ethpb.AttesterDutiesRequest{Epoch: currentEpoch + 2}
	_, err := vs.GetAttesterDuties(t.Context(), req)
	assert.ErrorContains(t, "can not be greater than next epoch", err)
}

func TestGetProposerDutiesV2_OK(t *testing.T) {
	genesis := util.NewBeaconBlock()
	depChainStart := params.BeaconConfig().MinGenesisActiveValidatorCount
	deposits, _, err := util.DeterministicDepositsAndKeys(depChainStart)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := transition.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err)
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)

	db := dbutil.SetupDB(t)
	require.NoError(t, db.SaveGenesisBlockRoot(t.Context(), genesisRoot))

	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now(),
	}
	vs := &Server{
		HeadFetcher:           chain,
		TimeFetcher:           chain,
		OptimisticModeFetcher: chain,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		BeaconDB:              db,
		CoreService:           &core.Service{},
	}

	req := &ethpb.ProposerDutiesRequest{Epoch: 0}
	res, err := vs.GetProposerDutiesV2(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, true, len(res.Duties) > 0)
	assert.Equal(t, false, res.ExecutionOptimistic)
	assert.NotNil(t, res.DependentRoot)
	for _, d := range res.Duties {
		assert.NotNil(t, d.Pubkey)
		assert.Equal(t, true, d.Slot < params.BeaconConfig().SlotsPerEpoch)
	}
}

func TestGetProposerDutiesV2_DependentRoot(t *testing.T) {
	helpers.ClearCache()
	spe := params.BeaconConfig().SlotsPerEpoch

	genesisRoot := [32]byte{0xff}

	t.Run("pre-Fulu epoch 1 computes dependent root", func(t *testing.T) {
		helpers.ClearCache()
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.ElectraForkEpoch = 0
		cfg.FuluForkEpoch = 1000
		params.OverrideBeaconConfig(cfg)

		bs, _ := util.DeterministicGenesisStateElectra(t, 64)
		roots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
		for i := range roots {
			roots[i] = make([]byte, 32)
			roots[i][0] = byte(i)
		}
		require.NoError(t, bs.SetBlockRoots(roots))
		require.NoError(t, bs.SetSlot(spe)) // epoch 1 start

		db := dbutil.SetupDB(t)
		require.NoError(t, db.SaveGenesisBlockRoot(t.Context(), genesisRoot))

		secondsPerSlot := params.BeaconConfig().SecondsPerSlot
		chain := &mockChain.ChainService{
			State:   bs,
			Root:    genesisRoot[:],
			Genesis: time.Now().Add(-time.Duration(uint64(spe)*secondsPerSlot) * time.Second),
		}
		vs := &Server{
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BeaconDB:              db,
			CoreService:           &core.Service{},
		}

		res, err := vs.GetProposerDutiesV2(t.Context(), &ethpb.ProposerDutiesRequest{Epoch: 1})
		require.NoError(t, err)
		// Pre-Fulu: ProposalDependentRoot uses epoch_start-1 = spe-1.
		assert.Equal(t, byte(spe-1), res.DependentRoot[0])
	})

	t.Run("post-Fulu epoch 1 uses genesis root", func(t *testing.T) {
		helpers.ClearCache()
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig().Copy()
		cfg.FuluForkEpoch = 0
		params.OverrideBeaconConfig(cfg)

		bs, _ := util.DeterministicGenesisStateFulu(t, 64)
		roots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
		for i := range roots {
			roots[i] = make([]byte, 32)
			roots[i][0] = byte(i)
		}
		require.NoError(t, bs.SetBlockRoots(roots))
		require.NoError(t, bs.SetSlot(spe)) // epoch 1 start

		db := dbutil.SetupDB(t)
		require.NoError(t, db.SaveGenesisBlockRoot(t.Context(), genesisRoot))

		secondsPerSlot := params.BeaconConfig().SecondsPerSlot
		chain := &mockChain.ChainService{
			State:   bs,
			Root:    genesisRoot[:],
			Genesis: time.Now().Add(-time.Duration(uint64(spe)*secondsPerSlot) * time.Second),
		}
		vs := &Server{
			HeadFetcher:           chain,
			TimeFetcher:           chain,
			OptimisticModeFetcher: chain,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BeaconDB:              db,
			CoreService:           &core.Service{},
		}

		res, err := vs.GetProposerDutiesV2(t.Context(), &ethpb.ProposerDutiesRequest{Epoch: 1})
		require.NoError(t, err)
		// Post-Fulu: epoch 1 uses genesis root from DB.
		assert.Equal(t, byte(0xff), res.DependentRoot[0])
	})
}

func TestGetProposerDutiesV2_Syncing(t *testing.T) {
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}
	_, err := vs.GetProposerDutiesV2(t.Context(), &ethpb.ProposerDutiesRequest{})
	assert.ErrorContains(t, "Syncing to latest head", err)
}

func TestGetProposerDutiesV2_EpochOutOfBound(t *testing.T) {
	chain := &mockChain.ChainService{Genesis: time.Now()}
	vs := &Server{
		TimeFetcher: chain,
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}
	currentEpoch := primitives.Epoch(chain.CurrentSlot() / params.BeaconConfig().SlotsPerEpoch)
	req := &ethpb.ProposerDutiesRequest{Epoch: currentEpoch + 2}
	_, err := vs.GetProposerDutiesV2(t.Context(), req)
	assert.ErrorContains(t, "can not be greater than next epoch", err)
}

func TestGetSyncCommitteeDuties_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = primitives.Epoch(0)
	params.OverrideBeaconConfig(cfg)

	deposits, _, err := util.DeterministicDepositsAndKeys(params.BeaconConfig().SyncCommitteeSize)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	bs, err := util.GenesisBeaconState(t.Context(), deposits, 0, eth1Data)
	require.NoError(t, err)

	h := &ethpb.BeaconBlockHeader{
		StateRoot:  bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength),
		ParentRoot: bytesutil.PadTo([]byte{'b'}, fieldparams.RootLength),
		BodyRoot:   bytesutil.PadTo([]byte{'c'}, fieldparams.RootLength),
	}
	require.NoError(t, bs.SetLatestBlockHeader(h))
	syncCommittee, err := altair.NextSyncCommittee(t.Context(), bs)
	require.NoError(t, err)
	require.NoError(t, bs.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, bs.SetSlot(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(params.BeaconConfig().EpochsPerSyncCommitteePeriod)-1))
	require.NoError(t, helpers.UpdateSyncCommitteeCache(bs))

	slot := uint64(params.BeaconConfig().SlotsPerEpoch) * uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * params.BeaconConfig().SecondsPerSlot
	genesisRoot := [32]byte{}
	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now().Add(time.Duration(-1*int64(slot-1)) * time.Second),
	}
	vs := &Server{
		HeadFetcher:           chain,
		TimeFetcher:           chain,
		OptimisticModeFetcher: chain,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		CoreService:           &core.Service{},
	}

	currentEpoch := primitives.Epoch(params.BeaconConfig().EpochsPerSyncCommitteePeriod - 1)
	req := &ethpb.SyncCommitteeDutiesRequest{
		Epoch:            currentEpoch,
		ValidatorIndices: []primitives.ValidatorIndex{0, 1},
	}
	res, err := vs.GetSyncCommitteeDuties(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, 2, len(res.Duties))
	assert.Equal(t, false, res.ExecutionOptimistic)
	for _, d := range res.Duties {
		assert.NotNil(t, d.Pubkey)
		assert.Equal(t, true, len(d.ValidatorSyncCommitteeIndices) > 0)
	}
}

func TestGetSyncCommitteeDuties_Syncing(t *testing.T) {
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}
	_, err := vs.GetSyncCommitteeDuties(t.Context(), &ethpb.SyncCommitteeDutiesRequest{})
	assert.ErrorContains(t, "Syncing to latest head", err)
}

func TestGetSyncCommitteeDuties_EpochOutOfBound(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = primitives.Epoch(0)
	cfg.EpochsPerSyncCommitteePeriod = 1
	params.OverrideBeaconConfig(cfg)

	chain := &mockChain.ChainService{Genesis: time.Now()}
	vs := &Server{
		TimeFetcher: chain,
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}
	currentEpoch := primitives.Epoch(chain.CurrentSlot() / params.BeaconConfig().SlotsPerEpoch)
	lastValid := core.SyncCommitteeDutiesLastValidEpoch(currentEpoch)
	req := &ethpb.SyncCommitteeDutiesRequest{Epoch: lastValid + 1}
	_, err := vs.GetSyncCommitteeDuties(t.Context(), req)
	assert.ErrorContains(t, "can not be greater than last valid epoch", err)
}

func TestGetPTCDuties_OK(t *testing.T) {
	helpers.ClearCache()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	numVals := uint64(fieldparams.PTCSize + 64)
	fuluSt, _ := util.DeterministicGenesisStateFulu(t, numVals)
	bs, err := gloas.UpgradeToGloas(fuluSt)
	require.NoError(t, err)
	require.NoError(t, helpers.UpdateCommitteeCache(t.Context(), bs, 0))

	genesisRoot := [32]byte{0xaa}
	db := dbutil.SetupDB(t)
	require.NoError(t, db.SaveGenesisBlockRoot(t.Context(), genesisRoot))

	chain := &mockChain.ChainService{
		State: bs, Root: genesisRoot[:], Genesis: time.Now(),
	}
	vs := &Server{
		HeadFetcher:           chain,
		TimeFetcher:           chain,
		OptimisticModeFetcher: chain,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		BeaconDB:              db,
		CoreService:           &core.Service{},
	}

	req := &ethpb.PTCDutiesRequest{
		Epoch:            0,
		ValidatorIndices: []primitives.ValidatorIndex{0, 1, 2, 3, 4},
	}
	res, err := vs.GetPTCDuties(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, false, res.ExecutionOptimistic)
	assert.NotNil(t, res.DependentRoot)
	for _, d := range res.Duties {
		assert.NotNil(t, d.Pubkey)
		assert.Equal(t, true, d.Slot < params.BeaconConfig().SlotsPerEpoch)
	}
}

func TestGetPTCDuties_Syncing(t *testing.T) {
	vs := &Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}
	_, err := vs.GetPTCDuties(t.Context(), &ethpb.PTCDutiesRequest{})
	assert.ErrorContains(t, "Syncing to latest head", err)
}

func TestGetPTCDuties_EpochOutOfBound(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)

	chain := &mockChain.ChainService{Genesis: time.Now()}
	vs := &Server{
		TimeFetcher: chain,
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}
	currentEpoch := primitives.Epoch(chain.CurrentSlot() / params.BeaconConfig().SlotsPerEpoch)
	req := &ethpb.PTCDutiesRequest{Epoch: currentEpoch + 2}
	_, err := vs.GetPTCDuties(t.Context(), req)
	assert.ErrorContains(t, "can not be greater than next epoch", err)
}

func TestGetPTCDuties_PreGloasFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.GloasForkEpoch = 100
	params.OverrideBeaconConfig(cfg)

	chain := &mockChain.ChainService{Genesis: time.Now()}
	vs := &Server{
		TimeFetcher: chain,
		SyncChecker: &mockSync.Sync{IsSyncing: false},
	}
	req := &ethpb.PTCDutiesRequest{Epoch: 0}
	_, err := vs.GetPTCDuties(t.Context(), req)
	s, ok := status.FromError(err)
	require.Equal(t, true, ok)
	assert.Equal(t, codes.InvalidArgument, s.Code())
	assert.ErrorContains(t, "before Gloas fork epoch", err)
}
