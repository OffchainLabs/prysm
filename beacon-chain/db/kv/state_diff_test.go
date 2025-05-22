package kv

import (
	"context"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/math"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"go.etcd.io/bbolt"
)

func TestStateDiff_LoadOrInitOffset(t *testing.T) {
	db := setupDB(t)

	offset, err := db.loadOrInitOffset(10)
	require.NoError(t, err)
	require.Equal(t, uint64(10), offset)

	offset, err = db.loadOrInitOffset(20)
	require.NoError(t, err)
	require.Equal(t, uint64(10), offset)

	offset, err = db.loadOrInitOffset(5)
	require.NoError(t, err)
	require.Equal(t, uint64(10), offset)
}

func TestStateDiff_ComputeLevel(t *testing.T) {
	db := setupDB(t)

	offset, err := db.loadOrInitOffset(0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), offset)

	// 2 ** 21
	lvl := computeLevel(offset, primitives.Slot(math.PowerOf2(21)))
	require.Equal(t, 0, lvl)

	// 2 ** 21 * 3
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(21)*3))
	require.Equal(t, 0, lvl)

	// 2 ** 18
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(18)))
	require.Equal(t, 1, lvl)

	// 2 ** 18 * 3
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(18)*3))
	require.Equal(t, 1, lvl)

	// 2 ** 16
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(16)))
	require.Equal(t, 2, lvl)

	// 2 ** 16 * 3
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(16)*3))
	require.Equal(t, 2, lvl)

	// 2 ** 13
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(13)))
	require.Equal(t, 3, lvl)

	// 2 ** 13 * 3
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(13)*3))
	require.Equal(t, 3, lvl)

	// 2 ** 11
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(11)))
	require.Equal(t, 4, lvl)

	// 2 ** 11 * 3
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(11)*3))
	require.Equal(t, 4, lvl)

	// 2 ** 9
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(9)))
	require.Equal(t, 5, lvl)

	// 2 ** 9 * 3
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(9)*3))
	require.Equal(t, 5, lvl)

	// 2 ** 5
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(5)))
	require.Equal(t, 6, lvl)

	// 2 ** 5 * 3
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(5)*3))
	require.Equal(t, 6, lvl)

	// 2 ** 7
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(7)))
	require.Equal(t, 6, lvl)

	// 2 ** 5 + 1
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(5)+1))
	require.Equal(t, -1, lvl)

	// 2 ** 5 + 16
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(5)+16))
	require.Equal(t, -1, lvl)

	// 2 ** 5 + 32
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(5)+32))
	require.Equal(t, 6, lvl)

}

func TestStateDiff_SaveFullSnapshot(t *testing.T) {
	db := setupDB(t)

	// Create state with slot 0
	st, err := util.NewBeaconStateElectra()
	require.NoError(t, err)
	slot := primitives.Slot(0)
	err = st.SetSlot(slot)
	require.NoError(t, err)
	stssz, err := st.MarshalSSZ()
	require.NoError(t, err)
	enc, err := addKey(version.Electra, stssz)
	require.NoError(t, err)

	err = db.SaveStateDiff(context.Background(), st)
	require.NoError(t, err)

	err = db.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		s := bucket.Get(makeKey(0, uint64(slot)))
		if s == nil {
			return bbolt.ErrIncompatibleValue
		}
		require.DeepSSZEqual(t, enc, s)
		return nil
	})
	require.NoError(t, err)
}

func TestStateDiff_SaveDiff(t *testing.T) {
	db := setupDB(t)

	// Create state with slot 2**21
	slot := primitives.Slot(math.PowerOf2(21))
	st, enc := createState(t, slot, version.Electra)

	err := db.SaveStateDiff(context.Background(), st)
	require.NoError(t, err)

	err = db.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		s := bucket.Get(makeKey(0, uint64(slot)))
		if s == nil {
			return bbolt.ErrIncompatibleValue
		}
		require.DeepSSZEqual(t, enc, s)
		return nil
	})
	require.NoError(t, err)

	// create state with slot 2**18 (+2**21)
	slot = primitives.Slot(math.PowerOf2(18) + math.PowerOf2(21))
	st, _ = createState(t, slot, version.Electra)

	err = db.SaveStateDiff(context.Background(), st)
	require.NoError(t, err)

	key := makeKey(1, uint64(slot))
	err = db.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		buf := append(key, "_s"...)
		s := bucket.Get(buf)
		if s == nil {
			return bbolt.ErrIncompatibleValue
		}
		buf = append(key, "_v"...)
		v := bucket.Get(buf)
		if v == nil {
			return bbolt.ErrIncompatibleValue
		}
		buf = append(key, "_b"...)
		b := bucket.Get(buf)
		if b == nil {
			return bbolt.ErrIncompatibleValue
		}
		return nil
	})
	require.NoError(t, err)
}

