package initialsync

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/async/abool"
	mock "github.com/OffchainLabs/prysm/v6/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db/filesystem"
	dbtest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	p2pt "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/container/slice"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/paulbellamy/ratecounter"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

func TestService_roundRobinSync(t *testing.T) {
	currentPeriod := blockLimiterPeriod
	blockLimiterPeriod = 1 * time.Second
	defer func() {
		blockLimiterPeriod = currentPeriod
	}()
	tests := []struct {
		name                string
		currentSlot         primitives.Slot
		availableBlockSlots []primitives.Slot
		expectedBlockSlots  []primitives.Slot
		peers               []*peerData
	}{
		{
			name:                "Single peer with no finalized blocks",
			currentSlot:         2,
			availableBlockSlots: makeSequence(1, 32),
			expectedBlockSlots:  makeSequence(1, 2),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 2),
					finalizedEpoch: 0,
					headSlot:       2,
				},
			},
		},
		{
			name:                "Multiple peers with no finalized blocks",
			currentSlot:         2,
			availableBlockSlots: makeSequence(1, 32),
			expectedBlockSlots:  makeSequence(1, 2),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 2),
					finalizedEpoch: 0,
					headSlot:       2,
				},
				{
					blocks:         makeSequence(1, 2),
					finalizedEpoch: 0,
					headSlot:       2,
				},
				{
					blocks:         makeSequence(1, 2),
					finalizedEpoch: 0,
					headSlot:       2,
				},
			},
		},
		{
			name:                "Single peer with all blocks",
			currentSlot:         131,
			availableBlockSlots: makeSequence(1, 192),
			expectedBlockSlots:  makeSequence(1, 131),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 1,
					headSlot:       131,
				},
			},
		},
		{
			name:                "Multiple peers with all blocks",
			currentSlot:         131,
			availableBlockSlots: makeSequence(1, 192),
			expectedBlockSlots:  makeSequence(1, 131),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 1,
					headSlot:       131,
				},
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 1,
					headSlot:       131,
				},
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 1,
					headSlot:       131,
				},
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 1,
					headSlot:       131,
				},
			},
		},
		{
			name:                "Multiple peers with failures",
			currentSlot:         320, // 10 epochs
			availableBlockSlots: makeSequence(1, 384),
			expectedBlockSlots:  makeSequence(1, 320),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 384),
					finalizedEpoch: 8,
					headSlot:       320,
				},
				{
					blocks:         makeSequence(1, 384),
					finalizedEpoch: 8,
					headSlot:       320,
					failureSlots:   makeSequence(1, 32), // first epoch
				},
				{
					blocks:         makeSequence(1, 384),
					finalizedEpoch: 8,
					headSlot:       320,
				},
				{
					blocks:         makeSequence(1, 384),
					finalizedEpoch: 8,
					headSlot:       320,
				},
			},
		},
		{
			name:                "Multiple peers with many skipped slots",
			currentSlot:         1280,
			availableBlockSlots: append(makeSequence(1, 64), makeSequence(1000, 1300)...),
			expectedBlockSlots:  append(makeSequence(1, 64), makeSequence(1000, 1280)...),
			peers: []*peerData{
				{
					blocks:         append(makeSequence(1, 64), makeSequence(1000, 1300)...),
					finalizedEpoch: 36,
					headSlot:       1280,
				},
				{
					blocks:         append(makeSequence(1, 64), makeSequence(1000, 1300)...),
					finalizedEpoch: 36,
					headSlot:       1280,
				},
				{
					blocks:         append(makeSequence(1, 64), makeSequence(1000, 1300)...),
					finalizedEpoch: 36,
					headSlot:       1280,
				},
			},
		},
		{
			name:                "Multiple peers with multiple failures",
			currentSlot:         320, // 10 epochs
			availableBlockSlots: makeSequence(1, 384),
			expectedBlockSlots:  makeSequence(1, 320),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 384),
					finalizedEpoch: 9,
					headSlot:       384,
				},
				{
					blocks:         makeSequence(1, 320),
					finalizedEpoch: 9,
					headSlot:       384,
					failureSlots:   makeSequence(1, 320),
				},
				{
					blocks:         makeSequence(1, 320),
					finalizedEpoch: 9,
					headSlot:       384,
					failureSlots:   makeSequence(1, 320),
				},
				{
					blocks:         makeSequence(1, 320),
					finalizedEpoch: 9,
					headSlot:       384,
					failureSlots:   makeSequence(1, 320),
				},
			},
		},
		{
			name:                "Multiple peers with different finalized epoch",
			currentSlot:         320, // 10 epochs
			availableBlockSlots: makeSequence(1, 384),
			expectedBlockSlots:  makeSequence(1, 320),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 384),
					finalizedEpoch: 10,
					headSlot:       384,
				},
				{
					blocks:         makeSequence(1, 384),
					finalizedEpoch: 10,
					headSlot:       384,
				},
				{
					blocks:         makeSequence(1, 256),
					finalizedEpoch: 5,
					headSlot:       256,
				},
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 2,
					headSlot:       192,
				},
			},
		},
		{
			name:                "Multiple peers with missing parent blocks",
			currentSlot:         160, // 5 epochs
			availableBlockSlots: makeSequence(1, 192),
			expectedBlockSlots:  makeSequence(1, 160),
			peers: []*peerData{
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 4,
					headSlot:       160,
				},
				{
					blocks:         append(makeSequence(1, 6), makeSequence(161, 165)...),
					finalizedEpoch: 4,
					headSlot:       160,
					forkedPeer:     true,
				},
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 4,
					headSlot:       160,
				},
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 4,
					headSlot:       160,
				},
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 4,
					headSlot:       160,
				},
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 4,
					headSlot:       160,
				},
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 4,
					headSlot:       160,
				},
				{
					blocks:         makeSequence(1, 192),
					finalizedEpoch: 4,
					headSlot:       160,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.availableBlockSlots == nil {
				tt.availableBlockSlots = tt.expectedBlockSlots
			}
			cache.initializeRootCache(tt.availableBlockSlots, t)

			p := p2pt.NewTestP2P(t)
			beaconDB := dbtest.SetupDB(t)

			connectPeers(t, p, tt.peers, p.Peers())
			cache.RLock()
			genesisRoot := cache.rootCache[0]
			cache.RUnlock()

			util.SaveBlock(t, t.Context(), beaconDB, util.NewBeaconBlock())

			st, err := util.NewBeaconState()
			require.NoError(t, err)
			gt := time.Now()
			vr := [32]byte{}
			mc := &mock.ChainService{
				State: st,
				Root:  genesisRoot[:],
				DB:    beaconDB,
				FinalizedCheckPoint: &eth.Checkpoint{
					Epoch: 0,
				},
				Genesis:        gt,
				ValidatorsRoot: vr,
			} // no-op mock
			clock := startup.NewClock(gt, vr)
			s := &Service{
				ctx:          t.Context(),
				cfg:          &Config{Chain: mc, P2P: p, DB: beaconDB},
				synced:       abool.New(),
				chainStarted: abool.NewBool(true),
				clock:        clock,
			}
			s.genesisTime = makeGenesisTime(tt.currentSlot)
			assert.NoError(t, s.roundRobinSync())
			if s.cfg.Chain.HeadSlot() < tt.currentSlot {
				t.Errorf("Head slot (%d) is less than expected currentSlot (%d)", s.cfg.Chain.HeadSlot(), tt.currentSlot)
			}
			assert.Equal(t, true, len(tt.expectedBlockSlots) <= len(mc.BlocksReceived), "Processes wrong number of blocks")
			var receivedBlockSlots []primitives.Slot
			for _, blk := range mc.BlocksReceived {
				receivedBlockSlots = append(receivedBlockSlots, blk.Block().Slot())
			}
			missing := slice.NotSlot(slice.IntersectionSlot(tt.expectedBlockSlots, receivedBlockSlots), tt.expectedBlockSlots)
			if len(missing) > 0 {
				t.Errorf("Missing blocks at slots %v", missing)
			}
		})
	}
}

