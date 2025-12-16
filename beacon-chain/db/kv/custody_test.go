package kv

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	bolt "go.etcd.io/bbolt"
)

// getCustodyInfoFromDB reads the custody info directly from the database for testing purposes.
func getCustodyInfoFromDB(t *testing.T, db *Store) (primitives.Slot, uint64) {
	t.Helper()
	var earliestSlot primitives.Slot
	var groupCount uint64

	err := db.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(custodyBucket)
		if bucket == nil {
			return nil
		}

		// Read group count
		groupCountBytes := bucket.Get(groupCountKey)
		if len(groupCountBytes) != 0 {
			groupCount = bytesutil.BytesToUint64BigEndian(groupCountBytes)
		}

		// Read earliest available slot
		earliestSlotBytes := bucket.Get(earliestAvailableSlotKey)
		if len(earliestSlotBytes) != 0 {
			earliestSlot = primitives.Slot(bytesutil.BytesToUint64BigEndian(earliestSlotBytes))
		}

		return nil
	})
	require.NoError(t, err)

	return earliestSlot, groupCount
}

// getSubscriptionStatusFromDB reads the subscription status directly from the database for testing purposes.
func getSubscriptionStatusFromDB(t *testing.T, db *Store) bool {
	t.Helper()
	var subscribed bool

	err := db.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(custodyBucket)
		if bucket == nil {
			return nil
		}

		bytes := bucket.Get(subscribeAllDataSubnetsKey)
		if len(bytes) != 0 && bytes[0] == 1 {
			subscribed = true
		}

		return nil
	})
	require.NoError(t, err)

	return subscribed
}


