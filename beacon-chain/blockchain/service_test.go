package blockchain

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/cache/depositsnapshot"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/altair"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/execution"
	mockExecution "github.com/OffchainLabs/prysm/v6/beacon-chain/execution/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v6/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/slashings"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	state_native "github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/stategen"
	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	consensusblocks "github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/container/trie"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/genesis"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func setupBeaconChain(t *testing.T, beaconDB db.Database) *Service {
	ctx := t.Context()
	var web3Service *execution.Service
	var err error
	srv, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		srv.Stop()
	})
	bState, _ := util.DeterministicGenesisState(t, 10)
	genesis.StoreStateDuringTest(t, bState)
	pbState, err := state_native.ProtobufBeaconStatePhase0(bState.ToProtoUnsafe())
	require.NoError(t, err)
	mockTrie, err := trie.NewTrie(0)
	require.NoError(t, err)
	err = beaconDB.SaveExecutionChainData(ctx, &ethpb.ETH1ChainData{
		BeaconState: pbState,
		Trie:        mockTrie.ToProto(),
		CurrentEth1Data: &ethpb.LatestETH1Data{
			BlockHash: make([]byte, 32),
		},
		ChainstartData: &ethpb.ChainStartData{
			Eth1Data: &ethpb.Eth1Data{
				DepositRoot:  make([]byte, 32),
				DepositCount: 0,
				BlockHash:    make([]byte, 32),
			},
		},
		DepositContainers: []*ethpb.DepositContainer{},
	})
	require.NoError(t, err)

	depositCache, err := depositsnapshot.New()
	require.NoError(t, err)

	web3Service, err = execution.NewService(
		ctx,
		execution.WithDatabase(beaconDB),
		execution.WithHttpEndpoint(endpoint),
		execution.WithDepositContractAddress(common.Address{}),
		execution.WithDepositCache(depositCache),
	)
	require.NoError(t, err, "Unable to set up web3 service")

	attService, err := attestations.NewService(ctx, &attestations.Config{Pool: attestations.NewPool()})
	require.NoError(t, err)

	fc := doublylinkedtree.New()
	stateGen := stategen.New(beaconDB, fc)
	// Safe a state in stategen to purposes of testing a service stop / shutdown.
	require.NoError(t, stateGen.SaveState(ctx, bytesutil.ToBytes32(bState.FinalizedCheckpoint().Root), bState))

	opts := []Option{
		WithDatabase(beaconDB),
		WithDepositCache(depositCache),
		WithChainStartFetcher(web3Service),
		WithAttestationPool(attestations.NewPool()),
		WithSlashingPool(slashings.NewPool()),
		WithExitPool(voluntaryexits.NewPool()),
		WithP2PBroadcaster(&mockAccessor{}),
		WithStateNotifier(&mockBeaconNode{}),
		WithForkChoiceStore(fc),
		WithAttestationService(attService),
		WithStateGen(stateGen),
		WithPayloadIDCache(cache.NewPayloadIDCache()),
		WithClockSynchronizer(startup.NewClockSynchronizer()),
	}

	chainService, err := NewService(ctx, opts...)
	require.NoError(t, err, "Unable to setup chain service")
	chainService.genesisTime = time.Unix(1, 0) // non-zero time

	return chainService
}

func TestChainStartStop_Initialized(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	chainService := setupBeaconChain(t, beaconDB)

	gt := time.Unix(23, 0)
	genesisBlk := util.NewBeaconBlock()
	blkRoot, err := genesisBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, genesisBlk)
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.SetGenesisTime(gt))
	require.NoError(t, s.SetSlot(1))
	require.NoError(t, beaconDB.SaveState(ctx, s, blkRoot))
	require.NoError(t, beaconDB.SaveHeadBlockRoot(ctx, blkRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, blkRoot))
	require.NoError(t, beaconDB.SaveJustifiedCheckpoint(ctx, &ethpb.Checkpoint{Root: blkRoot[:]}))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Root: blkRoot[:]}))
	ss := &ethpb.StateSummary{
		Slot: 1,
		Root: blkRoot[:],
	}
	require.NoError(t, beaconDB.SaveStateSummary(ctx, ss))
	chainService.cfg.FinalizedStateAtStartUp = s
	// Test the start function.
	chainService.Start()

	require.NoError(t, chainService.Stop(), "Unable to stop chain service")

	// The context should have been canceled.
	assert.Equal(t, context.Canceled, chainService.ctx.Err(), "Context was not canceled")
	require.LogsContain(t, hook, "data already exists")
}