func TestService_processBlock(t *testing.T) {
	beaconDB := dbtest.SetupDB(t)
	genesisBlk := util.NewBeaconBlock()
	genesisBlkRoot, err := genesisBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, t.Context(), beaconDB, genesisBlk)
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	s := NewService(t.Context(), &Config{
		P2P: p2pt.NewTestP2P(t),
		DB:  beaconDB,
		Chain: &mock.ChainService{
			State: st,
			Root:  genesisBlkRoot[:],
			DB:    beaconDB,
			FinalizedCheckPoint: &eth.Checkpoint{
				Epoch: 0,
			},
			Genesis:        time.Now(),
			ValidatorsRoot: [32]byte{},
		},
		StateNotifier: &mock.MockStateNotifier{},
	})
	ctx := t.Context()
	genesis := makeGenesisTime(32)

	t.Run("process duplicate block", func(t *testing.T) {
		blk1 := util.NewBeaconBlock()
		blk1.Block.Slot = 1
		blk1.Block.ParentRoot = genesisBlkRoot[:]
		blk1Root, err := blk1.Block.HashTreeRoot()
		require.NoError(t, err)
		blk2 := util.NewBeaconBlock()
		blk2.Block.Slot = 2
		blk2.Block.ParentRoot = blk1Root[:]

		// Process block normally.
		wsb, err := blocks.NewSignedBeaconBlock(blk1)
		require.NoError(t, err)
		rowsb, err := blocks.NewROBlock(wsb)
		require.NoError(t, err)
		err = s.processBlock(ctx, genesis, blocks.BlockWithROSidecars{Block: rowsb}, func(
			ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte, _ das.AvailabilityStore) error {
			assert.NoError(t, s.cfg.Chain.ReceiveBlock(ctx, block, blockRoot, nil))
			return nil
		}, nil)
		assert.NoError(t, err)

		// Duplicate processing should trigger error.
		wsb, err = blocks.NewSignedBeaconBlock(blk1)
		require.NoError(t, err)
		rowsb, err = blocks.NewROBlock(wsb)
		require.NoError(t, err)
		err = s.processBlock(ctx, genesis, blocks.BlockWithROSidecars{Block: rowsb}, func(
			ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte, _ das.AvailabilityStore) error {
			return nil
		}, nil)
		assert.ErrorContains(t, errBlockAlreadyProcessed.Error(), err)

		// Continue normal processing, should proceed w/o errors.
		wsb, err = blocks.NewSignedBeaconBlock(blk2)
		require.NoError(t, err)
		rowsb, err = blocks.NewROBlock(wsb)
		require.NoError(t, err)
		err = s.processBlock(ctx, genesis, blocks.BlockWithROSidecars{Block: rowsb}, func(
			ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte, _ das.AvailabilityStore) error {
			assert.NoError(t, s.cfg.Chain.ReceiveBlock(ctx, block, blockRoot, nil))
			return nil
		}, nil)
		assert.NoError(t, err)
		assert.Equal(t, primitives.Slot(2), s.cfg.Chain.HeadSlot(), "Unexpected head slot")
	})
}

