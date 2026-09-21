package pruner

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"

	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	slottest "github.com/OffchainLabs/prysm/v7/time/slots/testing"
	"github.com/sirupsen/logrus"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	dbtest "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

// testClockWaiter returns a clock waiter whose clock is already set, so that a started pruner
// never blocks waiting for it.
func testClockWaiter(t *testing.T) startup.ClockWaiter {
	clockSynchronizer := startup.NewClockSynchronizer()
	require.NoError(t, clockSynchronizer.SetClock(startup.NewClock(time.Now(), [32]byte{})))

	return clockSynchronizer
}

func TestPruner_PruningConditions(t *testing.T) {
	tests := []struct {
		name              string
		synced            bool
		backfillCompleted bool
		expectedLog       string
	}{
		{
			name:              "Not synced",
			synced:            false,
			backfillCompleted: true,
			expectedLog:       "Waiting for initial sync service to complete before starting pruner",
		},
		{
			name:              "Backfill incomplete",
			synced:            true,
			backfillCompleted: false,
			expectedLog:       "Waiting for backfill service to complete before starting pruner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logrus.SetLevel(logrus.DebugLevel)
			hook := logTest.NewGlobal()
			ctx, cancel := context.WithCancel(t.Context())
			beaconDB := dbtest.SetupDB(t)

			slotTicker := &slottest.MockTicker{Channel: make(chan primitives.Slot)}

			waitChan := make(chan struct{})
			waiter := func() error {
				close(waitChan)
				return nil
			}

			var initSyncWaiter, backfillWaiter func() error
			if !tt.synced {
				initSyncWaiter = waiter
			}
			if !tt.backfillCompleted {
				backfillWaiter = waiter
			}

			mockCustody := &mockCustodyUpdater{}
			p, err := New(ctx, beaconDB, testClockWaiter(t), initSyncWaiter, backfillWaiter, mockCustody, WithSlotTicker(slotTicker))
			require.NoError(t, err)

			go p.Start()
			<-waitChan
			cancel()

			if tt.expectedLog != "" {
				require.LogsContain(t, hook, tt.expectedLog)
			}

			require.NoError(t, p.Stop())
		})
	}
}

func TestPruner_PruneSuccess(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.MinEpochsForBlockRequests = 1
	params.OverrideBeaconConfig(config)

	ctx := t.Context()
	beaconDB := dbtest.SetupDB(t)

	// Create and save some blocks at different slots
	var blks []*eth.SignedBeaconBlock
	var parentRoot [32]byte
	for slot := primitives.Slot(1); slot <= 32; slot++ {
		blk := util.NewBeaconBlock()
		blk.Block.Slot = slot
		blk.Block.ParentRoot = append([]byte(nil), parentRoot[:]...)
		wsb, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
		blks = append(blks, blk)
		parentRoot, err = wsb.Block().HashTreeRoot()
		require.NoError(t, err)
		if slot == 1 {
			require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, parentRoot))
		}
	}
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &eth.Checkpoint{Epoch: 1, Root: parentRoot[:]}))

	// Create pruner with retention of 2 epochs (64 slots)
	retentionEpochs := primitives.Epoch(2)
	slotTicker := &slottest.MockTicker{Channel: make(chan primitives.Slot)}

	mockCustody := &mockCustodyUpdater{}
	p, err := New(
		ctx,
		beaconDB,
		testClockWaiter(t),
		nil,
		nil,
		mockCustody,
		WithSlotTicker(slotTicker),
	)
	require.NoError(t, err)

	p.ps = func(current primitives.Slot) primitives.Slot {
		return current - primitives.Slot(retentionEpochs)*params.BeaconConfig().SlotsPerEpoch
	}

	// Start pruner and trigger at middle of 3rd epoch (slot 80)
	go p.Start()
	currentSlot := primitives.Slot(80) // Middle of 3rd epoch
	slotTicker.Channel <- currentSlot
	// Send the same slot again to ensure the pruning operation completes
	slotTicker.Channel <- currentSlot

	for slot := primitives.Slot(1); slot <= 32; slot++ {
		root, err := blks[slot-1].Block.HashTreeRoot()
		require.NoError(t, err)
		present := beaconDB.HasBlock(ctx, root)
		// pruneUpto is 80 - 2*32 = 16. The block at pruneUpto is kept, since the states at or
		// after it are replayed from the state stored there, so only the blocks strictly before
		// it are pruned.
		if slot < 16 { // These should be pruned
			require.NoError(t, err)
			require.Equal(t, false, present, "Expected present at slot %d to be pruned", slot)
		} else { // These should remain
			require.NoError(t, err)
			require.Equal(t, true, present, "Expected present at slot %d to exist", slot)
		}
	}

	require.NoError(t, p.Stop())
}