func TestChainStartStop_GenesisZeroHashes(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	chainService := setupBeaconChain(t, beaconDB)

	gt := time.Unix(23, 0)
	genesisBlk := util.NewBeaconBlock()
	blkRoot, err := genesisBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb := util.SaveBlock(t, ctx, beaconDB, genesisBlk)
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.SetGenesisTime(gt))
	require.NoError(t, beaconDB.SaveState(ctx, s, blkRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, blkRoot))
	require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
	require.NoError(t, beaconDB.SaveJustifiedCheckpoint(ctx, &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Root: blkRoot[:]}))
	chainService.cfg.FinalizedStateAtStartUp = s
	// Test the start function.
	chainService.Start()

	require.NoError(t, chainService.Stop(), "Unable to stop chain service")

	// The context should have been canceled.
	assert.Equal(t, context.Canceled, chainService.ctx.Err(), "Context was not canceled")
	require.LogsContain(t, hook, "data already exists")
}

func TestChainService_InitializeBeaconChain(t *testing.T) {
	helpers.ClearCache()
	beaconDB := testDB.SetupDB(t)
	ctx := t.Context()

	bc := setupBeaconChain(t, beaconDB)
	var err error

	// Set up 10 deposits pre chain start for validators to register
	count := uint64(10)
	deposits, _, err := util.DeterministicDepositsAndKeys(count)
	require.NoError(t, err)
	dt, _, err := util.DepositTrieFromDeposits(deposits)
	require.NoError(t, err)
	hashTreeRoot, err := dt.HashTreeRoot()
	require.NoError(t, err)
	genState, err := transition.EmptyGenesisState()
	require.NoError(t, err)
	err = genState.SetEth1Data(&ethpb.Eth1Data{
		DepositRoot:  hashTreeRoot[:],
		DepositCount: uint64(len(deposits)),
		BlockHash:    make([]byte, 32),
	})
	require.NoError(t, err)
	genState, err = altair.ProcessPreGenesisDeposits(ctx, genState, deposits)
	require.NoError(t, err)

	_, err = bc.initializeBeaconChain(ctx, time.Unix(0, 0), genState, &ethpb.Eth1Data{DepositRoot: hashTreeRoot[:], BlockHash: make([]byte, 32)})
	require.NoError(t, err)

	_, err = bc.HeadState(ctx)
	assert.NoError(t, err)
	headBlk, err := bc.HeadBlock(ctx)
	require.NoError(t, err)
	if headBlk == nil {
		t.Error("Head state can't be nil after initialize beacon chain")
	}
	r, err := bc.HeadRoot(ctx)
	require.NoError(t, err)
	if bytesutil.ToBytes32(r) == params.BeaconConfig().ZeroHash {
		t.Error("Canonical root for slot 0 can't be zeros after initialize beacon chain")
	}
}

func TestChainService_CorrectGenesisRoots(t *testing.T) {
	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)

	chainService := setupBeaconChain(t, beaconDB)

	gt := time.Unix(23, 0)
	genesisBlk := util.NewBeaconBlock()
	blkRoot, err := genesisBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, genesisBlk)
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.SetGenesisTime(gt))
	require.NoError(t, s.SetSlot(0))
	require.NoError(t, beaconDB.SaveState(ctx, s, blkRoot))
	require.NoError(t, beaconDB.SaveHeadBlockRoot(ctx, blkRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, blkRoot))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Root: blkRoot[:]}))
	chainService.cfg.FinalizedStateAtStartUp = s
	// Test the start function.
	chainService.Start()

	cp := chainService.FinalizedCheckpt()
	require.DeepEqual(t, blkRoot[:], cp.Root, "Finalize Checkpoint root is incorrect")
	cp = chainService.CurrentJustifiedCheckpt()
	require.NoError(t, err)
	require.DeepEqual(t, params.BeaconConfig().ZeroHash[:], cp.Root, "Justified Checkpoint root is incorrect")

	require.NoError(t, chainService.Stop(), "Unable to stop chain service")

}

