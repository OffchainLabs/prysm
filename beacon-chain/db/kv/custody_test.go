package kv

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/time/slots"
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

	t.Run("update with higher group count", func(t *testing.T) {
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

	t.Run("attempt to update from true to false (should not change)", func(t *testing.T) {
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
