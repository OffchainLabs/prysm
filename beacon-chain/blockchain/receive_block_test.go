package blockchain

import (
	"sync"
	"testing"
	"time"

	blockchainTesting "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	coreblocks "github.com/OffchainLabs/prysm/v7/beacon-chain/core/blocks"
	statefeed "github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/execution"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	forkchoicetypes "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/types"
	lightClient "github.com/OffchainLabs/prysm/v7/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/voluntaryexits"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common/hexutil"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestService_ReceiveBlock(t *testing.T) {
	ctx := t.Context()

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

	badBlock := genFullBlock(t, util.DefaultBlockGenConfig(), 101)
	badRoot, err := badBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	badRoots := make(map[[32]byte]struct{})
	badRoots[badRoot] = struct{}{}
	resetCfg := features.InitWithReset(&features.Flags{
		BlacklistedRoots: badRoots,
	})
	defer resetCfg()

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
				notifier := s.cfg.StateNotifier.(*blockchainTesting.MockStateNotifier)
				require.Eventually(t, func() bool {
					return len(notifier.ReceivedEvents()) >= 1
				}, 2*time.Second, 10*time.Millisecond, "Expected at least 1 state notification")
			},
		},
		{
			name: "The block is blacklisted",
			args: args{
				block: badBlock,
			},
			wantedErr: errBlacklistedRoot.Error(),
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
				WithProposerPreferencesCache(cache.NewProposerPreferencesCache()),
				WithSubscribedValidatorsCache(cache.NewSubscribedValidatorsCache()),
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

func TestService_ValidateBlockForEquivocation(t *testing.T) {
	genesis, keys := util.DeterministicGenesisState(t, 64)
	tests := []struct {
		name      string
		mutate    func(*testing.T, *ethpb.SignedBeaconBlock)
		wantedErr string
	}{
		{
			name: "valid block is not imported",
		},
		{
			name: "valid proposer signature with invalid state root",
			mutate: func(_ *testing.T, block *ethpb.SignedBeaconBlock) {
				block.Block.StateRoot[0] ^= 1
			},
			wantedErr: "could not validate state root",
		},
		{
			name: "valid proposer signature with invalid RANDAO reveal",
			mutate: func(t *testing.T, block *ethpb.SignedBeaconBlock) {
				block.Block.Body.RandaoReveal = keys[(int(block.Block.ProposerIndex)+1)%len(keys)].Sign(make([]byte, 32)).Marshal()
				_, err := util.BlockSignature(genesis, block.Block, keys)
				require.NoError(t, err)
			},
			wantedErr: "signature in block failed to verify",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			s, tr := minimalTestService(t)
			require.NoError(t, s.saveGenesisData(ctx, genesis.Copy()))
			block, err := util.GenerateFullBlock(genesis, keys, &util.BlockGenConfig{}, 1)
			require.NoError(t, err)
			if tt.mutate != nil {
				tt.mutate(t, block)
			}
			block.Signature, err = signing.ComputeDomainAndSign(genesis, slots.ToEpoch(block.Block.Slot), block.Block, params.BeaconConfig().DomainBeaconProposer, keys[block.Block.ProposerIndex])
			require.NoError(t, err)
			require.NoError(t, coreblocks.VerifyBlockSignature(genesis, block.Block.ProposerIndex, block.Signature, block.Block.HashTreeRoot))
			signed, err := blocks.NewSignedBeaconBlock(block)
			require.NoError(t, err)
			roblock, err := blocks.NewROBlock(signed)
			require.NoError(t, err)
			headRoot, err := s.HeadRoot(ctx)
			require.NoError(t, err)
			nodeCount := tr.fcs.NodeCount()

			err = s.ValidateBlockForEquivocation(ctx, roblock)
			if tt.wantedErr != "" {
				require.ErrorContains(t, tt.wantedErr, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, false, s.InForkchoice(roblock.Root()))
			assert.Equal(t, false, tr.db.HasBlock(ctx, roblock.Root()))
			assert.Equal(t, false, tr.db.HasState(ctx, roblock.Root()))
			assert.Equal(t, nodeCount, tr.fcs.NodeCount())
			gotHeadRoot, err := s.HeadRoot(ctx)
			require.NoError(t, err)
			assert.DeepEqual(t, headRoot, gotHeadRoot)
		})
	}
}

func TestService_ValidateBlockForEquivocation_Fulu(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch, cfg.BellatrixForkEpoch, cfg.CapellaForkEpoch = 0, 0, 0
	cfg.DenebForkEpoch, cfg.ElectraForkEpoch, cfg.FuluForkEpoch = 0, 0, 0
	params.OverrideBeaconConfig(cfg)
	genesis, keys := util.DeterministicGenesisStateFulu(t, 64)
	block, err := util.GenerateFullBlockFulu(genesis, keys, &util.BlockGenConfig{}, 1)
	require.NoError(t, err)
	signed, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	roblock, err := blocks.NewROBlock(signed)
	require.NoError(t, err)
	tests := []struct {
		name       string
		payloadErr error
		knownBlock bool
		wantedErr  string
	}{
		{
			name: "valid execution payload",
		},
		{
			name:       "optimistic block is not validation evidence",
			payloadErr: execution.ErrAcceptedSyncingPayloadStatus,
			knownBlock: true,
			wantedErr:  "execution payload is optimistic",
		},
		{
			name:       "invalid execution payload does not prune fork choice",
			payloadErr: execution.ErrInvalidPayloadStatus,
			knownBlock: true,
			wantedErr:  ErrInvalidPayload.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			s, tr := minimalTestService(t, WithExecutionEngineCaller(&mockExecution.EngineClient{ErrNewPayload: tt.payloadErr}))
			require.NoError(t, s.saveGenesisData(ctx, genesis.Copy()))
			if tt.knownBlock {
				postState, err := transition.ExecuteStateTransition(ctx, genesis.Copy(), roblock)
				require.NoError(t, err)
				require.NoError(t, tr.fcs.InsertNode(ctx, postState, roblock))
			}
			nodeCount := tr.fcs.NodeCount()
			headRoot, err := s.HeadRoot(ctx)
			require.NoError(t, err)

			err = s.ValidateBlockForEquivocation(ctx, roblock)
			if tt.wantedErr != "" {
				require.ErrorContains(t, tt.wantedErr, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.knownBlock, s.InForkchoice(roblock.Root()))
			assert.Equal(t, nodeCount, tr.fcs.NodeCount())
			assert.Equal(t, false, tr.db.HasBlock(ctx, roblock.Root()))
			assert.Equal(t, false, tr.db.HasState(ctx, roblock.Root()))
			gotHeadRoot, err := s.HeadRoot(ctx)
			require.NoError(t, err)
			assert.DeepEqual(t, headRoot, gotHeadRoot)
		})
	}
}

func TestHandleDA(t *testing.T) {
	signedBeaconBlock, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			Body: &ethpb.BeaconBlockBody{},
		},
	})
	require.NoError(t, err)

	s, _ := minimalTestService(t)
	block, err := blocks.NewROBlockWithRoot(signedBeaconBlock, [32]byte{})
	require.NoError(t, err)
	elapsed, err := s.handleDA(t.Context(), nil, block)
	require.NoError(t, err)
	require.Equal(t, true, elapsed > 0, "Elapsed time should be greater than 0")
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
	wg.Go(func() {
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		require.NoError(t, s.ReceiveBlock(ctx, wsb, root, nil))
	})
	wg.Wait()
	notifier := s.cfg.StateNotifier.(*blockchainTesting.MockStateNotifier)
	require.Eventually(t, func() bool {
		return len(notifier.ReceivedEvents()) >= 1
	}, 2*time.Second, 10*time.Millisecond, "Expected at least 1 state notification")
	// Verify fork choice has processed the block. (Genesis block and the new block)
	assert.Equal(t, 2, s.cfg.ForkChoiceStore.NodeCount())
}