func TestChainService_InitializeChainInfo(t *testing.T) {
	genesis := util.NewBeaconBlock()
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)

	finalizedSlot := params.BeaconConfig().SlotsPerEpoch*2 + 1
	headBlock := util.NewBeaconBlock()
	headBlock.Block.Slot = finalizedSlot
	headBlock.Block.ParentRoot = bytesutil.PadTo(genesisRoot[:], 32)
	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(finalizedSlot))
	require.NoError(t, headState.SetGenesisValidatorsRoot(params.BeaconConfig().ZeroHash[:]))
	headRoot, err := headBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	c, tr := minimalTestService(t, WithFinalizedStateAtStartUp(headState))
	ctx, beaconDB, stateGen := tr.ctx, tr.db, tr.sg

	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))
	util.SaveBlock(t, ctx, beaconDB, genesis)
	require.NoError(t, beaconDB.SaveState(ctx, headState, headRoot))
	require.NoError(t, beaconDB.SaveState(ctx, headState, genesisRoot))
	util.SaveBlock(t, ctx, beaconDB, headBlock)
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: slots.ToEpoch(finalizedSlot), Root: headRoot[:]}))
	require.NoError(t, stateGen.SaveState(ctx, headRoot, headState))

	require.NoError(t, c.StartFromSavedState(headState))
	headBlk, err := c.HeadBlock(ctx)
	require.NoError(t, err)
	pb, err := headBlk.Proto()
	require.NoError(t, err)
	assert.DeepEqual(t, headBlock, pb, "Head block incorrect")
	s, err := c.HeadState(ctx)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, headState.ToProtoUnsafe(), s.ToProtoUnsafe(), "Head state incorrect")
	assert.Equal(t, c.HeadSlot(), headBlock.Block.Slot, "Head slot incorrect")
	r, err := c.HeadRoot(t.Context())
	require.NoError(t, err)
	if !bytes.Equal(headRoot[:], r) {
		t.Error("head slot incorrect")
	}
	assert.Equal(t, genesisRoot, c.originBlockRoot, "Genesis block root incorrect")
}

func TestChainService_InitializeChainInfo_SetHeadAtGenesis(t *testing.T) {
	genesis := util.NewBeaconBlock()
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)

	finalizedSlot := params.BeaconConfig().SlotsPerEpoch*2 + 1
	headBlock := util.NewBeaconBlock()
	headBlock.Block.Slot = finalizedSlot
	headBlock.Block.ParentRoot = bytesutil.PadTo(genesisRoot[:], 32)
	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(finalizedSlot))
	require.NoError(t, headState.SetGenesisValidatorsRoot(params.BeaconConfig().ZeroHash[:]))
	headRoot, err := headBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	c, tr := minimalTestService(t, WithFinalizedStateAtStartUp(headState))
	ctx, beaconDB := tr.ctx, tr.db

	util.SaveBlock(t, ctx, beaconDB, genesis)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))
	require.NoError(t, beaconDB.SaveState(ctx, headState, genesisRoot))
	require.NoError(t, beaconDB.SaveState(ctx, headState, headRoot))
	util.SaveBlock(t, ctx, beaconDB, headBlock)
	ss := &ethpb.StateSummary{
		Slot: finalizedSlot,
		Root: headRoot[:],
	}
	require.NoError(t, beaconDB.SaveStateSummary(ctx, ss))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Root: headRoot[:], Epoch: slots.ToEpoch(finalizedSlot)}))

	require.NoError(t, c.StartFromSavedState(headState))
	s, err := c.HeadState(ctx)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, headState.ToProtoUnsafe(), s.ToProtoUnsafe(), "Head state incorrect")
	assert.Equal(t, genesisRoot, c.originBlockRoot, "Genesis block root incorrect")
	pb, err := c.head.block.Proto()
	require.NoError(t, err)
	assert.DeepEqual(t, headBlock, pb)
}

func TestChainService_SaveHeadNoDB(t *testing.T) {
	ctx := t.Context()
	s := testServiceWithDB(t)
	blk := util.NewBeaconBlock()
	blk.Block.Slot = 1
	r, err := blk.HashTreeRoot()
	require.NoError(t, err)
	newState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.cfg.StateGen.SaveState(ctx, r, newState))
	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, s.saveHeadNoDB(ctx, wsb, r, newState, false))

	newB, err := s.cfg.BeaconDB.HeadBlock(ctx)
	require.NoError(t, err)
	if reflect.DeepEqual(newB, blk) {
		t.Error("head block should not be equal")
	}
}