func TestService_processBlockBatch(t *testing.T) {
	beaconDB := dbtest.SetupDB(t)
	genesisBlk := util.NewBeaconBlock()
	genesisBlkRoot, err := genesisBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, t.Context(), beaconDB, genesisBlk)
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	s := NewService(t.Context(), &Config{
		P2P: p2pt.NewTestP2P(t),
		DB:  beaconDB,
		Chain: &mock.ChainService{
			State: st,
			Root:  genesisBlkRoot[:],
			DB:    beaconDB,
			FinalizedCheckPoint: &eth.Checkpoint{
				Epoch: 0,
			},
		},
		StateNotifier: &mock.MockStateNotifier{},
	})
	ctx := t.Context()
	genesis := makeGenesisTime(32)
	s.genesisTime = genesis

	t.Run("process non-linear batch", func(t *testing.T) {
		var batch []blocks.BlockWithROSidecars
		currBlockRoot := genesisBlkRoot
		for i := primitives.Slot(1); i < 10; i++ {
			parentRoot := currBlockRoot
			blk1 := util.NewBeaconBlock()
			blk1.Block.Slot = i
			blk1.Block.ParentRoot = parentRoot[:]
			blk1Root, err := blk1.Block.HashTreeRoot()
			require.NoError(t, err)
			util.SaveBlock(t, t.Context(), beaconDB, blk1)
			wsb, err := blocks.NewSignedBeaconBlock(blk1)
			require.NoError(t, err)
			rowsb, err := blocks.NewROBlock(wsb)
			require.NoError(t, err)
			batch = append(batch, blocks.BlockWithROSidecars{Block: rowsb})
			currBlockRoot = blk1Root
		}

		var batch2 []blocks.BlockWithROSidecars
		for i := primitives.Slot(10); i < 20; i++ {
			parentRoot := currBlockRoot
			blk1 := util.NewBeaconBlock()
			blk1.Block.Slot = i
			blk1.Block.ParentRoot = parentRoot[:]
			blk1Root, err := blk1.Block.HashTreeRoot()
			require.NoError(t, err)
			util.SaveBlock(t, t.Context(), beaconDB, blk1)
			wsb, err := blocks.NewSignedBeaconBlock(blk1)
			require.NoError(t, err)
			rowsb, err := blocks.NewROBlock(wsb)
			require.NoError(t, err)
			batch2 = append(batch2, blocks.BlockWithROSidecars{Block: rowsb})
			currBlockRoot = blk1Root
		}

		cbnormal := func(ctx context.Context, blks []blocks.ROBlock, avs das.AvailabilityStore) error {
			assert.NoError(t, s.cfg.Chain.ReceiveBlockBatch(ctx, blks, avs))
			return nil
		}
		// Process block normally.
		count, err := s.processBatchedBlocks(ctx, batch, cbnormal)
		assert.NoError(t, err)
		require.Equal(t, uint64(len(batch)), count)

		cbnil := func(ctx context.Context, blocks []blocks.ROBlock, _ das.AvailabilityStore) error {
			return nil
		}

		// Duplicate processing should trigger error.
		count, err = s.processBatchedBlocks(ctx, batch, cbnil)
		assert.ErrorContains(t, "block is already processed", err)
		require.Equal(t, uint64(0), count)

		var badBatch2 []blocks.BlockWithROSidecars
		for i, b := range batch2 {
			// create a non-linear batch
			if i%3 == 0 && i != 0 {
				continue
			}
			badBatch2 = append(badBatch2, b)
		}

		// Bad batch should fail because it is non linear
		count, err = s.processBatchedBlocks(ctx, badBatch2, cbnil)
		expectedSubErr := "expected linear block list"
		assert.ErrorContains(t, expectedSubErr, err)
		require.Equal(t, uint64(0), count)

		// Continue normal processing, should proceed w/o errors.
		count, err = s.processBatchedBlocks(ctx, batch2, cbnormal)
		assert.NoError(t, err)
		assert.Equal(t, primitives.Slot(19), s.cfg.Chain.HeadSlot(), "Unexpected head slot")
		require.Equal(t, uint64(len(batch2)), count)
	})
}