func TestPruner_PruneBoundedByFinality(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.MinEpochsForBlockRequests = 1
	config.FuluForkEpoch = 0
	params.OverrideBeaconConfig(config)

	for _, full := range []bool{false, true} {
		name := "blinded blocks"
		if full {
			name = "full blocks"
		}
		t.Run(name, func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{SaveFullExecutionPayloads: full})
			defer reset()

			for _, tt := range []struct {
				name            string
				finalizedSlot   primitives.Slot
				finalizedEpoch  primitives.Epoch
				retentionCutoff primitives.Slot
				wantCutoff      primitives.Slot
				finalized       bool
				advanceFinality bool
			}{
				{name: "no finalized checkpoint", retentionCutoff: 96},
				{name: "genesis finalized", finalized: true, retentionCutoff: 96},
				{name: "stalled finality", finalized: true, finalizedSlot: 63, finalizedEpoch: 2, retentionCutoff: 96, wantCutoff: 63, advanceFinality: true},
				{name: "skipped checkpoint slot", finalized: true, finalizedSlot: 63, finalizedEpoch: 2, retentionCutoff: 64, wantCutoff: 63},
				{name: "retention limits pruning", finalized: true, finalizedSlot: 128, finalizedEpoch: 4, retentionCutoff: 64, wantCutoff: 64},
				{name: "finality at retention boundary", finalized: true, finalizedSlot: 128, finalizedEpoch: 4, retentionCutoff: 128, wantCutoff: 128},
			} {
				t.Run(tt.name, func(t *testing.T) {
					ctx := t.Context()
					beaconDB := dbtest.SetupDB(t)
					roots := make(map[primitives.Slot][32]byte)
					var parentRoot [32]byte
					for _, slot := range []primitives.Slot{0, 31, 63, 65, 95, 128, 160} {
						blk := util.NewBeaconBlockBellatrix()
						blk.Block.Slot = slot
						blk.Block.ParentRoot = parentRoot[:]
						wsb, err := blocks.NewSignedBeaconBlock(blk)
						require.NoError(t, err)
						require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
						root, err := wsb.Block().HashTreeRoot()
						require.NoError(t, err)
						roots[slot] = root
						parentRoot = root
						if slot == 0 {
							require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, root))
						}
						st, err := util.NewBeaconStateBellatrix()
						require.NoError(t, err)
						require.NoError(t, st.SetSlot(slot))
						require.NoError(t, beaconDB.SaveState(ctx, st, root))
						require.NoError(t, beaconDB.SaveStateSummary(ctx, &eth.StateSummary{Slot: slot, Root: root[:]}))
						stored, err := beaconDB.Block(ctx, root)
						require.NoError(t, err)
						require.Equal(t, !full, stored.IsBlinded())
					}
					if tt.finalized {
						root := roots[tt.finalizedSlot]
						require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &eth.Checkpoint{Epoch: tt.finalizedEpoch, Root: root[:]}))
					}

					custody := &mockCustodyUpdater{}
					p, err := New(ctx, beaconDB, testClockWaiter(t), nil, nil, custody)
					require.NoError(t, err)
					currentSlot := tt.retentionCutoff + 2*config.SlotsPerEpoch
					require.NoError(t, p.prune(currentSlot))

					checkRetained := func(cutoff primitives.Slot) {
						t.Helper()
						for slot, root := range roots {
							want := slot >= cutoff
							require.Equal(t, want, beaconDB.HasBlock(ctx, root), "block at slot %d", slot)
							require.Equal(t, want, beaconDB.HasState(ctx, root), "state at slot %d", slot)
							require.Equal(t, want, beaconDB.HasStateSummary(ctx, root), "summary at slot %d", slot)
						}
						require.Equal(t, cutoff, p.prunedUpto)
						require.Equal(t, cutoff, custody.earliestAvailableSlot)
					}
					checkRetained(tt.wantCutoff)
					if !tt.advanceFinality {
						return
					}

					// Advancing the clock alone must not delete more of the unfinalized chain.
					require.NoError(t, p.prune(currentSlot+config.SlotsPerEpoch))
					checkRetained(tt.wantCutoff)
					require.Equal(t, 1, custody.updateCallCount)

					root := roots[128]
					require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &eth.Checkpoint{Epoch: 4, Root: root[:]}))
					require.NoError(t, p.prune(currentSlot+2*config.SlotsPerEpoch))
					checkRetained(128)
					require.Equal(t, 2, custody.updateCallCount)
				})
			}
		})
	}
}