func TestHasBlock_ForkChoiceAndDB_DoublyLinkedTree(t *testing.T) {
	ctx := t.Context()
	s := testServiceWithDB(t)
	b := util.NewBeaconBlock()
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, r)
	require.NoError(t, err)
	beaconState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.cfg.ForkChoiceStore.InsertNode(ctx, beaconState, roblock))

	assert.Equal(t, false, s.hasBlock(ctx, [32]byte{}), "Should not have block")
	assert.Equal(t, true, s.hasBlock(ctx, r), "Should have block")
}

func TestServiceStop_SaveCachedBlocks(t *testing.T) {
	s := testServiceWithDB(t)
	s.initSyncBlocks = make(map[[32]byte]interfaces.ReadOnlySignedBeaconBlock)
	bb := util.NewBeaconBlock()
	r, err := bb.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(bb)
	require.NoError(t, err)
	require.NoError(t, s.saveInitSyncBlock(s.ctx, r, wsb))
	require.NoError(t, s.Stop())
	require.Equal(t, true, s.cfg.BeaconDB.HasBlock(s.ctx, r))
}

func BenchmarkHasBlockDB(b *testing.B) {
	ctx := b.Context()
	s := testServiceWithDB(b)
	blk := util.NewBeaconBlock()
	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(b, err)
	require.NoError(b, s.cfg.BeaconDB.SaveBlock(ctx, wsb))
	r, err := blk.Block.HashTreeRoot()
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		require.Equal(b, true, s.cfg.BeaconDB.HasBlock(ctx, r), "Block is not in DB")
	}
}

func BenchmarkHasBlockForkChoiceStore_DoublyLinkedTree(b *testing.B) {
	ctx := b.Context()
	s := testServiceWithDB(b)
	blk := util.NewBeaconBlock()
	r, err := blk.Block.HashTreeRoot()
	require.NoError(b, err)
	beaconState, err := util.NewBeaconState()
	require.NoError(b, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(b, err)
	roblock, err := consensusblocks.NewROBlockWithRoot(wsb, r)
	require.NoError(b, err)
	require.NoError(b, s.cfg.ForkChoiceStore.InsertNode(ctx, beaconState, roblock))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		require.Equal(b, true, s.cfg.ForkChoiceStore.HasNode(r), "Block is not in fork choice store")
	}
}

func TestChainService_EverythingOptimistic(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableStartOptimistic: true,
	})
	defer resetFn()

	genesis := util.NewBeaconBlock()
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	finalizedSlot := params.BeaconConfig().SlotsPerEpoch*2 + 1
	headBlock := util.NewBeaconBlock()
	headBlock.Block.Slot = finalizedSlot
	headBlock.Block.ParentRoot = bytesutil.PadTo(genesisRoot[:], 32)
	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(finalizedSlot))
	require.NoError(t, headState.SetGenesisValidatorsRoot(params.BeaconConfig().ZeroHash[:]))
	headRoot, err := headBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	c, tr := minimalTestService(t, WithFinalizedStateAtStartUp(headState))
	ctx, beaconDB, stateGen := tr.ctx, tr.db, tr.sg

	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))
	util.SaveBlock(t, ctx, beaconDB, genesis)
	require.NoError(t, beaconDB.SaveState(ctx, headState, headRoot))
	require.NoError(t, beaconDB.SaveState(ctx, headState, genesisRoot))
	util.SaveBlock(t, ctx, beaconDB, headBlock)
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: slots.ToEpoch(finalizedSlot), Root: headRoot[:]}))

	require.NoError(t, err)
	require.NoError(t, stateGen.SaveState(ctx, headRoot, headState))
	require.NoError(t, beaconDB.SaveLastValidatedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: slots.ToEpoch(finalizedSlot), Root: headRoot[:]}))
	require.NoError(t, c.StartFromSavedState(headState))
	require.Equal(t, true, c.cfg.ForkChoiceStore.HasNode(headRoot))
	op, err := c.cfg.ForkChoiceStore.IsOptimistic(headRoot)
	require.NoError(t, err)
	require.Equal(t, true, op)
}