func TestService_blockProviderScoring(t *testing.T) {
	currentPeriod := blockLimiterPeriod
	blockLimiterPeriod = 1 * time.Second
	defer func() {
		blockLimiterPeriod = currentPeriod
	}()
	cache.initializeRootCache(makeSequence(1, 640), t)

	p := p2pt.NewTestP2P(t)
	beaconDB := dbtest.SetupDB(t)

	peerData := []*peerData{
		{
			// The slowest peer, only a single block in couple of epochs.
			blocks:         []primitives.Slot{1, 65, 129},
			finalizedEpoch: 5,
			headSlot:       160,
		},
		{
			// A relatively slow peer, still should perform better than the slowest peer.
			blocks:         append([]primitives.Slot{1, 2, 3, 4, 65, 66, 67, 68, 129, 130}, makeSequence(131, 160)...),
			finalizedEpoch: 5,
			headSlot:       160,
		},
		{
			// This peer has all blocks - should be a preferred one.
			blocks:         makeSequence(1, 320),
			finalizedEpoch: 5,
			headSlot:       160,
		},
	}

	peer1 := connectPeer(t, p, peerData[0], p.Peers())
	peer2 := connectPeer(t, p, peerData[1], p.Peers())
	peer3 := connectPeer(t, p, peerData[2], p.Peers())

	cache.RLock()
	genesisRoot := cache.rootCache[0]
	cache.RUnlock()

	util.SaveBlock(t, t.Context(), beaconDB, util.NewBeaconBlock())

	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, err)
	gt := time.Now()
	vr := [32]byte{}
	mc := &mock.ChainService{
		State: st,
		Root:  genesisRoot[:],
		DB:    beaconDB,
		FinalizedCheckPoint: &eth.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		Genesis:        gt,
		ValidatorsRoot: vr,
	} // no-op mock
	clock := startup.NewClock(gt, vr)
	s := &Service{
		ctx:          t.Context(),
		cfg:          &Config{Chain: mc, P2P: p, DB: beaconDB},
		synced:       abool.New(),
		chainStarted: abool.NewBool(true),
		clock:        clock,
	}
	scorer := s.cfg.P2P.Peers().Scorers().BlockProviderScorer()
	expectedBlockSlots := makeSequence(1, 160)
	currentSlot := primitives.Slot(160)

	assert.Equal(t, scorer.MaxScore(), scorer.Score(peer1))
	assert.Equal(t, scorer.MaxScore(), scorer.Score(peer2))
	assert.Equal(t, scorer.MaxScore(), scorer.Score(peer3))

	s.genesisTime = makeGenesisTime(currentSlot)
	assert.NoError(t, s.roundRobinSync())
	if s.cfg.Chain.HeadSlot() < currentSlot {
		t.Errorf("Head slot (%d) is less than expected currentSlot (%d)", s.cfg.Chain.HeadSlot(), currentSlot)
	}
	assert.Equal(t, true, len(expectedBlockSlots) <= len(mc.BlocksReceived), "Processes wrong number of blocks")
	var receivedBlockSlots []primitives.Slot
	for _, blk := range mc.BlocksReceived {
		receivedBlockSlots = append(receivedBlockSlots, blk.Block().Slot())
	}
	missing := slice.NotSlot(slice.IntersectionSlot(expectedBlockSlots, receivedBlockSlots), expectedBlockSlots)
	if len(missing) > 0 {
		t.Errorf("Missing blocks at slots %v", missing)
	}

	// Increment all peers' stats, so that nobody is boosted (as new, not yet used peer).
	scorer.IncrementProcessedBlocks(peer1, 1)
	scorer.IncrementProcessedBlocks(peer2, 1)
	scorer.IncrementProcessedBlocks(peer3, 1)
	score1 := scorer.Score(peer1)
	score2 := scorer.Score(peer2)
	score3 := scorer.Score(peer3)
	assert.Equal(t, true, score1 < score3, "Incorrect score (%v) for peer: %v (must be lower than %v)", score1, peer1, score3)
	assert.Equal(t, true, score2 < score3, "Incorrect score (%v) for peer: %v (must be lower than %v)", score2, peer2, score3)
	assert.Equal(t, true, scorer.ProcessedBlocks(peer3) > 100, "Not enough blocks returned by healthy peer: %d", scorer.ProcessedBlocks(peer3))
}