func TestUpdateCustodyInfo(t *testing.T) {
	ctx := t.Context()

	t.Run("initial update with empty database", func(t *testing.T) {
		const (
			earliestSlot = primitives.Slot(100)
			groupCount   = uint64(5)
		)

		db := setupDB(t)

		slot, count, err := db.UpdateCustodyInfo(ctx, earliestSlot, groupCount)
		require.NoError(t, err)
		require.Equal(t, earliestSlot, slot)
		require.Equal(t, groupCount, count)

		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, earliestSlot, storedSlot)
		require.Equal(t, groupCount, storedCount)
	})

	t.Run("update with higher group count and higher slot", func(t *testing.T) {
		const (
			initialSlot  = primitives.Slot(100)
			initialCount = uint64(5)
			earliestSlot = primitives.Slot(200)
			groupCount   = uint64(10)
		)

		db := setupDB(t)

		_, _, err := db.UpdateCustodyInfo(ctx, initialSlot, initialCount)
		require.NoError(t, err)

		slot, count, err := db.UpdateCustodyInfo(ctx, earliestSlot, groupCount)
		require.NoError(t, err)
		require.Equal(t, earliestSlot, slot)
		require.Equal(t, groupCount, count)

		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, earliestSlot, storedSlot)
		require.Equal(t, groupCount, storedCount)
	})

	t.Run("update with higher group count and lower slot should preserve higher slot", func(t *testing.T) {
		// This is the bug scenario: when switching from normal mode to semi-supernode,
		// the incoming slot might be lower than the stored slot, but we should preserve
		// the higher stored slot to avoid advertising that we can serve data we don't have.
		const (
			initialSlot  = primitives.Slot(1835523) // Higher stored slot
			initialCount = uint64(10)
			earliestSlot = primitives.Slot(1835456) // Lower incoming slot (e.g., from head slot)
			groupCount   = uint64(64)               // Increasing custody (e.g., semi-supernode)
		)

		db := setupDB(t)

		_, _, err := db.UpdateCustodyInfo(ctx, initialSlot, initialCount)
		require.NoError(t, err)

		// When custody count increases but slot is lower, the higher slot should be preserved
		slot, count, err := db.UpdateCustodyInfo(ctx, earliestSlot, groupCount)
		require.NoError(t, err)
		require.Equal(t, initialSlot, slot, "earliestAvailableSlot should not decrease when custody group count increases")
		require.Equal(t, groupCount, count)

		// Verify in the database
		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, initialSlot, storedSlot, "stored slot should be the higher value")
		require.Equal(t, groupCount, storedCount)
	})

	t.Run("pre-fulu scenario: checkpoint sync before fork, restart with semi-supernode", func(t *testing.T) {
		// This test covers the pre-Fulu bug scenario:
		// 1. Node starts with checkpoint sync BEFORE Fulu fork - uses EarliestSlot() (checkpoint block slot)
		// 2. Validators connect after Fulu activates - maintainCustodyInfo() updates to head slot (higher)
		// 3. Node restarts with --semi-supernode - updateCustodyInfoInDB uses EarliestSlot() again
		// The bug was that step 3 would overwrite the higher slot from step 2.
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 100
		params.OverrideBeaconConfig(cfg)

		fuluForkSlot, err := slots.EpochStart(cfg.FuluForkEpoch)
		require.NoError(t, err)

		// Derive slot values relative to Fulu fork
		checkpointBlockSlot := fuluForkSlot - 10       // Checkpoint sync happened before Fulu
		headSlot := fuluForkSlot + 5                   // Head slot after Fulu activates
		defaultCustody := cfg.CustodyRequirement      // Default custody from config
		validatorCustody := cfg.CustodyRequirement + 6 // Custody after validators connect
		semiSupernodeCustody := cfg.NumberOfCustodyGroups // Semi-supernode custodies all groups

		// Verify our test setup: checkpoint is pre-Fulu, head is post-Fulu
		require.Equal(t, true, checkpointBlockSlot < fuluForkSlot, "checkpoint must be before Fulu fork")
		require.Equal(t, true, headSlot >= fuluForkSlot, "head must be at or after Fulu fork")

		db := setupDB(t)

		// Step 1: Node starts with checkpoint sync (pre-Fulu)
		// updateCustodyInfoInDB sees saved.Slot() < fuluForkSlot, so uses EarliestSlot()
		slot, count, err := db.UpdateCustodyInfo(ctx, checkpointBlockSlot, defaultCustody)
		require.NoError(t, err)
		require.Equal(t, checkpointBlockSlot, slot)
		require.Equal(t, defaultCustody, count)

		// Step 2: Validators connect after Fulu activates, maintainCustodyInfo() runs
		// Uses headSlot which is higher than checkpointBlockSlot
		slot, count, err = db.UpdateCustodyInfo(ctx, headSlot, validatorCustody)
		require.NoError(t, err)
		require.Equal(t, headSlot, slot, "should update to head slot")
		require.Equal(t, validatorCustody, count)

		// Verify step 2 stored correctly
		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, headSlot, storedSlot)
		require.Equal(t, validatorCustody, storedCount)

		// Step 3: Restart with --semi-supernode
		// updateCustodyInfoInDB sees saved.Slot() < fuluForkSlot, so uses EarliestSlot() again
		slot, count, err = db.UpdateCustodyInfo(ctx, checkpointBlockSlot, semiSupernodeCustody)
		require.NoError(t, err)
		require.Equal(t, headSlot, slot, "earliestAvailableSlot should NOT decrease back to checkpoint slot")
		require.Equal(t, semiSupernodeCustody, count)

		// Verify the database preserved the higher slot
		storedSlot, storedCount = getCustodyInfoFromDB(t, db)
		require.Equal(t, headSlot, storedSlot, "stored slot should remain at head slot, not checkpoint slot")
		require.Equal(t, semiSupernodeCustody, storedCount)
	})

	t.Run("post-fulu scenario: finalized slot lower than stored head slot", func(t *testing.T) {
		// This test covers the post-Fulu bug scenario:
		// Post-fork, updateCustodyInfoInDB uses saved.Slot() (finalized slot) directly,
		// not EarliestSlot(). But the same bug can occur because:
		// - maintainCustodyInfo() stores headSlot (higher)
		// - Restart uses finalized slot (lower than head)
		// Our fix ensures earliestAvailableSlot never decreases.
		params.SetupTestConfigCleanup(t)
		cfg := params.BeaconConfig()
		cfg.FuluForkEpoch = 100
		params.OverrideBeaconConfig(cfg)

		fuluForkSlot, err := slots.EpochStart(cfg.FuluForkEpoch)
		require.NoError(t, err)

		// Derive slot values relative to Fulu fork - all slots are AFTER Fulu
		finalizedSlotAtStart := fuluForkSlot + 100       // Finalized slot at first start (post-Fulu)
		headSlot := fuluForkSlot + 200                   // Head slot when validators connect
		finalizedSlotRestart := fuluForkSlot + 150       // Finalized slot at restart (< headSlot)
		defaultCustody := cfg.CustodyRequirement        // Default custody from config
		validatorCustody := cfg.CustodyRequirement + 6   // Custody after validators connect
		semiSupernodeCustody := cfg.NumberOfCustodyGroups // Semi-supernode custodies all groups

		// Verify our test setup: all slots are post-Fulu
		require.Equal(t, true, finalizedSlotAtStart >= fuluForkSlot, "finalized slot must be at or after Fulu fork")
		require.Equal(t, true, headSlot >= fuluForkSlot, "head slot must be at or after Fulu fork")
		require.Equal(t, true, finalizedSlotRestart >= fuluForkSlot, "restart finalized slot must be at or after Fulu fork")
		require.Equal(t, true, finalizedSlotRestart < headSlot, "restart finalized slot must be less than head slot")

		db := setupDB(t)

		// Step 1: Node starts post-Fulu
		// updateCustodyInfoInDB sees saved.Slot() >= fuluForkSlot, so uses saved.Slot() directly
		slot, count, err := db.UpdateCustodyInfo(ctx, finalizedSlotAtStart, defaultCustody)
		require.NoError(t, err)
		require.Equal(t, finalizedSlotAtStart, slot)
		require.Equal(t, defaultCustody, count)

		// Step 2: Validators connect, maintainCustodyInfo() uses head slot
		slot, count, err = db.UpdateCustodyInfo(ctx, headSlot, validatorCustody)
		require.NoError(t, err)
		require.Equal(t, headSlot, slot)
		require.Equal(t, validatorCustody, count)

		// Step 3: Restart with --semi-supernode
		// updateCustodyInfoInDB uses finalized slot which is lower than stored head slot
		slot, count, err = db.UpdateCustodyInfo(ctx, finalizedSlotRestart, semiSupernodeCustody)
		require.NoError(t, err)
		require.Equal(t, headSlot, slot, "earliestAvailableSlot should NOT decrease to finalized slot")
		require.Equal(t, semiSupernodeCustody, count)

		// Verify database preserved the higher slot
		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, headSlot, storedSlot)
		require.Equal(t, semiSupernodeCustody, storedCount)
	})

	t.Run("update with lower group count should not update", func(t *testing.T) {
		const (
			initialSlot  = primitives.Slot(200)
			initialCount = uint64(10)
			earliestSlot = primitives.Slot(300)
			groupCount   = uint64(8)
		)

		db := setupDB(t)

		_, _, err := db.UpdateCustodyInfo(ctx, initialSlot, initialCount)
		require.NoError(t, err)

		slot, count, err := db.UpdateCustodyInfo(ctx, earliestSlot, groupCount)
		require.NoError(t, err)
		require.Equal(t, initialSlot, slot)
		require.Equal(t, initialCount, count)

		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, initialSlot, storedSlot)
		require.Equal(t, initialCount, storedCount)
	})
}