func TestStartFromSavedState_ValidatorIndexCacheUpdated(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableStartOptimistic: true,
	})
	defer resetFn()

	genesis := util.NewBeaconBlockElectra()
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	finalizedSlot := params.BeaconConfig().SlotsPerEpoch*2 + 1
	headBlock := util.NewBeaconBlockElectra()
	headBlock.Block.Slot = finalizedSlot
	headBlock.Block.ParentRoot = bytesutil.PadTo(genesisRoot[:], 32)
	headState, err := util.NewBeaconState()
	require.NoError(t, err)
	hexKey := "0x93247f2209abcacf57b75a51dafae777f9dd38bc7053d1af526f220a7489a6d3a2753e5f3e8b1cfe39b56f43611df74a"
	key, err := hexutil.Decode(hexKey)
	require.NoError(t, err)
	hexKey2 := "0x42247f2209abcacf57b75a51dafae777f9dd38bc7053d1af526f220a7489a6d3a2753e5f3e8b1cfe39b56f43611df74a"
	key2, err := hexutil.Decode(hexKey2)
	require.NoError(t, err)
	require.NoError(t, headState.SetValidators([]*ethpb.Validator{
		{
			PublicKey:             key,
			WithdrawalCredentials: make([]byte, fieldparams.RootLength),
		},
		{
			PublicKey:             key2,
			WithdrawalCredentials: make([]byte, fieldparams.RootLength),
		},
	}))
	require.NoError(t, headState.SetSlot(finalizedSlot))
	require.NoError(t, headState.SetGenesisValidatorsRoot(params.BeaconConfig().ZeroHash[:]))
	headRoot, err := headBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	c, tr := minimalTestService(t, WithFinalizedStateAtStartUp(headState))
	ctx, beaconDB, stateGen := tr.ctx, tr.db, tr.sg

	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))
	util.SaveBlock(t, ctx, beaconDB, genesis)
	require.NoError(t, beaconDB.SaveState(ctx, headState, headRoot))
	require.NoError(t, beaconDB.SaveState(ctx, headState, genesisRoot))
	util.SaveBlock(t, ctx, beaconDB, headBlock)
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: slots.ToEpoch(finalizedSlot), Root: headRoot[:]}))

	require.NoError(t, err)
	require.NoError(t, stateGen.SaveState(ctx, headRoot, headState))
	require.NoError(t, beaconDB.SaveLastValidatedCheckpoint(ctx, &ethpb.Checkpoint{Epoch: slots.ToEpoch(finalizedSlot), Root: headRoot[:]}))
	require.NoError(t, c.StartFromSavedState(headState))

	index, ok := headState.ValidatorIndexByPubkey(bytesutil.ToBytes48(key))
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ValidatorIndex(0), index) // first index
	index2, ok := headState.ValidatorIndexByPubkey(bytesutil.ToBytes48(key2))
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ValidatorIndex(1), index2) // first index
}

// MockClockSetter satisfies the ClockSetter interface for testing the conditions where blockchain.Service should
// call SetGenesis.
type MockClockSetter struct {
	G   *startup.Clock
	Err error
}

var _ startup.ClockSetter = &MockClockSetter{}

// SetClock satisfies the ClockSetter interface.
// The value is written to an exported field 'G' so that it can be accessed in tests.
func (s *MockClockSetter) SetClock(g *startup.Clock) error {
	s.G = g
	return s.Err
}

func TestNotifyIndex(t *testing.T) {
	// Initialize a blobNotifierMap
	bn := &blobNotifierMap{
		seenIndex: make(map[[32]byte][]bool),
		notifiers: make(map[[32]byte]chan uint64),
	}

	// Sample root and index
	var root [32]byte
	copy(root[:], "exampleRoot")

	ds := util.SlotAtEpoch(t, params.BeaconConfig().DenebForkEpoch)
	// Test notifying a new index
	bn.notifyIndex(root, 1, ds)
	if !bn.seenIndex[root][1] {
		t.Errorf("Index was not marked as seen")
	}

	// Test that a new channel is created
	if _, ok := bn.notifiers[root]; !ok {
		t.Errorf("Notifier channel was not created")
	}

	// Test notifying an already seen index
	bn.notifyIndex(root, 1, 1)
	if len(bn.notifiers[root]) > 1 {
		t.Errorf("Notifier channel should not receive multiple messages for the same index")
	}

	// Test notifying a new index again
	bn.notifyIndex(root, 2, ds)
	if !bn.seenIndex[root][2] {
		t.Errorf("Index was not marked as seen")
	}

	// Test that the notifier channel receives the index
	select {
	case idx := <-bn.notifiers[root]:
		if idx != 1 {
			t.Errorf("Received index on channel is incorrect")
		}
	default:
		t.Errorf("Notifier channel did not receive the index")
	}
}