func TestService_syncToFinalizedEpoch(t *testing.T) {
	cache.initializeRootCache(makeSequence(1, 640), t)

	p := p2pt.NewTestP2P(t)
	beaconDB := dbtest.SetupDB(t)
	cache.RLock()
	genesisRoot := cache.rootCache[0]
	cache.RUnlock()

	util.SaveBlock(t, t.Context(), beaconDB, util.NewBeaconBlock())

	st, err := util.NewBeaconState()
	require.NoError(t, err)
	gt := time.Now()
	vr := [32]byte{}
	clock := startup.NewClock(gt, vr)
	mc := &mock.ChainService{
		State: st,
		Root:  genesisRoot[:],
		DB:    beaconDB,
		FinalizedCheckPoint: &eth.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		Genesis:        gt,
		ValidatorsRoot: vr,
	}
	s := &Service{
		ctx:          t.Context(),
		cfg:          &Config{Chain: mc, P2P: p, DB: beaconDB},
		synced:       abool.New(),
		chainStarted: abool.NewBool(true),
		counter:      ratecounter.NewRateCounter(counterSeconds * time.Second),
		clock:        clock,
	}
	expectedBlockSlots := makeSequence(1, 191)
	currentSlot := primitives.Slot(191)

	// Sync to finalized epoch.
	hook := logTest.NewGlobal()
	connectPeer(t, p, &peerData{
		blocks:         makeSequence(1, 240),
		finalizedEpoch: 5,
		headSlot:       195,
	}, p.Peers())
	genesis := makeGenesisTime(currentSlot)
	s.genesisTime = genesis
	assert.NoError(t, s.syncToFinalizedEpoch(t.Context()))
	if s.cfg.Chain.HeadSlot() < currentSlot {
		t.Errorf("Head slot (%d) is less than expected currentSlot (%d)", s.cfg.Chain.HeadSlot(), currentSlot)
	}
	assert.Equal(t, true, len(expectedBlockSlots) <= len(mc.BlocksReceived), "Processes wrong number of blocks")
	var receivedBlockSlots []primitives.Slot
	for _, blk := range mc.BlocksReceived {
		receivedBlockSlots = append(receivedBlockSlots, blk.Block().Slot())
	}
	missing := slice.NotSlot(slice.IntersectionSlot(expectedBlockSlots, receivedBlockSlots), expectedBlockSlots)
	if len(missing) > 0 {
		t.Errorf("Missing blocks at slots %v", missing)
	}
	assert.LogsDoNotContain(t, hook, "Already synced to finalized epoch")

	// Try to re-sync, should be exited immediately (node is already synced to finalized epoch).
	hook.Reset()
	s.genesisTime = genesis
	assert.NoError(t, s.syncToFinalizedEpoch(t.Context()))
	assert.LogsContain(t, hook, "Already synced to finalized epoch")
}