func TestUpdateEarliestAvailableSlot(t *testing.T) {
	ctx := t.Context()

	t.Run("allow decreasing earliest slot (backfill scenario)", func(t *testing.T) {
		const (
			initialSlot  = primitives.Slot(300)
			initialCount = uint64(10)
			earliestSlot = primitives.Slot(200) // Lower than initial (backfill discovered earlier blocks)
		)

		db := setupDB(t)

		// Initialize custody info
		_, _, err := db.UpdateCustodyInfo(ctx, initialSlot, initialCount)
		require.NoError(t, err)

		// Update with a lower slot (should update for backfill)
		err = db.UpdateEarliestAvailableSlot(ctx, earliestSlot)
		require.NoError(t, err)

		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, earliestSlot, storedSlot)
		require.Equal(t, initialCount, storedCount)
	})

	t.Run("allow increasing slot within MIN_EPOCHS_FOR_BLOCK_REQUESTS (pruning scenario)", func(t *testing.T) {
		db := setupDB(t)

		// Calculate the current slot and minimum required slot based on actual current time
		genesisTime := time.Unix(int64(params.BeaconConfig().MinGenesisTime+params.BeaconConfig().GenesisDelay), 0)
		currentSlot := slots.CurrentSlot(genesisTime)
		currentEpoch := slots.ToEpoch(currentSlot)
		minEpochsForBlocks := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)

		var minRequiredEpoch primitives.Epoch
		if currentEpoch > minEpochsForBlocks {
			minRequiredEpoch = currentEpoch - minEpochsForBlocks
		} else {
			minRequiredEpoch = 0
		}

		minRequiredSlot, err := slots.EpochStart(minRequiredEpoch)
		require.NoError(t, err)

		// Initial setup: set earliest slot well before minRequiredSlot
		const groupCount = uint64(5)
		initialSlot := primitives.Slot(1000)

		_, _, err = db.UpdateCustodyInfo(ctx, initialSlot, groupCount)
		require.NoError(t, err)

		// Try to increase to a slot that's still BEFORE minRequiredSlot (should succeed)
		validSlot := minRequiredSlot - 100

		err = db.UpdateEarliestAvailableSlot(ctx, validSlot)
		require.NoError(t, err)

		// Verify the database was updated
		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, validSlot, storedSlot)
		require.Equal(t, groupCount, storedCount)
	})

	t.Run("prevent increasing slot beyond MIN_EPOCHS_FOR_BLOCK_REQUESTS", func(t *testing.T) {
		db := setupDB(t)

		// Calculate the current slot and minimum required slot based on actual current time
		genesisTime := time.Unix(int64(params.BeaconConfig().MinGenesisTime+params.BeaconConfig().GenesisDelay), 0)
		currentSlot := slots.CurrentSlot(genesisTime)
		currentEpoch := slots.ToEpoch(currentSlot)
		minEpochsForBlocks := primitives.Epoch(params.BeaconConfig().MinEpochsForBlockRequests)

		var minRequiredEpoch primitives.Epoch
		if currentEpoch > minEpochsForBlocks {
			minRequiredEpoch = currentEpoch - minEpochsForBlocks
		} else {
			minRequiredEpoch = 0
		}

		minRequiredSlot, err := slots.EpochStart(minRequiredEpoch)
		require.NoError(t, err)

		// Initial setup: set a valid earliest slot (well before minRequiredSlot)
		const initialCount = uint64(5)
		initialSlot := primitives.Slot(1000)

		_, _, err = db.UpdateCustodyInfo(ctx, initialSlot, initialCount)
		require.NoError(t, err)

		// Try to set earliest slot beyond the minimum required slot
		invalidSlot := minRequiredSlot + 100

		// This should fail
		err = db.UpdateEarliestAvailableSlot(ctx, invalidSlot)
		require.ErrorContains(t, "cannot increase earliest available slot", err)
		require.ErrorContains(t, "exceeds minimum required slot", err)

		// Verify the database wasn't updated
		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, initialSlot, storedSlot)
		require.Equal(t, initialCount, storedCount)
	})

	t.Run("no change when slot equals current slot", func(t *testing.T) {
		const (
			initialSlot  = primitives.Slot(100)
			initialCount = uint64(5)
		)

		db := setupDB(t)

		// Initialize custody info
		_, _, err := db.UpdateCustodyInfo(ctx, initialSlot, initialCount)
		require.NoError(t, err)

		// Update with the same slot
		err = db.UpdateEarliestAvailableSlot(ctx, initialSlot)
		require.NoError(t, err)

		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, initialSlot, storedSlot)
		require.Equal(t, initialCount, storedCount)
	})
}