func TestPruner_PruneFinalityUnavailable(t *testing.T) {
	for _, tt := range []struct {
		name       string
		checkpoint *eth.Checkpoint
		err        error
		wantErr    string
	}{
		{name: "nil checkpoint"},
		{name: "checkpoint lookup failed", err: errors.New("checkpoint read failed"), wantErr: "get finalized checkpoint for pruning"},
		{name: "missing finalized block", checkpoint: &eth.Checkpoint{Epoch: 1, Root: make([]byte, 32)}, wantErr: "get finalized block for pruning"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			beaconDB := dbtest.SetupDB(t)
			blk := util.NewBeaconBlock()
			blk.Block.Slot = 1
			wsb, err := blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)
			require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
			root, err := wsb.Block().HashTreeRoot()
			require.NoError(t, err)
			custody := &mockCustodyUpdater{}
			p, err := New(ctx, &finalizedCheckpointDB{Database: beaconDB, checkpoint: tt.checkpoint, err: tt.err}, testClockWaiter(t), nil, nil, custody)
			require.NoError(t, err)
			p.ps = func(primitives.Slot) primitives.Slot { return 96 }

			err = p.prune(160)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, tt.wantErr, err)
			}
			require.Equal(t, true, beaconDB.HasBlock(ctx, root))
			require.Equal(t, primitives.Slot(0), p.prunedUpto)
			require.Equal(t, 0, custody.updateCallCount)
		})
	}
}

type finalizedCheckpointDB struct {
	db.Database
	checkpoint *eth.Checkpoint
	err        error
}

func (d *finalizedCheckpointDB) FinalizedCheckpoint(context.Context) (*eth.Checkpoint, error) {
	return d.checkpoint, d.err
}

// Mock custody updater for testing
type mockCustodyUpdater struct {
	custodyGroupCount     uint64
	earliestAvailableSlot primitives.Slot
	updateCallCount       int
}

func (m *mockCustodyUpdater) UpdateEarliestAvailableSlot(earliestAvailableSlot primitives.Slot) error {
	m.updateCallCount++
	m.earliestAvailableSlot = earliestAvailableSlot
	return nil
}

