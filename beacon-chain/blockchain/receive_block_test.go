package blockchain

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	blockchainTesting "github.com/prysmaticlabs/prysm/v5/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	statefeed "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/feed/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/das"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/operations/voluntaryexits"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpbv1 "github.com/prysmaticlabs/prysm/v5/proto/eth/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestService_ReceiveBlock(t *testing.T) {
	ctx := context.Background()

	genesis, keys := util.DeterministicGenesisState(t, 64)
	copiedGen := genesis.Copy()
	genFullBlock := func(t *testing.T, conf *util.BlockGenConfig, slot primitives.Slot) *ethpb.SignedBeaconBlock {
		blk, err := util.GenerateFullBlock(copiedGen.Copy(), keys, conf, slot)
		require.NoError(t, err)
		return blk
	}
	//params.SetupTestConfigCleanupWithLock(t)
	bc := params.BeaconConfig().Copy()
	bc.ShardCommitteePeriod = 0 // Required for voluntary exits test in reasonable time.
	params.OverrideBeaconConfig(bc)

	type args struct {
		block *ethpb.SignedBeaconBlock
	}
	tests := []struct {
		name      string
		args      args
		wantedErr string
		check     func(*testing.T, *Service)
	}{
		{
			name: "applies block with state transition",
			args: args{
				block: genFullBlock(t, util.DefaultBlockGenConfig(), 2 /*slot*/),
			},
			check: func(t *testing.T, s *Service) {
				if hs := s.head.state.Slot(); hs != 2 {
					t.Errorf("Unexpected state slot. Got %d but wanted %d", hs, 2)
				}
				if bs := s.head.block.Block().Slot(); bs != 2 {
					t.Errorf("Unexpected head block slot. Got %d but wanted %d", bs, 2)
				}
			},
		},
		{
			name: "saves attestations to pool",
			args: args{
				block: genFullBlock(t,
					&util.BlockGenConfig{
						NumProposerSlashings: 0,
						NumAttesterSlashings: 0,
						NumAttestations:      2,
						NumDeposits:          0,
						NumVoluntaryExits:    0,
					},
					1, /*slot*/
				),
			},
			check: func(t *testing.T, s *Service) {
				if baCount := len(s.cfg.AttPool.BlockAttestations()); baCount != 0 {
					t.Errorf("Did not get the correct number of block attestations saved to the pool. "+
						"Got %d but wanted %d", baCount, 0)
				}
			},
		},
		{
			name: "updates exit pool",
			args: args{
				block: genFullBlock(t, &util.BlockGenConfig{
					NumProposerSlashings: 0,
					NumAttesterSlashings: 0,
					NumAttestations:      0,
					NumDeposits:          0,
					NumVoluntaryExits:    3,
				},
					1, /*slot*/
				),
			},
			check: func(t *testing.T, s *Service) {
				pending, err := s.cfg.ExitPool.PendingExits()
				require.NoError(t, err)
				if len(pending) != 0 {
					t.Errorf(
						"Did not mark the correct number of exits. Got %d pending but wanted %d",
						len(pending),
						0,
					)
				}
			},
		},
		{
			name: "notifies block processed on state feed",
			args: args{
				block: genFullBlock(t, util.DefaultBlockGenConfig(), 1 /*slot*/),
			},
			check: func(t *testing.T, s *Service) {
				// Hacky sleep, should use a better way to be able to resolve the race
				// between event being sent out and processed.
				time.Sleep(100 * time.Millisecond)
				if recvd := len(s.cfg.StateNotifier.(*blockchainTesting.MockStateNotifier).ReceivedEvents()); recvd < 1 {
					t.Errorf("Received %d state notifications, expected at least 1", recvd)
				}
			},
		},
	}

	wg := new(sync.WaitGroup)
	for _, tt := range tests {
		wg.Add(1)
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				wg.Done()
			}()
			genesis = genesis.Copy()
			s, tr := minimalTestService(t,
				WithFinalizedStateAtStartUp(genesis),
				WithExitPool(voluntaryexits.NewPool()),
				WithStateNotifier(&blockchainTesting.MockStateNotifier{RecordEvents: true}),
				WithTrackedValidatorsCache(cache.NewTrackedValidatorsCache()),
			)

			beaconDB := tr.db
			genesisBlockRoot := bytesutil.ToBytes32(nil)
			require.NoError(t, beaconDB.SaveState(ctx, genesis, genesisBlockRoot))

			// Initialize it here.
			_ = s.cfg.StateNotifier.StateFeed()
			require.NoError(t, s.saveGenesisData(ctx, genesis))
			root, err := tt.args.block.Block.HashTreeRoot()
			require.NoError(t, err)
			wsb, err := blocks.NewSignedBeaconBlock(tt.args.block)
			require.NoError(t, err)
			err = s.ReceiveBlock(ctx, wsb, root, nil)
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				require.NoError(t, err)
				tt.check(t, s)
			}
		})
	}
	wg.Wait()
}