func TestService_ValidUnprocessed(t *testing.T) {
	beaconDB := dbtest.SetupDB(t)
	genesisBlk := util.NewBeaconBlock()
	genesisBlkRoot, err := genesisBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, t.Context(), beaconDB, genesisBlk)

	var batch []blocks.BlockWithROSidecars
	currBlockRoot := genesisBlkRoot
	for i := primitives.Slot(1); i < 10; i++ {
		parentRoot := currBlockRoot
		blk1 := util.NewBeaconBlock()
		blk1.Block.Slot = i
		blk1.Block.ParentRoot = parentRoot[:]
		blk1Root, err := blk1.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, t.Context(), beaconDB, blk1)
		wsb, err := blocks.NewSignedBeaconBlock(blk1)
		require.NoError(t, err)
		rowsb, err := blocks.NewROBlock(wsb)
		require.NoError(t, err)
		batch = append(batch, blocks.BlockWithROSidecars{Block: rowsb})
		currBlockRoot = blk1Root
	}

	retBlocks, err := validUnprocessed(t.Context(), batch, 2, func(ctx context.Context, block blocks.ROBlock) bool {
		// Ignore first 2 blocks in the batch.
		return block.Block().Slot() <= 2
	})
	require.NoError(t, err)

	// Ensure that the unprocessed batch is returned correctly.
	assert.Equal(t, len(retBlocks), len(batch)-2)
}

func TestService_PropcessFetchedDataRegSync(t *testing.T) {
	ctx := t.Context()

	// Create a data columns storage.
	dir := t.TempDir()
	dataColumnStorage, err := filesystem.NewDataColumnStorage(ctx, filesystem.WithDataColumnBasePath(dir))
	require.NoError(t, err)

	// Create Fulu blocks.
	fuluBlock1 := util.NewBeaconBlockFulu()
	signedFuluBlock1, err := blocks.NewSignedBeaconBlock(fuluBlock1)
	require.NoError(t, err)
	roFuluBlock1, err := blocks.NewROBlock(signedFuluBlock1)
	require.NoError(t, err)
	block1Root := roFuluBlock1.Root()

	fuluBlock2 := util.NewBeaconBlockFulu()
	fuluBlock2.Block.Body.BlobKzgCommitments = [][]byte{make([]byte, fieldparams.KzgCommitmentSize)} // Dummy commitment.
	fuluBlock2.Block.Slot = 1
	fuluBlock2.Block.ParentRoot = block1Root[:]
	signedFuluBlock2, err := blocks.NewSignedBeaconBlock(fuluBlock2)
	require.NoError(t, err)

	roFuluBlock2, err := blocks.NewROBlock(signedFuluBlock2)
	require.NoError(t, err)
	block2Root := roFuluBlock2.Root()
	parentRoot2 := roFuluBlock2.Block().ParentRoot()
	bodyRoot2, err := roFuluBlock2.Block().Body().HashTreeRoot()
	require.NoError(t, err)

	// Create a mock chain service.
	const validatorCount = uint64(64)
	state, _ := util.DeterministicGenesisState(t, validatorCount)
	chain := &mock.ChainService{
		FinalizedCheckPoint: &eth.Checkpoint{},
		DB:                  dbtest.SetupDB(t),
		State:               state,
		Root:                block1Root[:],
	}

	// Create a new service instance.
	service := &Service{
		cfg: &Config{
			Chain:             chain,
			DataColumnStorage: dataColumnStorage,
		},
		counter: ratecounter.NewRateCounter(counterSeconds * time.Second),
	}

	// Save the parent block in the database.
	err = chain.DB.SaveBlock(ctx, roFuluBlock1)
	require.NoError(t, err)

	// Create data column sidecars.
	const count = uint64(3)
	params := make([]util.DataColumnParam, 0, count)
	for i := range count {
		param := util.DataColumnParam{Index: i, BodyRoot: bodyRoot2[:], ParentRoot: parentRoot2[:], Slot: roFuluBlock2.Block().Slot()}
		params = append(params, param)
	}
	_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

	blocksWithSidecars := []blocks.BlockWithROSidecars{
		{Block: roFuluBlock2, Columns: verifiedRoDataColumnSidecars},
	}

	data := &blocksQueueFetchedData{
		bwb: blocksWithSidecars,
	}

	actual, err := service.processFetchedDataRegSync(ctx, data)
	require.NoError(t, err)
	require.Equal(t, uint64(1), actual)

	// Check block and data column sidecars were saved correctly.
	require.Equal(t, true, chain.DB.HasBlock(ctx, block2Root))

	summary := dataColumnStorage.Summary(block2Root)
	for i := range count {
		require.Equal(t, true, summary.HasIndex(i))
	}
}