func TestPruner_UpdatesEarliestAvailableSlot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.FuluForkEpoch = 0 // Enable Fulu from epoch 0
	// The pruner is triggered at epoch 2 below, and the database refuses to advertise an earliest
	// available slot inside the MIN_EPOCHS_FOR_BLOCK_REQUESTS window, so it has to be small here.
	config.MinEpochsForBlockRequests = 1
	params.OverrideBeaconConfig(config)

	logrus.SetLevel(logrus.DebugLevel)
	hook := logTest.NewGlobal()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	beaconDB := dbtest.SetupDB(t)
	retentionEpochs := primitives.Epoch(2)

	slotTicker := &slottest.MockTicker{Channel: make(chan primitives.Slot)}

	// Create mock custody updater
	mockCustody := &mockCustodyUpdater{
		custodyGroupCount:     4,
		earliestAvailableSlot: 0,
	}

	// Create pruner with mock custody updater
	p, err := New(
		ctx,
		beaconDB,
		testClockWaiter(t),
		nil,
		nil,
		mockCustody,
		WithSlotTicker(slotTicker),
	)
	require.NoError(t, err)

	p.ps = func(current primitives.Slot) primitives.Slot {
		return current - primitives.Slot(retentionEpochs)*params.BeaconConfig().SlotsPerEpoch
	}

	// Save some blocks to be pruned
	var parentRoot [32]byte
	for i := primitives.Slot(1); i <= 32; i++ {
		blk := util.NewBeaconBlock()
		blk.Block.Slot = i
		blk.Block.ParentRoot = parentRoot[:]
		wsb, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
		parentRoot, err = wsb.Block().HashTreeRoot()
		require.NoError(t, err)
		if i == 1 {
			require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, parentRoot))
		}
	}
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &eth.Checkpoint{Epoch: 1, Root: parentRoot[:]}))

	// Start pruner and trigger at slot 80 (middle of 3rd epoch)
	go p.Start()
	currentSlot := primitives.Slot(80)
	slotTicker.Channel <- currentSlot

	// Wait for pruning to complete
	time.Sleep(100 * time.Millisecond)

	// Check that UpdateEarliestAvailableSlot was called
	assert.Equal(t, true, mockCustody.updateCallCount > 0, "UpdateEarliestAvailableSlot should have been called")

	// The earliest available slot is pruneUpto, whose block and state are kept as the replay
	// anchor: pruneUpto = currentSlot - retentionEpochs*slotsPerEpoch = 80 - 2*32 = 16.
	expectedEarliestSlot := primitives.Slot(16)
	require.Equal(t, expectedEarliestSlot, mockCustody.earliestAvailableSlot, "Earliest available slot should be updated correctly")
	require.Equal(t, uint64(4), mockCustody.custodyGroupCount, "Custody group count should be preserved")

	// Verify that no error was logged
	for _, entry := range hook.AllEntries() {
		if entry.Level == logrus.ErrorLevel {
			t.Errorf("Unexpected error log: %s", entry.Message)
		}
	}

	require.NoError(t, p.Stop())
}

// Mock custody updater that returns an error for UpdateEarliestAvailableSlot
type mockCustodyUpdaterWithUpdateError struct {
	updateCallCount int
}

func (m *mockCustodyUpdaterWithUpdateError) UpdateEarliestAvailableSlot(earliestAvailableSlot primitives.Slot) error {
	m.updateCallCount++
	return errors.New("failed to update earliest available slot")
}