func TestService_ReceiveBlockBatch(t *testing.T) {
	ctx := t.Context()

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
				notifier := s.cfg.StateNotifier.(*blockchainTesting.MockStateNotifier)
				require.Eventually(t, func() bool {
					return len(notifier.ReceivedEvents()) >= 1
				}, 2*time.Second, 10*time.Millisecond, "Expected at least 1 state notification")
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
			err = s.ReceiveBlockBatch(ctx, []blocks.ROBlock{rwsb}, nil, &das.MockAvailabilityStore{})
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
	if s.HasBlock(t.Context(), r) {
		t.Error("Should not have block")
	}
	wsb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)
	require.NoError(t, s.saveInitSyncBlock(t.Context(), r, wsb))
	if !s.HasBlock(t.Context(), r) {
		t.Error("Should have block")
	}
	b := util.NewBeaconBlock()
	b.Block.Slot = 1
	util.SaveBlock(t, t.Context(), s.cfg.BeaconDB, b)
	r, err = b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, true, s.HasBlock(t.Context(), r))
	err = s.blockBeingSynced.set(r)
	require.NoError(t, err)
	err = s.blockBeingSynced.set(r)
	require.ErrorIs(t, err, errBlockBeingSynced)
	require.Equal(t, false, s.HasBlock(t.Context(), r))
}