func TestStateDiff_OffsetCache(t *testing.T) {
	// test for slot numbers 0 and 1 for every version
	for slotNum := 0; slotNum < 2; slotNum++ {
		// test for every version
		for v := 0; v < 6; v++ {
			t.Run(fmt.Sprintf("slotNum=%d,%s", slotNum, version.String(v)), func(t *testing.T) {
				db := setupDB(t)

				_, err := db.stateDiffCache.getOffset()
				require.ErrorContains(t, "offset is not set", err)

				slot := primitives.Slot(slotNum)
				st, _ := createState(t, slot, v)
				err = db.SaveStateDiff(context.Background(), st)
				require.NoError(t, err)

				offset, err := db.stateDiffCache.getOffset()
				require.NoError(t, err)
				require.Equal(t, uint64(slotNum), offset)

				slot2 := primitives.Slot(uint64(slotNum) + math.PowerOf2(params.StateHierarchyExponents()[0]))
				st2, _ := createState(t, slot2, v)
				err = db.SaveStateDiff(context.Background(), st2)
				require.NoError(t, err)

				offset, err = db.stateDiffCache.getOffset()
				require.NoError(t, err)
				require.Equal(t, uint64(slot), offset)
			})
		}
	}
}

func TestStateDiff_AnchorCache(t *testing.T) {
	// test for every version
	for v := 1; v < 6; v++ {
		t.Run(version.String(v), func(t *testing.T) {
			exponents := params.StateHierarchyExponents()
			localCache := make([]state.ReadOnlyBeaconState, len(exponents)-1)
			db := setupDB(t)

			// first the cache should be empty
			for i := 0; i < len(params.StateHierarchyExponents()); i++ {
				anchor := db.stateDiffCache.getAnchor(i)
				require.IsNil(t, anchor)
			}

			// add state on every level and check cache

			// add level 0 first
			slot := primitives.Slot(0)
			st, _ := createState(t, slot, v)
			err := db.SaveStateDiff(context.Background(), st)
			require.NoError(t, err)
			localCache[0] = st

			// anchor cache must match local cache
			for i := 0; i < len(exponents)-1; i++ {
				if localCache[i] == nil {
					require.IsNil(t, db.stateDiffCache.getAnchor(i))
					continue
				}
				localSSZ, err := localCache[i].MarshalSSZ()
				require.NoError(t, err)
				anchorSSZ, err := db.stateDiffCache.getAnchor(i).MarshalSSZ()
				require.NoError(t, err)
				require.DeepEqual(t, localSSZ, anchorSSZ)
			}

			// skip last level as it does not get cached
			for i := len(exponents) - 2; i > 0; i-- {
				slot = primitives.Slot(math.PowerOf2(exponents[i]))
				st, _ := createState(t, slot, v)
				err = db.SaveStateDiff(context.Background(), st)
				require.NoError(t, err)
				localCache[i] = st

				// anchor cache must match local cache
				for i := 0; i < len(exponents)-1; i++ {
					if localCache[i] == nil {
						require.IsNil(t, db.stateDiffCache.getAnchor(i))
						continue
					}
					localSSZ, err := localCache[i].MarshalSSZ()
					require.NoError(t, err)
					anchorSSZ, err := db.stateDiffCache.getAnchor(i).MarshalSSZ()
					require.NoError(t, err)
					require.DeepEqual(t, localSSZ, anchorSSZ)
				}
			}
		})
	}
}

func createState(t *testing.T, slot primitives.Slot, v int) (state.ReadOnlyBeaconState, []byte) {
	var st state.BeaconState
	var err error
	switch v {
	case version.Altair:
		st, err = util.NewBeaconStateAltair()
		require.NoError(t, err)
	case version.Bellatrix:
		st, err = util.NewBeaconStateBellatrix()
		require.NoError(t, err)
	case version.Capella:
		st, err = util.NewBeaconStateCapella()
		require.NoError(t, err)
	case version.Deneb:
		st, err = util.NewBeaconStateDeneb()
		require.NoError(t, err)
	case version.Electra:
		st, err = util.NewBeaconStateElectra()
		require.NoError(t, err)
	default:
		st, err = util.NewBeaconState()
		require.NoError(t, err)
	}

	err = st.SetSlot(slot)
	require.NoError(t, err)
	stssz, err := st.MarshalSSZ()
	require.NoError(t, err)
	enc, err := addKey(v, stssz)
	require.NoError(t, err)
	return st, enc
}