func TestWithRetentionPeriod_EnforcesMinimum(t *testing.T) {
	// Use minimal config for testing
	params.SetupTestConfigCleanup(t)
	config := params.MinimalSpecConfig()
	params.OverrideBeaconConfig(config)

	ctx := t.Context()
	beaconDB := dbtest.SetupDB(t)

	// Get the minimum required epochs (272 + 1 = 273 for minimal)
	minRequiredEpochs := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests + 1)

	// Use a slot that's guaranteed to be after the minimum retention period
	currentSlot := primitives.Slot(minRequiredEpochs+100) * (params.BeaconConfig().SlotsPerEpoch)

	tests := []struct {
		name                string
		userRetentionEpochs primitives.Epoch
		expectedPruneSlot   primitives.Slot
		description         string
	}{
		{
			name:                "User value below minimum - should use minimum",
			userRetentionEpochs: 2, // Way below minimum
			expectedPruneSlot:   currentSlot - primitives.Slot(minRequiredEpochs)*params.BeaconConfig().SlotsPerEpoch,
			description:         "Should use minimum when user value is too low",
		},
		{
			name:                "User value at minimum",
			userRetentionEpochs: minRequiredEpochs,
			expectedPruneSlot:   currentSlot - primitives.Slot(minRequiredEpochs)*params.BeaconConfig().SlotsPerEpoch,
			description:         "Should use user value when at minimum",
		},
		{
			name:                "User value above minimum",
			userRetentionEpochs: minRequiredEpochs + 10,
			expectedPruneSlot:   currentSlot - primitives.Slot(minRequiredEpochs+10)*params.BeaconConfig().SlotsPerEpoch,
			description:         "Should use user value when above minimum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			logrus.SetLevel(logrus.WarnLevel)

			mockCustody := &mockCustodyUpdater{}
			// Create pruner with retention period
			p, err := New(
				ctx,
				beaconDB,
				testClockWaiter(t),
				nil,
				nil,
				mockCustody,
				WithRetentionPeriod(tt.userRetentionEpochs),
			)
			require.NoError(t, err)

			// Test the pruning calculation
			pruneUptoSlot := p.ps(currentSlot)

			// Verify the pruning slot
			assert.Equal(t, tt.expectedPruneSlot, pruneUptoSlot, tt.description)

			// Check if warning was logged when value was too low
			if tt.userRetentionEpochs < minRequiredEpochs {
				assert.LogsContain(t, hook, "Retention period too low, ignoring and using minimum required value")
			}
		})
	}
}

func TestPruneStartSlotFunc(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())

	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	retentionEpochs := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests + 1)
	retentionSlots := primitives.Slot(retentionEpochs) * slotsPerEpoch

	t.Run("clamps an overflowing retention period", func(t *testing.T) {
		for _, epochs := range []primitives.Epoch{slots.MaxSafeEpoch(), slots.MaxSafeEpoch() + 1, math.MaxUint64} {
			ps := pruneStartSlotFunc(epochs)
			require.Equal(t, primitives.Slot(0), ps(1_000_000))
		}
	})

	t.Run("aligns the cutoff on an epoch start", func(t *testing.T) {
		ps := pruneStartSlotFunc(retentionEpochs)

		epochStart := retentionSlots + 10*slotsPerEpoch
		for offsetInEpoch := primitives.Slot(0); offsetInEpoch < slotsPerEpoch; offsetInEpoch++ {
			require.Equal(t, epochStart-retentionSlots, ps(epochStart+offsetInEpoch))
		}

		require.Equal(t, epochStart-retentionSlots+slotsPerEpoch, ps(epochStart+slotsPerEpoch))
	})

	t.Run("prunes nothing before the retention period has elapsed", func(t *testing.T) {
		ps := pruneStartSlotFunc(retentionEpochs)

		require.Equal(t, primitives.Slot(0), ps(retentionSlots))
		require.Equal(t, primitives.Slot(0), ps(retentionSlots-1))
	})
}