func TestService_ReceiveBlockUpdateHead(t *testing.T) {
	s, tr := minimalTestService(t,
		WithExitPool(voluntaryexits.NewPool()),
		WithStateNotifier(&blockchainTesting.MockStateNotifier{RecordEvents: true}))
	ctx, beaconDB := tr.ctx, tr.db
	genesis, keys := util.DeterministicGenesisState(t, 64)
	b, err := util.GenerateFullBlock(genesis, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)
	genesisBlockRoot := bytesutil.ToBytes32(nil)
	require.NoError(t, beaconDB.SaveState(ctx, genesis, genesisBlockRoot))

	// Initialize it here.
	_ = s.cfg.StateNotifier.StateFeed()
	require.NoError(t, s.saveGenesisData(ctx, genesis))
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		require.NoError(t, s.ReceiveBlock(ctx, wsb, root, nil))
		wg.Done()
	}()
	wg.Wait()
	time.Sleep(100 * time.Millisecond)
	if recvd := len(s.cfg.StateNotifier.(*blockchainTesting.MockStateNotifier).ReceivedEvents()); recvd < 1 {
		t.Errorf("Received %d state notifications, expected at least 1", recvd)
	}
	// Verify fork choice has processed the block. (Genesis block and the new block)
	assert.Equal(t, 2, s.cfg.ForkChoiceStore.NodeCount())
}

func TestService_ReceiveBlockBatch(t *testing.T) {
	ctx := context.Background()

	genesis, keys := util.DeterministicGenesisState(t, 64)
	genFullBlock := func(t *testing.T, conf *util.BlockGenConfig, slot primitives.Slot) *ethpb.SignedBeaconBlock {
		blk, err := util.GenerateFullBlock(genesis, keys, conf, slot)
		assert.NoError(t, err)
		return blk
	}

	type args struct {
		block *ethpb.SignedBeaconBlock
	}
	tests := []struct {
		name      string
		args      args
		wantedErr string
		check     func(*testing.T, *Service)
	}{
		{
			name: "applies block with state transition",
			args: args{
				block: genFullBlock(t, util.DefaultBlockGenConfig(), 2 /*slot*/),
			},
			check: func(t *testing.T, s *Service) {
				assert.Equal(t, primitives.Slot(2), s.head.state.Slot(), "Incorrect head state slot")
				assert.Equal(t, primitives.Slot(2), s.head.block.Block().Slot(), "Incorrect head block slot")
			},
		},
		{
			name: "notifies block processed on state feed",
			args: args{
				block: genFullBlock(t, util.DefaultBlockGenConfig(), 1 /*slot*/),
			},
			check: func(t *testing.T, s *Service) {
				time.Sleep(100 * time.Millisecond)
				if recvd := len(s.cfg.StateNotifier.(*blockchainTesting.MockStateNotifier).ReceivedEvents()); recvd < 1 {
					t.Errorf("Received %d state notifications, expected at least 1", recvd)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := minimalTestService(t, WithStateNotifier(&blockchainTesting.MockStateNotifier{RecordEvents: true}))
			err := s.saveGenesisData(ctx, genesis)
			require.NoError(t, err)
			wsb, err := blocks.NewSignedBeaconBlock(tt.args.block)
			require.NoError(t, err)
			rwsb, err := blocks.NewROBlock(wsb)
			require.NoError(t, err)
			err = s.ReceiveBlockBatch(ctx, []blocks.ROBlock{rwsb}, &das.MockAvailabilityStore{})
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
				tt.check(t, s)
			}
		})
	}
}