func TestService_processBlocksWithDataColumns(t *testing.T) {
	ctx := t.Context()

	t.Run("no blocks", func(t *testing.T) {
		fuluBlock := util.NewBeaconBlockFulu()

		signedFuluBlock, err := blocks.NewSignedBeaconBlock(fuluBlock)
		require.NoError(t, err)
		roFuluBlock, err := blocks.NewROBlock(signedFuluBlock)
		require.NoError(t, err)

		service := new(Service)
		err = service.processBlocksWithDataColumns(ctx, nil, nil, roFuluBlock)
		require.NoError(t, err)
	})

	t.Run("nominal", func(t *testing.T) {
		fuluBlock := util.NewBeaconBlockFulu()
		fuluBlock.Block.Body.BlobKzgCommitments = [][]byte{make([]byte, fieldparams.KzgCommitmentSize)} // Dummy commitment.
		signedFuluBlock, err := blocks.NewSignedBeaconBlock(fuluBlock)
		require.NoError(t, err)
		roFuluBlock, err := blocks.NewROBlock(signedFuluBlock)
		require.NoError(t, err)
		bodyRoot, err := roFuluBlock.Block().Body().HashTreeRoot()
		require.NoError(t, err)

		// Create data column sidecars.
		const count = uint64(3)
		params := make([]util.DataColumnParam, 0, count)
		for i := range count {
			param := util.DataColumnParam{Index: i, BodyRoot: bodyRoot[:]}
			params = append(params, param)
		}
		_, verifiedRoDataColumnSidecars := util.CreateTestVerifiedRoDataColumnSidecars(t, params)

		blocksWithSidecars := []blocks.BlockWithROSidecars{
			{Block: roFuluBlock, Columns: verifiedRoDataColumnSidecars},
		}

		// Create a data columns storage.
		dir := t.TempDir()
		dataColumnStorage, err := filesystem.NewDataColumnStorage(ctx, filesystem.WithDataColumnBasePath(dir))
		require.NoError(t, err)

		// Create a service.
		service := &Service{
			cfg: &Config{
				P2P:               p2pt.NewTestP2P(t),
				DataColumnStorage: dataColumnStorage,
			},
			counter: ratecounter.NewRateCounter(counterSeconds * time.Second),
		}

		receiverFunc := func(ctx context.Context, blks []blocks.ROBlock, avs das.AvailabilityStore) error {
			require.Equal(t, 1, len(blks))
			return nil
		}

		err = service.processBlocksWithDataColumns(ctx, blocksWithSidecars, receiverFunc, roFuluBlock)
		require.NoError(t, err)

		// Verify that the data columns were saved correctly.
		summary := dataColumnStorage.Summary(roFuluBlock.Root())
		for i := range count {
			require.Equal(t, true, summary.HasIndex(i))
		}
	})
}
