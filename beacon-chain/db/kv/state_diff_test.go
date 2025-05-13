package kv

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/math"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"go.etcd.io/bbolt"
)

func TestStateDiff_LoadOrInitOffset(t *testing.T) {
	db := setupDB(t)

	offset, err := loadOrInitOffset(db, 10)
	require.NoError(t, err)
	require.Equal(t, uint64(10), offset)

	offset, err = loadOrInitOffset(db, 20)
	require.NoError(t, err)
	require.Equal(t, uint64(10), offset)

	offset, err = loadOrInitOffset(db, 5)
	require.NoError(t, err)
	require.Equal(t, uint64(10), offset)
}

func TestStateDiff_ComputeLevel(t *testing.T) {
	db := setupDB(t)

	offset, err := loadOrInitOffset(db, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), offset)

	// 2 ** 21
	lvl, shouldSave := computeLevel(math.PowerOf2(21))
	require.Equal(t, 0, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 21 * 3
	lvl, shouldSave = computeLevel(math.PowerOf2(21) * 3)
	require.Equal(t, 0, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 18
	lvl, shouldSave = computeLevel(math.PowerOf2(18))
	require.Equal(t, 1, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 18 * 3
	lvl, shouldSave = computeLevel(math.PowerOf2(18) * 3)
	require.Equal(t, 1, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 16
	lvl, shouldSave = computeLevel(math.PowerOf2(16))
	require.Equal(t, 2, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 16 * 3
	lvl, shouldSave = computeLevel(math.PowerOf2(16) * 3)
	require.Equal(t, 2, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 13
	lvl, shouldSave = computeLevel(math.PowerOf2(13))
	require.Equal(t, 3, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 13 * 3
	lvl, shouldSave = computeLevel(math.PowerOf2(13) * 3)
	require.Equal(t, 3, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 11
	lvl, shouldSave = computeLevel(math.PowerOf2(11))
	require.Equal(t, 4, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 11 * 3
	lvl, shouldSave = computeLevel(math.PowerOf2(11) * 3)
	require.Equal(t, 4, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 9
	lvl, shouldSave = computeLevel(math.PowerOf2(9))
	require.Equal(t, 5, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 9 * 3
	lvl, shouldSave = computeLevel(math.PowerOf2(9) * 3)
	require.Equal(t, 5, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 5
	lvl, shouldSave = computeLevel(math.PowerOf2(5))
	require.Equal(t, 6, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 5 * 3
	lvl, shouldSave = computeLevel(math.PowerOf2(5) * 3)
	require.Equal(t, 6, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 7
	lvl, shouldSave = computeLevel(math.PowerOf2(7))
	require.Equal(t, 6, lvl)
	require.Equal(t, true, shouldSave)

	// 2 ** 5 + 1
	lvl, shouldSave = computeLevel(math.PowerOf2(5) + 1)
	require.Equal(t, false, shouldSave)

	// 2 ** 5 + 16
	lvl, shouldSave = computeLevel(math.PowerOf2(5) + 16)
	require.Equal(t, false, shouldSave)

	// 2 ** 5 + 32
	lvl, shouldSave = computeLevel(math.PowerOf2(5) + 32)
	require.Equal(t, true, shouldSave)
	require.Equal(t, 6, lvl)

}

func TestStateDiff_SaveFullSnapshot(t *testing.T) {
	db := setupDB(t)

	// Set offset to zero
	offset, err := loadOrInitOffset(db, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), offset)

	// Create state with slot 2**21 * 3
	st, err := util.NewBeaconStateElectra()
	require.NoError(t, err)
	slot := primitives.Slot(math.PowerOf2(21))
	err = st.SetSlot(slot)
	require.NoError(t, err)
	stssz, err := st.MarshalSSZ()
	require.NoError(t, err)

	err = db.SaveStateDiff(nil, st)
	require.NoError(t, err)

	err = db.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		for i := 6; i >= 0; i-- {
			s := bucket.Get(makeKey(i, uint64(slot)))
			require.NotNil(t, s, "key not found")
			if i > 0 {
				require.DeepEqual(t, EmptyNodeMarker, s, "node not marked as empty")
			} else {
				require.DeepSSZEqual(t, stssz, s, "retrieved state does not match saved state")
			}
		}
		return nil
	})
	require.NoError(t, err)
}