func TestService_HasBlock(t *testing.T) {
	s, _ := minimalTestService(t)
	r := [32]byte{'a'}
	if s.HasBlock(context.Background(), r) {
		t.Error("Should not have block")
	}
	wsb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	require.NoError(t, s.saveInitSyncBlock(context.Background(), r, wsb))
	if !s.HasBlock(context.Background(), r) {
		t.Error("Should have block")
	}
	b := util.NewBeaconBlock()
	b.Block.Slot = 1
	util.SaveBlock(t, context.Background(), s.cfg.BeaconDB, b)
	r, err = b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, true, s.HasBlock(context.Background(), r))
	s.blockBeingSynced.set(r)
	require.Equal(t, false, s.HasBlock(context.Background(), r))
}

func TestCheckSaveHotStateDB_Enabling(t *testing.T) {
	hook := logTest.NewGlobal()
	s, _ := minimalTestService(t)
	st := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epochsSinceFinalitySaveHotStateDB))
	s.genesisTime = time.Now().Add(time.Duration(-1*int64(st)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	require.NoError(t, s.checkSaveHotStateDB(context.Background()))
	assert.LogsContain(t, hook, "Entering mode to save hot states in DB")
}

func TestCheckSaveHotStateDB_Disabling(t *testing.T) {
	hook := logTest.NewGlobal()

	s, _ := minimalTestService(t)

	st := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epochsSinceFinalitySaveHotStateDB))
	s.genesisTime = time.Now().Add(time.Duration(-1*int64(st)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)
	require.NoError(t, s.checkSaveHotStateDB(context.Background()))
	s.genesisTime = time.Now()

	require.NoError(t, s.checkSaveHotStateDB(context.Background()))
	assert.LogsContain(t, hook, "Exiting mode to save hot states in DB")
}

func TestCheckSaveHotStateDB_Overflow(t *testing.T) {
	hook := logTest.NewGlobal()
	s, _ := minimalTestService(t)
	s.genesisTime = time.Now()

	require.NoError(t, s.checkSaveHotStateDB(context.Background()))
	assert.LogsDoNotContain(t, hook, "Entering mode to save hot states in DB")
}

func TestHandleCaches_EnablingLargeSize(t *testing.T) {
	hook := logTest.NewGlobal()
	s, _ := minimalTestService(t)
	st := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epochsSinceFinalitySaveHotStateDB))
	s.genesisTime = time.Now().Add(time.Duration(-1*int64(st)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	require.NoError(t, s.handleCaches())
	assert.LogsContain(t, hook, "Expanding committee cache size")
}

func TestHandleCaches_DisablingLargeSize(t *testing.T) {
	hook := logTest.NewGlobal()
	s, _ := minimalTestService(t)

	st := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epochsSinceFinalitySaveHotStateDB))
	s.genesisTime = time.Now().Add(time.Duration(-1*int64(st)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)
	require.NoError(t, s.handleCaches())
	s.genesisTime = time.Now()

	require.NoError(t, s.handleCaches())
	assert.LogsContain(t, hook, "Reducing committee cache size")
}

func TestHandleBlockBLSToExecutionChanges(t *testing.T) {
	service, tr := minimalTestService(t)
	pool := tr.blsPool

	t.Run("pre Capella block", func(t *testing.T) {
		body := &ethpb.BeaconBlockBodyBellatrix{}
		pbb := &ethpb.BeaconBlockBellatrix{
			Body: body,
		}
		blk, err := blocks.NewBeaconBlock(pbb)
		require.NoError(t, err)
		require.NoError(t, service.markIncludedBlockBLSToExecChanges(blk))
	})

	t.Run("Post Capella no changes", func(t *testing.T) {
		body := &ethpb.BeaconBlockBodyCapella{}
		pbb := &ethpb.BeaconBlockCapella{
			Body: body,
		}
		blk, err := blocks.NewBeaconBlock(pbb)
		require.NoError(t, err)
		require.NoError(t, service.markIncludedBlockBLSToExecChanges(blk))
	})

	t.Run("Post Capella some changes", func(t *testing.T) {
		idx := primitives.ValidatorIndex(123)
		change := &ethpb.BLSToExecutionChange{
			ValidatorIndex: idx,
		}
		signedChange := &ethpb.SignedBLSToExecutionChange{
			Message: change,
		}
		body := &ethpb.BeaconBlockBodyCapella{
			BlsToExecutionChanges: []*ethpb.SignedBLSToExecutionChange{signedChange},
		}
		pbb := &ethpb.BeaconBlockCapella{
			Body: body,
		}
		blk, err := blocks.NewBeaconBlock(pbb)
		require.NoError(t, err)

		pool.InsertBLSToExecChange(signedChange)
		require.Equal(t, true, pool.ValidatorExists(idx))
		require.NoError(t, service.markIncludedBlockBLSToExecChanges(blk))
		require.Equal(t, false, pool.ValidatorExists(idx))
	})
}

