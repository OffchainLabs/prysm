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
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/libp2p/go-libp2p/core/peer"
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
			p, err := New(ctx, beaconDB, time.Now(), initSyncWaiter, backfillWaiter, WithSlotTicker(slotTicker))
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

	p, err := New(
		ctx,
		beaconDB,
		time.Now(),
		nil,
		nil,
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

// Mock P2P service for testing
type mockP2PService struct {
	custodyGroupCount     uint64
	earliestAvailableSlot primitives.Slot
	updateCallCount       int
	lastUpdatedSlot       primitives.Slot
	lastUpdatedCount      uint64
}

func (m *mockP2PService) EarliestAvailableSlot() (primitives.Slot, error) {
	return m.earliestAvailableSlot, nil
}

func (m *mockP2PService) CustodyGroupCount() (uint64, error) {
	return m.custodyGroupCount, nil
}

func (m *mockP2PService) UpdateCustodyInfo(earliestAvailableSlot primitives.Slot, custodyGroupCount uint64) (primitives.Slot, uint64, error) {
	m.updateCallCount++
	m.lastUpdatedSlot = earliestAvailableSlot
	m.lastUpdatedCount = custodyGroupCount
	m.earliestAvailableSlot = earliestAvailableSlot
	return earliestAvailableSlot, custodyGroupCount, nil
}

func (m *mockP2PService) CustodyGroupCountFromPeer(pid peer.ID) uint64 {
	return m.custodyGroupCount
}

func TestPruner_UpdatesEarliestAvailableSlot(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	hook := logTest.NewGlobal()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	beaconDB := dbtest.SetupDB(t)
	retentionEpochs := primitives.Epoch(2)

	slotTicker := &slottest.MockTicker{Channel: make(chan primitives.Slot)}

	// Create mock P2P service
	mockP2P := &mockP2PService{
		custodyGroupCount:     4,
		earliestAvailableSlot: 0,
	}

	// Create pruner with mock P2P service
	p, err := New(
		ctx,
		beaconDB,
		time.Now(),
		nil,
		nil,
		WithSlotTicker(slotTicker),
		WithP2PService(mockP2P),
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

	// Check that UpdateCustodyInfo was called
	assert.Equal(t, true, mockP2P.updateCallCount > 0, "UpdateCustodyInfo should have been called")

	// The earliest available slot should be pruneUpto + 1
	// pruneUpto = currentSlot - retentionEpochs*slotsPerEpoch = 80 - 2*32 = 16
	// So earliest available slot should be 16 + 1 = 17
	expectedEarliestSlot := primitives.Slot(17)
	require.Equal(t, expectedEarliestSlot, mockP2P.lastUpdatedSlot, "Earliest available slot should be updated correctly")
	require.Equal(t, mockP2P.custodyGroupCount, mockP2P.lastUpdatedCount, "Custody group count should be preserved")

	// Check log entries
	found := false
	for _, entry := range hook.AllEntries() {
		if entry.Message == "Updated earliest available slot after pruning" {
			found = true
			require.Equal(t, expectedEarliestSlot, entry.Data["earliestAvailableSlot"])
		}
	}
	assert.Equal(t, true, found, "Should log successful earliest available slot update")

	require.NoError(t, p.Stop())
}

// Mock P2P service that returns an error for CustodyGroupCount
type mockP2PServiceWithError struct {
	updateCallCount  int
	lastUpdatedSlot  primitives.Slot
	lastUpdatedCount uint64
}

func (m *mockP2PServiceWithError) EarliestAvailableSlot() (primitives.Slot, error) {
	return 0, nil
}

func (m *mockP2PServiceWithError) CustodyGroupCount() (uint64, error) {
	return 0, errors.New("custody group count error")
}

func (m *mockP2PServiceWithError) UpdateCustodyInfo(earliestAvailableSlot primitives.Slot, custodyGroupCount uint64) (primitives.Slot, uint64, error) {
	m.updateCallCount++
	m.lastUpdatedSlot = earliestAvailableSlot
	m.lastUpdatedCount = custodyGroupCount
	return earliestAvailableSlot, custodyGroupCount, nil
}

func (m *mockP2PServiceWithError) CustodyGroupCountFromPeer(pid peer.ID) uint64 {
	return 4
}

func TestPruner_SkipsUpdateOnCustodyGroupCountError(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	hook := logTest.NewGlobal()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	beaconDB := dbtest.SetupDB(t)
	retentionEpochs := primitives.Epoch(2)

	slotTicker := &slottest.MockTicker{Channel: make(chan primitives.Slot)}

	// Create mock P2P service that returns an error for CustodyGroupCount
	mockP2P := &mockP2PServiceWithError{}

	// Create pruner with mock P2P service
	p, err := New(
		ctx,
		beaconDB,
		time.Now(),
		nil,
		nil,
		WithSlotTicker(slotTicker),
		WithP2PService(mockP2P),
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

	// Should not have called UpdateCustodyInfo due to error
	assert.Equal(t, 0, mockP2P.updateCallCount, "UpdateCustodyInfo should not be called when CustodyGroupCount fails")

	// Check error log
	found := false
	for _, entry := range hook.AllEntries() {
		if entry.Message == "Failed to get custody group count, cannot update earliest available slot after pruning" {
			found = true
			break
		}
	}
	assert.Equal(t, true, found, "Should log error when custody group count fails")

	require.NoError(t, p.Stop())
}