func TestCheckSaveHotStateDB_Enabling(t *testing.T) {
	hook := logTest.NewGlobal()
	s, _ := minimalTestService(t)
	st := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epochsSinceFinalitySaveHotStateDB))
	s.genesisTime = time.Now().Add(time.Duration(-1*int64(st)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	require.NoError(t, s.checkSaveHotStateDB(t.Context()))
	assert.LogsContain(t, hook, "Entering mode to save hot states in DB")
}

func TestCheckSaveHotStateDB_Disabling(t *testing.T) {
	hook := logTest.NewGlobal()

	s, _ := minimalTestService(t)

	st := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epochsSinceFinalitySaveHotStateDB))
	s.genesisTime = time.Now().Add(time.Duration(-1*int64(st)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)
	require.NoError(t, s.checkSaveHotStateDB(t.Context()))
	s.genesisTime = time.Now()

	require.NoError(t, s.checkSaveHotStateDB(t.Context()))
	assert.LogsContain(t, hook, "Exiting mode to save hot states in DB")
}

func TestCheckSaveHotStateDB_Overflow(t *testing.T) {
	hook := logTest.NewGlobal()
	s, _ := minimalTestService(t)
	s.SetGenesisTime(time.Now())

	require.NoError(t, s.checkSaveHotStateDB(t.Context()))
	assert.LogsDoNotContain(t, hook, "Entering mode to save hot states in DB")
}

func TestHandleCaches_EnablingLargeSize(t *testing.T) {
	hook := logTest.NewGlobal()
	s, _ := minimalTestService(t)
	st := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epochsSinceFinalitySaveHotStateDB))
	s.SetGenesisTime(time.Now().Add(time.Duration(-1*int64(st)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second))

	helpers.ClearCache()
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
	fc, ok := e.Data.(*statefeed.FinalizedCheckpointData)
	require.Equal(t, true, ok, "event has wrong data type")
	assert.Equal(t, primitives.Epoch(123), fc.Epoch)
	assert.DeepEqual(t, sbbRoot, fc.Block)
	assert.DeepEqual(t, finalizedStRoot, fc.State)
	assert.Equal(t, false, fc.ExecutionOptimistic)
}