func TestPruner_UpdateEarliestSlotError(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.FuluForkEpoch = 0 // Enable Fulu from epoch 0
	params.OverrideBeaconConfig(config)

	logrus.SetLevel(logrus.DebugLevel)
	hook := logTest.NewGlobal()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	beaconDB := dbtest.SetupDB(t)
	retentionEpochs := primitives.Epoch(2)

	slotTicker := &slottest.MockTicker{Channel: make(chan primitives.Slot)}

	// Create mock custody updater that returns an error for UpdateEarliestAvailableSlot
	mockCustody := &mockCustodyUpdaterWithUpdateError{}

	// Create pruner with mock custody updater
	p, err := New(
		ctx,
		beaconDB,
		testClockWaiter(t),
		nil,
		nil,
		mockCustody,
		WithSlotTicker(slotTicker),
	)
	require.NoError(t, err)

	p.ps = func(current primitives.Slot) primitives.Slot {
		return current - primitives.Slot(retentionEpochs)*params.BeaconConfig().SlotsPerEpoch
	}

	// Save some blocks to be pruned
	var parentRoot [32]byte
	for i := primitives.Slot(1); i <= 32; i++ {
		blk := util.NewBeaconBlock()
		blk.Block.Slot = i
		blk.Block.ParentRoot = parentRoot[:]
		wsb, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
		parentRoot, err = wsb.Block().HashTreeRoot()
		require.NoError(t, err)
		if i == 1 {
			require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, parentRoot))
		}
	}
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &eth.Checkpoint{Epoch: 1, Root: parentRoot[:]}))

	// Start pruner and trigger at slot 80
	go p.Start()
	currentSlot := primitives.Slot(80)
	slotTicker.Channel <- currentSlot

	// Wait for pruning to complete
	time.Sleep(100 * time.Millisecond)

	// Should have called UpdateEarliestAvailableSlot
	assert.Equal(t, 1, mockCustody.updateCallCount, "UpdateEarliestAvailableSlot should be called")

	// Check that error was logged by the prune function
	found := false
	for _, entry := range hook.AllEntries() {
		if entry.Level == logrus.ErrorLevel && entry.Message == "Failed to prune database" {
			found = true
			break
		}
	}
	assert.Equal(t, true, found, "Should log error when UpdateEarliestAvailableSlot fails")

	require.NoError(t, p.Stop())
}

func TestPruner_PruneFinalityWithStateDiff(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.MinEpochsForBlockRequests = 1
	config.FuluForkEpoch = 0
	params.OverrideBeaconConfig(config)
	reset := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer reset()
	previousFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{StateDiffExponents: []int{7, 5}})
	defer flags.Init(previousFlags)

	ctx := t.Context()
	beaconDB := dbtest.SetupDB(t)
	genesisState, err := util.NewBeaconStateFulu()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisData(ctx, genesisState))
	parentRoot, err := beaconDB.GenesisBlockRoot(ctx)
	require.NoError(t, err)
	roots := make(map[primitives.Slot][32]byte)
	for _, slot := range []primitives.Slot{31, 32, 63, 65, 96} {
		st := genesisState.Copy()
		require.NoError(t, st.SetSlot(slot))
		stateRoot, err := st.HashTreeRoot(ctx)
		require.NoError(t, err)
		blk := util.NewBeaconBlockFulu()
		blk.Block.Slot = slot
		blk.Block.ParentRoot = parentRoot[:]
		blk.Block.StateRoot = stateRoot[:]
		wsb, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
		root, err := wsb.Block().HashTreeRoot()
		require.NoError(t, err)
		roots[slot] = root
		parentRoot = root
		require.NoError(t, beaconDB.SaveStateSummary(ctx, &eth.StateSummary{Slot: slot, Root: root[:]}))
		if slot == 32 {
			require.NoError(t, beaconDB.SaveState(ctx, st, root))
		}
	}
	finalizedRoot := roots[63]
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &eth.Checkpoint{Epoch: 2, Root: finalizedRoot[:]}))
	custody := &mockCustodyUpdater{}
	p, err := New(ctx, beaconDB, testClockWaiter(t), nil, nil, custody)
	require.NoError(t, err)

	// The age cutoff is 96, but finality at 63 requires the replay boundary at 32.
	require.NoError(t, p.prune(160))
	require.Equal(t, false, beaconDB.HasBlock(ctx, roots[31]))
	for _, slot := range []primitives.Slot{32, 63, 65, 96} {
		require.Equal(t, true, beaconDB.HasBlock(ctx, roots[slot]), "block at slot %d", slot)
	}
	anchor, err := beaconDB.State(ctx, roots[32])
	require.NoError(t, err)
	require.NotNil(t, anchor)
	require.Equal(t, primitives.Slot(32), anchor.Slot())
	require.Equal(t, primitives.Slot(32), p.prunedUpto)
	require.Equal(t, primitives.Slot(32), custody.earliestAvailableSlot)
}
