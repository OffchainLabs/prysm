package kv

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/testing/require"
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

	t.Run("update with lower group count should not update group count", func(t *testing.T) {
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
		require.Equal(t, earliestSlot, slot) // Slot should be updated
		require.Equal(t, initialCount, count) // Count should stay the same

		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, earliestSlot, storedSlot) // Slot should be updated
		require.Equal(t, initialCount, storedCount) // Count should stay the same
	})

	t.Run("update earliest slot with same group count (pruning scenario)", func(t *testing.T) {
		const (
			initialSlot  = primitives.Slot(100)
			groupCount   = uint64(128) // Same custody group count
			earliestSlot1 = primitives.Slot(200)
			earliestSlot2 = primitives.Slot(300)
			earliestSlot3 = primitives.Slot(400)
		)

		db := setupDB(t)

		// Initial update
		_, _, err := db.UpdateCustodyInfo(ctx, initialSlot, groupCount)
		require.NoError(t, err)

		// First pruning event - custody group stays the same but earliest slot advances
		slot, count, err := db.UpdateCustodyInfo(ctx, earliestSlot1, groupCount)
		require.NoError(t, err)
		require.Equal(t, earliestSlot1, slot)
		require.Equal(t, groupCount, count)

		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, earliestSlot1, storedSlot)
		require.Equal(t, groupCount, storedCount)

		// Second pruning event
		slot, count, err = db.UpdateCustodyInfo(ctx, earliestSlot2, groupCount)
		require.NoError(t, err)
		require.Equal(t, earliestSlot2, slot)
		require.Equal(t, groupCount, count)

		storedSlot, storedCount = getCustodyInfoFromDB(t, db)
		require.Equal(t, earliestSlot2, storedSlot)
		require.Equal(t, groupCount, storedCount)

		// Third pruning event
		slot, count, err = db.UpdateCustodyInfo(ctx, earliestSlot3, groupCount)
		require.NoError(t, err)
		require.Equal(t, earliestSlot3, slot)
		require.Equal(t, groupCount, count)

		storedSlot, storedCount = getCustodyInfoFromDB(t, db)
		require.Equal(t, earliestSlot3, storedSlot)
		require.Equal(t, groupCount, storedCount)
	})

	t.Run("should not update with lower earliest slot", func(t *testing.T) {
		const (
			initialSlot  = primitives.Slot(300)
			initialCount = uint64(10)
			earliestSlot = primitives.Slot(200) // Lower than initial
			groupCount   = uint64(10)
		)

		db := setupDB(t)

		_, _, err := db.UpdateCustodyInfo(ctx, initialSlot, initialCount)
		require.NoError(t, err)

		// Try to update with a lower slot (should not update)
		slot, count, err := db.UpdateCustodyInfo(ctx, earliestSlot, groupCount)
		require.NoError(t, err)
		require.Equal(t, initialSlot, slot) // Should keep the higher slot
		require.Equal(t, initialCount, count)

		storedSlot, storedCount := getCustodyInfoFromDB(t, db)
		require.Equal(t, initialSlot, storedSlot) // Should keep the higher slot
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