func TestUpdateCustodyInfoInDB(t *testing.T) {
	const (
		fuluForkEpoch         = 10
		custodyRequirement    = uint64(4)
		earliestStoredSlot    = primitives.Slot(12)
		numberOfCustodyGroups = uint64(64)
		numberOfColumns       = uint64(128)
	)

	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.FuluForkEpoch = fuluForkEpoch
	cfg.CustodyRequirement = custodyRequirement
	cfg.NumberOfCustodyGroups = numberOfCustodyGroups
	cfg.NumberOfColumns = numberOfColumns
	params.OverrideBeaconConfig(cfg)

	ctx := t.Context()
	pbBlock := util.NewBeaconBlock()
	pbBlock.Block.Slot = 12
	signedBeaconBlock, err := blocks.NewSignedBeaconBlock(pbBlock)
	require.NoError(t, err)

	roBlock, err := blocks.NewROBlock(signedBeaconBlock)
	require.NoError(t, err)

	t.Run("CGC increases before fulu", func(t *testing.T) {
		service, requirements := minimalTestService(t)
		err = requirements.db.SaveBlock(ctx, roBlock)
		require.NoError(t, err)

		// Before Fulu
		// -----------
		actualEas, actualCgc, err := service.updateCustodyInfoInDB(15)
		require.NoError(t, err)
		require.Equal(t, earliestStoredSlot, actualEas)
		require.Equal(t, custodyRequirement, actualCgc)

		actualEas, actualCgc, err = service.updateCustodyInfoInDB(17)
		require.NoError(t, err)
		require.Equal(t, earliestStoredSlot, actualEas)
		require.Equal(t, custodyRequirement, actualCgc)

		resetFlags := flags.Get()
		gFlags := new(flags.GlobalFlags)
		gFlags.SubscribeAllDataSubnets = true
		flags.Init(gFlags)
		defer flags.Init(resetFlags)

		actualEas, actualCgc, err = service.updateCustodyInfoInDB(19)
		require.NoError(t, err)
		require.Equal(t, earliestStoredSlot, actualEas)
		require.Equal(t, numberOfCustodyGroups, actualCgc)

		// After Fulu
		// ----------
		actualEas, actualCgc, err = service.updateCustodyInfoInDB(fuluForkEpoch*primitives.Slot(cfg.SlotsPerEpoch) + 1)
		require.NoError(t, err)
		require.Equal(t, earliestStoredSlot, actualEas)
		require.Equal(t, numberOfCustodyGroups, actualCgc)
	})

	t.Run("CGC increases after fulu", func(t *testing.T) {
		service, requirements := minimalTestService(t)
		err = requirements.db.SaveBlock(ctx, roBlock)
		require.NoError(t, err)

		// Before Fulu
		// -----------
		actualEas, actualCgc, err := service.updateCustodyInfoInDB(15)
		require.NoError(t, err)
		require.Equal(t, earliestStoredSlot, actualEas)
		require.Equal(t, custodyRequirement, actualCgc)

		actualEas, actualCgc, err = service.updateCustodyInfoInDB(17)
		require.NoError(t, err)
		require.Equal(t, earliestStoredSlot, actualEas)
		require.Equal(t, custodyRequirement, actualCgc)

		// After Fulu
		// ----------
		resetFlags := flags.Get()
		gFlags := new(flags.GlobalFlags)
		gFlags.SubscribeAllDataSubnets = true
		flags.Init(gFlags)
		defer flags.Init(resetFlags)

		slot := fuluForkEpoch*primitives.Slot(cfg.SlotsPerEpoch) + 1
		actualEas, actualCgc, err = service.updateCustodyInfoInDB(slot)
		require.NoError(t, err)
		require.Equal(t, slot, actualEas)
		require.Equal(t, numberOfCustodyGroups, actualCgc)

		actualEas, actualCgc, err = service.updateCustodyInfoInDB(slot + 2)
		require.NoError(t, err)
		require.Equal(t, slot, actualEas)
		require.Equal(t, numberOfCustodyGroups, actualCgc)
	})
}