func Test_sendNewFinalizedEvent(t *testing.T) {
	s, _ := minimalTestService(t)
	notifier := &blockchainTesting.MockStateNotifier{RecordEvents: true}
	s.cfg.StateNotifier = notifier
	finalizedSt, err := util.NewBeaconState()
	require.NoError(t, err)
	finalizedStRoot, err := finalizedSt.HashTreeRoot(s.ctx)
	require.NoError(t, err)
	b := util.NewBeaconBlock()
	b.Block.StateRoot = finalizedStRoot[:]
	sbb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	sbbRoot, err := sbb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, s.cfg.BeaconDB.SaveBlock(s.ctx, sbb))
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: 123,
		Root:  sbbRoot[:],
	}))

	s.sendNewFinalizedEvent(s.ctx, st)

	require.Equal(t, 1, len(notifier.ReceivedEvents()))
	e := notifier.ReceivedEvents()[0]
	assert.Equal(t, statefeed.FinalizedCheckpoint, int(e.Type))
	fc, ok := e.Data.(*ethpbv1.EventFinalizedCheckpoint)
	require.Equal(t, true, ok, "event has wrong data type")
	assert.Equal(t, primitives.Epoch(123), fc.Epoch)
	assert.DeepEqual(t, sbbRoot[:], fc.Block)
	assert.DeepEqual(t, finalizedStRoot[:], fc.State)
	assert.Equal(t, false, fc.ExecutionOptimistic)
}

func Test_executePostFinalizationTasks(t *testing.T) {
	logHook := logTest.NewGlobal()

	headState, err := util.NewBeaconStateElectra()
	require.NoError(t, err)
	finalizedStRoot, err := headState.HashTreeRoot(context.Background())
	require.NoError(t, err)

	genesis := util.NewBeaconBlock()
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	finalizedSlot := params.BeaconConfig().SlotsPerEpoch*122 + 1
	headBlock := util.NewBeaconBlock()
	headBlock.Block.Slot = finalizedSlot
	headBlock.Block.StateRoot = finalizedStRoot[:]
	headBlock.Block.ParentRoot = bytesutil.PadTo(genesisRoot[:], 32)
	headRoot, err := headBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	hexKey := "0x93247f2209abcacf57b75a51dafae777f9dd38bc7053d1af526f220a7489a6d3a2753e5f3e8b1cfe39b56f43611df74a"
	key, err := hexutil.Decode(hexKey)
	require.NoError(t, err)
	require.NoError(t, headState.SetValidators([]*ethpb.Validator{
		{
			PublicKey:             key,
			WithdrawalCredentials: make([]byte, fieldparams.RootLength),
		},
	}))
	require.NoError(t, headState.SetSlot(finalizedSlot))
	require.NoError(t, headState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: 123,
		Root:  headRoot[:],
	}))
	require.NoError(t, headState.SetGenesisValidatorsRoot(params.BeaconConfig().ZeroHash[:]))

	s, tr := minimalTestService(t, WithFinalizedStateAtStartUp(headState))
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

	notifier := &blockchainTesting.MockStateNotifier{RecordEvents: true}
	s.cfg.StateNotifier = notifier
	s.executePostFinalizationTasks(s.ctx, headState)

	time.Sleep(1 * time.Second) // sleep for a second because event is in a separate go routine
	require.Equal(t, 1, len(notifier.ReceivedEvents()))
	e := notifier.ReceivedEvents()[0]
	assert.Equal(t, statefeed.FinalizedCheckpoint, int(e.Type))
	fc, ok := e.Data.(*ethpbv1.EventFinalizedCheckpoint)
	require.Equal(t, true, ok, "event has wrong data type")
	assert.Equal(t, primitives.Epoch(123), fc.Epoch)
	assert.DeepEqual(t, headRoot[:], fc.Block)
	assert.DeepEqual(t, finalizedStRoot[:], fc.State)
	assert.Equal(t, false, fc.ExecutionOptimistic)

	// check the cache
	index, ok := headState.ValidatorIndexByPubkey(bytesutil.ToBytes48(key))
	require.Equal(t, true, ok)
	require.Equal(t, primitives.ValidatorIndex(0), index) // first index

	// check deposit
	require.LogsContain(t, logHook, "Finalized deposit insertion completed at index")
}
