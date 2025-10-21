package pruner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	eth "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"

	"github.com/OffchainLabs/prysm/v6/testing/util"
	slottest "github.com/OffchainLabs/prysm/v6/time/slots/testing"
	"github.com/sirupsen/logrus"

	dbtest "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
)

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
			p, err := New(ctx, beaconDB, time.Now(), initSyncWaiter, backfillWaiter, mockCustody, WithSlotTicker(slotTicker))
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
	ctx := t.Context()
	beaconDB := dbtest.SetupDB(t)

	// Create and save some blocks at different slots
	var blks []*eth.SignedBeaconBlock
	for slot := primitives.Slot(1); slot <= 32; slot++ {
		blk := util.NewBeaconBlock()
		blk.Block.Slot = slot
		wsb, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
		blks = append(blks, blk)
	}

	// Create pruner with retention of 2 epochs (64 slots)
	retentionEpochs := primitives.Epoch(2)
	slotTicker := &slottest.MockTicker{Channel: make(chan primitives.Slot)}

	mockCustody := &mockCustodyUpdater{}
	p, err := New(
		ctx,
		beaconDB,
		time.Now(),
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
		if slot <= 16 { // These should be pruned
			require.NoError(t, err)
			require.Equal(t, false, present, "Expected present at slot %d to be pruned", slot)
		} else { // These should remain
			require.NoError(t, err)
			require.Equal(t, true, present, "Expected present at slot %d to exist", slot)
		}
	}

	require.NoError(t, p.Stop())
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
		time.Now(),
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
	for i := primitives.Slot(1); i <= 32; i++ {
		blk := util.NewBeaconBlock()
		blk.Block.Slot = i
		wsb, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
	}

	// Start pruner and trigger at slot 80 (middle of 3rd epoch)
	go p.Start()
	currentSlot := primitives.Slot(80)
	slotTicker.Channel <- currentSlot

	// Wait for pruning to complete
	time.Sleep(100 * time.Millisecond)

	// Check that UpdateEarliestAvailableSlot was called
	assert.Equal(t, true, mockCustody.updateCallCount > 0, "UpdateEarliestAvailableSlot should have been called")

	// The earliest available slot should be pruneUpto + 1
	// pruneUpto = currentSlot - retentionEpochs*slotsPerEpoch = 80 - 2*32 = 16
	// So earliest available slot should be 16 + 1 = 17
	expectedEarliestSlot := primitives.Slot(17)
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
				time.Now(),
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
		time.Now(),
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
	for i := primitives.Slot(1); i <= 32; i++ {
		blk := util.NewBeaconBlock()
		blk.Block.Slot = i
		wsb, err := blocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
	}

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