func TestUpdateSubscribedToAllDataSubnets(t *testing.T) {
	ctx := context.Background()

	t.Run("initial update with empty database - set to false", func(t *testing.T) {
		db := setupDB(t)

		prev, err := db.UpdateSubscribedToAllDataSubnets(ctx, false)
		require.NoError(t, err)
		require.Equal(t, false, prev)

		stored := getSubscriptionStatusFromDB(t, db)
		require.Equal(t, false, stored)
	})

	t.Run("initial update with empty database - set to true", func(t *testing.T) {
		db := setupDB(t)

		prev, err := db.UpdateSubscribedToAllDataSubnets(ctx, true)
		require.NoError(t, err)
		require.Equal(t, false, prev)

		stored := getSubscriptionStatusFromDB(t, db)
		require.Equal(t, true, stored)
	})

	t.Run("attempt to update from true to false (should not change)", func(t *testing.T) {
		db := setupDB(t)

		_, err := db.UpdateSubscribedToAllDataSubnets(ctx, true)
		require.NoError(t, err)

		prev, err := db.UpdateSubscribedToAllDataSubnets(ctx, false)
		require.NoError(t, err)
		require.Equal(t, true, prev)

		stored := getSubscriptionStatusFromDB(t, db)
		require.Equal(t, true, stored)
	})

	t.Run("update from true to true (no change)", func(t *testing.T) {
		db := setupDB(t)

		_, err := db.UpdateSubscribedToAllDataSubnets(ctx, true)
		require.NoError(t, err)

		prev, err := db.UpdateSubscribedToAllDataSubnets(ctx, true)
		require.NoError(t, err)
		require.Equal(t, true, prev)

		stored := getSubscriptionStatusFromDB(t, db)
		require.Equal(t, true, stored)
	})
}