func Test_executePostFinalizationTasks(t *testing.T) {
	logHook := logTest.NewGlobal()

	headState, err := util.NewBeaconStateElectra()
	require.NoError(t, err)
	finalizedStRoot, err := headState.HashTreeRoot(t.Context())
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
	t.Run("pre deposit request", func(t *testing.T) {
		require.NoError(t, headState.SetEth1DepositIndex(1))
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

		require.Eventually(t, func() bool {
			return len(notifier.ReceivedEvents()) == 1
		}, 5*time.Second, 50*time.Millisecond, "Expected exactly 1 state notification")
		e := notifier.ReceivedEvents()[0]
		assert.Equal(t, statefeed.FinalizedCheckpoint, int(e.Type))
		fc, ok := e.Data.(*statefeed.FinalizedCheckpointData)
		require.Equal(t, true, ok, "event has wrong data type")
		assert.Equal(t, primitives.Epoch(123), fc.Epoch)
		assert.DeepEqual(t, headRoot, fc.Block)
		assert.DeepEqual(t, finalizedStRoot, fc.State)
		assert.Equal(t, false, fc.ExecutionOptimistic)

		// check the cache
		index, ok := headState.ValidatorIndexByPubkey(bytesutil.ToBytes48(key))
		require.Equal(t, true, ok)
		require.Equal(t, primitives.ValidatorIndex(0), index) // first index

		// check deposit
		require.LogsContain(t, logHook, "Finalized deposit insertion completed at index")
	})
	t.Run("deposit requests started", func(t *testing.T) {
		require.NoError(t, headState.SetEth1DepositIndex(1))
		require.NoError(t, headState.SetDepositRequestsStartIndex(1))
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

		require.Eventually(t, func() bool {
			return len(notifier.ReceivedEvents()) == 1
		}, 5*time.Second, 50*time.Millisecond, "Expected exactly 1 state notification")
		e := notifier.ReceivedEvents()[0]
		assert.Equal(t, statefeed.FinalizedCheckpoint, int(e.Type))
		fc, ok := e.Data.(*statefeed.FinalizedCheckpointData)
		require.Equal(t, true, ok, "event has wrong data type")
		assert.Equal(t, primitives.Epoch(123), fc.Epoch)
		assert.DeepEqual(t, headRoot, fc.Block)
		assert.DeepEqual(t, finalizedStRoot, fc.State)
		assert.Equal(t, false, fc.ExecutionOptimistic)

		// check the cache
		index, ok := headState.ValidatorIndexByPubkey(bytesutil.ToBytes48(key))
		require.Equal(t, true, ok)
		require.Equal(t, primitives.ValidatorIndex(0), index) // first index
	})

}

func TestProcessLightClientBootstrap(t *testing.T) {
	featCfg := &features.Flags{}
	featCfg.EnableLightClient = true
	reset := features.InitWithReset(featCfg)
	defer reset()

	s, tr := minimalTestService(t, WithLCStore())
	ctx := tr.ctx

	for testVersion := version.Altair; testVersion <= version.Electra; testVersion++ {
		t.Run(version.String(testVersion), func(t *testing.T) {
			l := util.NewTestLightClient(t, testVersion)

			require.NoError(t, s.cfg.BeaconDB.SaveBlock(ctx, l.FinalizedBlock))
			finalizedBlockRoot, err := l.FinalizedBlock.Block().HashTreeRoot()
			require.NoError(t, err)
			require.NoError(t, s.cfg.BeaconDB.SaveState(ctx, l.FinalizedState, finalizedBlockRoot))

			cp := l.AttestedState.FinalizedCheckpoint()
			require.DeepSSZEqual(t, finalizedBlockRoot, [32]byte(cp.Root))

			require.NoError(t, s.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: cp.Epoch, Root: [32]byte(cp.Root)}))

			s.executePostFinalizationTasks(s.ctx, l.AttestedState)

			// Wait for the light client bootstrap to be saved (runs in goroutine)
			var b interfaces.LightClientBootstrap
			require.Eventually(t, func() bool {
				var err error
				b, err = s.lcStore.LightClientBootstrap(ctx, [32]byte(cp.Root))
				return err == nil && b != nil
			}, 5*time.Second, 50*time.Millisecond, "Light client bootstrap was not saved within timeout")

			btst, err := lightClient.NewLightClientBootstrapFromBeaconState(ctx, l.FinalizedState.Slot(), l.FinalizedState, l.FinalizedBlock)
			require.NoError(t, err)
			require.DeepEqual(t, btst, b)
			require.Equal(t, b.Version(), testVersion)
		})
	}
}
