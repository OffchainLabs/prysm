package pruner

import (
	"context"
	"testing"
	"time"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	eth "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"

	"github.com/prysmaticlabs/prysm/v5/testing/util"
	slottest "github.com/prysmaticlabs/prysm/v5/time/slots/testing"
	"github.com/sirupsen/logrus"

	dbtest "github.com/prysmaticlabs/prysm/v5/beacon-chain/db/testing"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
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
			expectedLog:       "Skipping pruning as initial sync is in progress",
		},
		{
			name:              "Backfill incomplete",
			synced:            true,
			backfillCompleted: false,
			expectedLog:       "Skipping pruning as backfill is in progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logrus.SetLevel(logrus.DebugLevel)
			hook := logTest.NewGlobal()
			t.Log(hook.Levels())
			ctx := context.Background()
			beaconDB := dbtest.SetupDB(t)

			slotTicker := &slottest.MockTicker{Channel: make(chan primitives.Slot)}

			p, err := New(ctx, beaconDB, time.Now(), &mockSyncChecker{synced: tt.synced}, &mockBackfillChecker{complete: tt.backfillCompleted}, WithSlotTicker(slotTicker))
			require.NoError(t, err)

			p.Start()
			slotTicker.Channel <- 16
			slotTicker.Channel <- 16

			if tt.expectedLog != "" {
				require.LogsContain(t, hook, tt.expectedLog)
			}

			require.NoError(t, p.Stop())
		})
	}
}

func TestPruner_PruneSuccess(t *testing.T) {
	ctx := context.Background()
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
		&mockSyncChecker{synced: true},
		&mockBackfillChecker{complete: true},
		WithSlotTicker(slotTicker),
	)
	require.NoError(t, err)

	p.ps = func(current primitives.Slot) primitives.Slot {
		return current - primitives.Slot(retentionEpochs)*params.BeaconConfig().SlotsPerEpoch
	}

	// Start pruner and trigger at middle of 3rd epoch (slot 80)
	p.Start()
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

// Mock implementations
type mockSyncChecker struct {
	synced bool
}

func (m *mockSyncChecker) Synced() bool {
	return m.synced
}

type mockBackfillChecker struct {
	complete bool
}

func (m *mockBackfillChecker) IsComplete() bool {
	return m.complete
}
