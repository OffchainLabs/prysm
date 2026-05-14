package kv

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/math"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"go.etcd.io/bbolt"
)

func TestStateDiff_LoadOrInitOffset(t *testing.T) {
	setDefaultStateDiffExponents()

	db := setupDB(t)
	err := setOffsetInDB(db, 10)
	require.NoError(t, err)
	offset := db.getOffset()
	require.Equal(t, uint64(10), offset)

	err = db.setOffset(10)
	require.ErrorContains(t, "offset already set", err)
	offset = db.getOffset()
	require.Equal(t, uint64(10), offset)
}

func TestStateDiff_LoadOffset(t *testing.T) {
	setDefaultStateDiffExponents()

	db := setupDB(t)
	_, err := db.loadOffset()
	require.ErrorContains(t, "offset not found", err)

	err = setOffsetInDB(db, 10)
	require.NoError(t, err)
	offset, err := db.loadOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(10), offset)
}

func TestStateDiff_EncodeDecodeExponents(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		exponents := []int{21, 18, 16, 13}
		encoded, err := encodeStateDiffExponents(exponents)
		require.NoError(t, err)
		decoded, err := decodeStateDiffExponents(encoded)
		require.NoError(t, err)
		require.DeepEqual(t, exponents, decoded)
	})

	t.Run("encode-empty", func(t *testing.T) {
		_, err := encodeStateDiffExponents(nil)
		require.ErrorContains(t, "cannot be empty", err)
	})

	t.Run("encode-negative", func(t *testing.T) {
		_, err := encodeStateDiffExponents([]int{21, -1})
		require.ErrorContains(t, "out of range", err)
		require.ErrorContains(t, "between 2 and", err)
	})

	t.Run("encode-too-large", func(t *testing.T) {
		_, err := encodeStateDiffExponents([]int{flags.MaxStateDiffExponent + 1})
		require.ErrorContains(t, "out of range", err)
		require.ErrorContains(t, "between 2 and", err)
	})

	t.Run("decode-empty", func(t *testing.T) {
		_, err := decodeStateDiffExponents(nil)
		require.ErrorContains(t, "missing length prefix", err)
	})

	t.Run("decode-zero-length", func(t *testing.T) {
		_, err := decodeStateDiffExponents([]byte{0})
		require.ErrorContains(t, "length cannot be zero", err)
	})

	t.Run("decode-length-mismatch", func(t *testing.T) {
		_, err := decodeStateDiffExponents([]byte{2, 10})
		require.ErrorContains(t, "length mismatch", err)
	})

	t.Run("decode-too-many-exponents", func(t *testing.T) {
		encoded := make([]byte, 17)
		encoded[0] = 16
		_, err := decodeStateDiffExponents(encoded)
		require.ErrorContains(t, "exceeds max 15", err)
	})

	t.Run("decode-out-of-range", func(t *testing.T) {
		_, err := decodeStateDiffExponents([]byte{1, byte(flags.MaxStateDiffExponent + 1)})
		require.ErrorContains(t, "out of range when decoding", err)
	})

	t.Run("decode-not-decreasing", func(t *testing.T) {
		_, err := decodeStateDiffExponents([]byte{2, 10, 10})
		require.ErrorContains(t, "strictly decreasing", err)
	})

	t.Run("decode-last-too-small", func(t *testing.T) {
		_, err := decodeStateDiffExponents([]byte{2, 10, 4})
		require.ErrorContains(t, "last state diff exponent must be at least 5", err)
	})
}

func TestStateDiff_InitializeStoresExponents(t *testing.T) {
	setDefaultStateDiffExponents()
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()

	db := setupDB(t)
	st, _ := createState(t, 0, version.Phase0)
	require.NoError(t, db.initializeStateDiff(0, st))

	stored, err := db.loadStateDiffExponents()
	require.NoError(t, err)
	require.DeepEqual(t, flags.Get().StateDiffExponents, stored)
}

func TestStateDiff_LoadExponentsMissing(t *testing.T) {
	db := setupDB(t)
	_, err := db.loadStateDiffExponents()
	require.ErrorContains(t, "exponents metadata not found", err)
}

func TestStateDiff_ComputeLevel(t *testing.T) {
	db := setupDB(t)
	setDefaultStateDiffExponents()

	err := setOffsetInDB(db, 0)
	require.NoError(t, err)

	offset := db.getOffset()

	// should be -1. slot < offset
	lvl := computeLevel(10, primitives.Slot(9))
	require.Equal(t, -1, lvl)

	// 2 ** 21
	lvl = computeLevel(offset, primitives.Slot(math.PowerOf2(21)))
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
	setDefaultStateDiffExponents()

	for v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			db := setupDB(t)

			// Create state with slot 0
			st, enc := createState(t, 0, v)

			err := setOffsetInDB(db, 0)
			require.NoError(t, err)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			err = db.db.View(func(tx *bbolt.Tx) error {
				bucket := tx.Bucket(stateDiffBucket)
				if bucket == nil {
					return bbolt.ErrBucketNotFound
				}
				s := bucket.Get(makeKeyForStateDiffTree(0, uint64(0)))
				if s == nil {
					return bbolt.ErrIncompatibleValue
				}
				require.DeepSSZEqual(t, enc, s)
				return nil
			})
			require.NoError(t, err)
		})
	}
}

func TestStateDiff_StateByDiff_NonZeroOffsetSkipsRedundantLevelDiff(t *testing.T) {
	setStateDiffExponents([]int{6, 5, 4})

	db := setupDB(t)

	offset := uint64(1000)
	require.NoError(t, setOffsetInDB(db, offset))

	stOffset, _ := createState(t, primitives.Slot(offset), version.Phase0)
	require.NoError(t, db.saveStateByDiff(context.Background(), stOffset))

	st32, _ := createState(t, primitives.Slot(offset+32), version.Phase0)
	require.NoError(t, db.saveStateByDiff(context.Background(), st32))

	st64, _ := createState(t, primitives.Slot(offset+64), version.Phase0)
	require.NoError(t, db.saveStateByDiff(context.Background(), st64))

	st80, _ := createState(t, primitives.Slot(offset+80), version.Phase0)
	require.NoError(t, db.saveStateByDiff(context.Background(), st80))

	readSt, err := db.stateByDiff(context.Background(), primitives.Slot(offset+80))
	require.NoError(t, err)

	stWantSSZ, err := st80.MarshalSSZ()
	require.NoError(t, err)
	stGotSSZ, err := readSt.MarshalSSZ()
	require.NoError(t, err)
	require.DeepSSZEqual(t, stWantSSZ, stGotSSZ)
}

func TestStateDiff_PopulateStateDiffCacheFromDB(t *testing.T) {
	setDefaultStateDiffExponents()

	db := setupDB(t)
	_, err := populateStateDiffCacheFromDB(db, 0)
	require.ErrorContains(t, "offset snapshot", err)

	st, _ := createState(t, 0, version.Phase0)
	require.NoError(t, setOffsetInDB(db, 0))
	require.NoError(t, db.saveStateByDiff(context.Background(), st))

	err = db.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		key := makeKeyForStateDiffTree(2, math.PowerOf2(16))
		if err := bucket.Put(append(key, stateSuffix...), []byte{1}); err != nil {
			return err
		}
		if err := bucket.Put(append(key, validatorSuffix...), []byte{1}); err != nil {
			return err
		}
		return bucket.Put(append(key, balancesSuffix...), []byte{1})
	})
	require.NoError(t, err)

	cache, err := populateStateDiffCacheFromDB(db, 0)
	require.NoError(t, err)
	require.NotNil(t, cache)
	require.Equal(t, uint64(0), cache.getOffset())
	require.NotNil(t, cache.getAnchor(0))
	require.Equal(t, true, cache.levelHasData(0))
	require.Equal(t, false, cache.levelHasData(1))
	require.Equal(t, true, cache.levelHasData(2))
}

func TestStateDiff_PopulateStateDiffCacheFromDB_SingleExponent(t *testing.T) {
	setStateDiffExponents([]int{5})

	db := setupDB(t)
	require.NoError(t, setOffsetInDB(db, 0))
	st, _ := createState(t, 0, version.Phase0)
	require.NoError(t, db.saveStateByDiff(context.Background(), st))

	cache, err := populateStateDiffCacheFromDB(db, 0)
	require.NoError(t, err)
	require.NotNil(t, cache)
	require.Equal(t, 0, len(cache.anchors))
	require.Equal(t, true, cache.levelHasData(0))
}

func TestStateDiff_PopulateStateDiffCacheFromDB_InvalidLevelKey(t *testing.T) {
	setDefaultStateDiffExponents()

	db := setupDB(t)
	st, _ := createState(t, 0, version.Phase0)
	require.NoError(t, setOffsetInDB(db, 0))
	require.NoError(t, db.saveStateByDiff(context.Background(), st))

	require.NoError(t, db.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		key := makeKeyForStateDiffTree(2, 1)
		return bucket.Put(append(key, stateSuffix...), []byte{1})
	}))

	_, err := populateStateDiffCacheFromDB(db, 0)
	require.ErrorIs(t, ErrStateDiffCorrupted, err)
}

func TestStateDiff_PopulateStateDiffCacheFromDB_MissingLevelSuffixes(t *testing.T) {
	setDefaultStateDiffExponents()

	db := setupDB(t)
	st, _ := createState(t, 0, version.Phase0)
	require.NoError(t, setOffsetInDB(db, 0))
	require.NoError(t, db.saveStateByDiff(context.Background(), st))

	require.NoError(t, db.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		key := makeKeyForStateDiffTree(2, math.PowerOf2(16))
		return bucket.Put(append(key, stateSuffix...), []byte{1})
	}))

	_, err := populateStateDiffCacheFromDB(db, 0)
	require.ErrorIs(t, ErrStateDiffCorrupted, err)
}

func TestStateDiff_LatestSlotForLevel(t *testing.T) {
	setDefaultStateDiffExponents()

	db := setupDB(t)
	require.NoError(t, setOffsetInDB(db, 0))

	// Write entries at level 2 with slots whose little-endian byte order
	// differs from numerical order. bbolt sorts keys lexicographically,
	// so the last key in byte order is not necessarily the highest slot.
	//
	//   Slot 1:     LE = 01 00 00 00 00 00 00 00  (lex-last)
	//   Slot 512:   LE = 00 02 00 00 00 00 00 00
	//   Slot 65536: LE = 00 00 01 00 00 00 00 00  (lex-first)
	level := 2
	slots := []uint64{1, 512, 65536}

	require.NoError(t, db.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		for _, slot := range slots {
			key := makeKeyForStateDiffTree(level, slot)
			if err := bucket.Put(append(key, stateSuffix...), []byte{1}); err != nil {
				return err
			}
			if err := bucket.Put(append(key, validatorSuffix...), []byte{1}); err != nil {
				return err
			}
			if err := bucket.Put(append(key, balancesSuffix...), []byte{1}); err != nil {
				return err
			}
		}
		return nil
	}))

	maxSlot, err := latestSlotForLevel(db, level)
	require.NoError(t, err)
	require.Equal(t, uint64(65536), maxSlot)
}

func TestStateDiff_GetBaseAndDiffChainSkipsEmptyLevels(t *testing.T) {
	setDefaultStateDiffExponents()

	db := setupDB(t)
	require.NoError(t, setOffsetInDB(db, 0))
	st, _ := createState(t, 0, version.Phase0)
	require.NoError(t, db.saveFullSnapshot(st))

	cache, err := populateStateDiffCacheFromDB(db, 0)
	require.NoError(t, err)
	cache.levelsWithData[0] = true
	cache.levelsWithData[1] = false
	cache.levelsWithData[2] = true
	db.stateDiffCache = cache

	slot := primitives.Slot(math.PowerOf2(18) + math.PowerOf2(16))
	key := makeKeyForStateDiffTree(2, uint64(slot))
	require.NoError(t, db.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}
		if err := bucket.Put(append(key, stateSuffix...), []byte{1}); err != nil {
			return err
		}
		if err := bucket.Put(append(key, validatorSuffix...), []byte{2}); err != nil {
			return err
		}
		return bucket.Put(append(key, balancesSuffix...), []byte{3})
	}))

	_, diffChain, err := db.getBaseAndDiffChain(0, slot)
	require.NoError(t, err)
	require.Equal(t, 1, len(diffChain))
}

func TestStateDiff_SaveAndReadFullSnapshot(t *testing.T) {
	setDefaultStateDiffExponents()

	for v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			db := setupDB(t)

			st, _ := createState(t, 0, v)

			err := setOffsetInDB(db, 0)
			require.NoError(t, err)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			readSt, err := db.stateByDiff(context.Background(), 0)
			require.NoError(t, err)
			require.NotNil(t, readSt)

			stSSZ, err := st.MarshalSSZ()
			require.NoError(t, err)
			readStSSZ, err := readSt.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, stSSZ, readStSSZ)
		})
	}
}

func TestStateDiff_SaveDiff(t *testing.T) {
	setDefaultStateDiffExponents()

	for v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			db := setupDB(t)

			// Create state with slot 2**21
			slot := primitives.Slot(math.PowerOf2(21))
			st, enc := createState(t, slot, v)

			err := setOffsetInDB(db, uint64(slot))
			require.NoError(t, err)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			err = db.db.View(func(tx *bbolt.Tx) error {
				bucket := tx.Bucket(stateDiffBucket)
				if bucket == nil {
					return bbolt.ErrBucketNotFound
				}
				s := bucket.Get(makeKeyForStateDiffTree(0, uint64(slot)))
				if s == nil {
					return bbolt.ErrIncompatibleValue
				}
				require.DeepSSZEqual(t, enc, s)
				return nil
			})
			require.NoError(t, err)

			// create state with slot 2**18 (+2**21)
			slot = primitives.Slot(math.PowerOf2(18) + math.PowerOf2(21))
			st, _ = createState(t, slot, v)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			key := makeKeyForStateDiffTree(1, uint64(slot))
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
		})
	}
}

func TestStateDiff_SaveAndReadDiff(t *testing.T) {
	setDefaultStateDiffExponents()

	for v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			db := setupDB(t)

			st, _ := createState(t, 0, v)

			err := setOffsetInDB(db, 0)
			require.NoError(t, err)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			slot := primitives.Slot(math.PowerOf2(5))
			st, _ = createState(t, slot, v)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			readSt, err := db.stateByDiff(context.Background(), slot)
			require.NoError(t, err)
			require.NotNil(t, readSt)

			stSSZ, err := st.MarshalSSZ()
			require.NoError(t, err)
			readStSSZ, err := readSt.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, stSSZ, readStSSZ)
		})
	}
}

func TestStateDiff_SaveAndReadDiff_WithRepetitiveAnchorSlots(t *testing.T) {
	globalFlags := flags.GlobalFlags{
		StateDiffExponents: []int{20, 14, 10, 7, 5},
	}
	flags.Init(&globalFlags)

	for v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			db := setupDB(t)

			err := setOffsetInDB(db, 0)

			st, _ := createState(t, 0, v)
			require.NoError(t, err)
			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			slot := primitives.Slot(math.PowerOf2(11))
			st, _ = createState(t, slot, v)
			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			slot = primitives.Slot(math.PowerOf2(11) + math.PowerOf2(5))
			st, _ = createState(t, slot, v)
			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			readSt, err := db.stateByDiff(context.Background(), slot)
			require.NoError(t, err)
			require.NotNil(t, readSt)

			stSSZ, err := st.MarshalSSZ()
			require.NoError(t, err)
			readStSSZ, err := readSt.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, stSSZ, readStSSZ)
		})
	}
}

func TestStateDiff_SaveAndReadDiff_MultipleLevels(t *testing.T) {
	setDefaultStateDiffExponents()

	for v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			db := setupDB(t)

			st, _ := createState(t, 0, v)

			err := setOffsetInDB(db, 0)
			require.NoError(t, err)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			slot := primitives.Slot(math.PowerOf2(11))
			st, _ = createState(t, slot, v)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			readSt, err := db.stateByDiff(context.Background(), slot)
			require.NoError(t, err)
			require.NotNil(t, readSt)

			stSSZ, err := st.MarshalSSZ()
			require.NoError(t, err)
			readStSSZ, err := readSt.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, stSSZ, readStSSZ)

			slot = primitives.Slot(math.PowerOf2(11) + math.PowerOf2(9))
			st, _ = createState(t, slot, v)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			readSt, err = db.stateByDiff(context.Background(), slot)
			require.NoError(t, err)
			require.NotNil(t, readSt)

			stSSZ, err = st.MarshalSSZ()
			require.NoError(t, err)
			readStSSZ, err = readSt.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, stSSZ, readStSSZ)

			slot = primitives.Slot(math.PowerOf2(11) + math.PowerOf2(9) + math.PowerOf2(5))
			st, _ = createState(t, slot, v)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			readSt, err = db.stateByDiff(context.Background(), slot)
			require.NoError(t, err)
			require.NotNil(t, readSt)

			stSSZ, err = st.MarshalSSZ()
			require.NoError(t, err)
			readStSSZ, err = readSt.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, stSSZ, readStSSZ)
		})
	}
}

func TestStateDiff_SaveAndReadDiffForkTransition(t *testing.T) {
	setDefaultStateDiffExponents()

	for v := range version.All()[:len(version.All())-1] {
		t.Run(version.String(v), func(t *testing.T) {
			db := setupDB(t)

			st, _ := createState(t, 0, v)

			err := setOffsetInDB(db, 0)
			require.NoError(t, err)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			slot := primitives.Slot(math.PowerOf2(5))
			st, _ = createState(t, slot, v+1)

			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)

			readSt, err := db.stateByDiff(context.Background(), slot)
			require.NoError(t, err)
			require.NotNil(t, readSt)

			stSSZ, err := st.MarshalSSZ()
			require.NoError(t, err)
			readStSSZ, err := readSt.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, stSSZ, readStSSZ)
		})
	}
}

func TestStateDiff_OffsetCache(t *testing.T) {
	setDefaultStateDiffExponents()

	// test for slot numbers 0 and 1 for every version
	for slotNum := range 2 {
		for v := range version.All() {
			t.Run(fmt.Sprintf("slotNum=%d,%s", slotNum, version.String(v)), func(t *testing.T) {
				db := setupDB(t)

				slot := primitives.Slot(slotNum)
				err := setOffsetInDB(db, uint64(slot))
				require.NoError(t, err)
				st, _ := createState(t, slot, v)
				err = db.saveStateByDiff(context.Background(), st)
				require.NoError(t, err)

				offset := db.stateDiffCache.getOffset()
				require.Equal(t, uint64(slotNum), offset)

				slot2 := primitives.Slot(uint64(slotNum) + math.PowerOf2(uint64(flags.Get().StateDiffExponents[0])))
				st2, _ := createState(t, slot2, v)
				err = db.saveStateByDiff(context.Background(), st2)
				require.NoError(t, err)

				offset = db.stateDiffCache.getOffset()
				require.Equal(t, uint64(slot), offset)
			})
		}
	}
}

func TestStateDiff_AnchorCache(t *testing.T) {
	setDefaultStateDiffExponents()

	for v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			exponents := flags.Get().StateDiffExponents
			localCache := make([]state.ReadOnlyBeaconState, len(exponents)-1)
			db := setupDB(t)
			err := setOffsetInDB(db, 0) // lvl 0
			require.NoError(t, err)

			// at first the cache should be empty
			for i := 0; i < len(flags.Get().StateDiffExponents)-1; i++ {
				anchor := db.stateDiffCache.getAnchor(i)
				require.IsNil(t, anchor)
			}

			// add level 0
			slot := primitives.Slot(0) // offset 0 is already set
			st, _ := createState(t, slot, v)
			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)
			localCache[0] = st

			// level 0 should be the same
			localSSZ, err := localCache[0].MarshalSSZ()
			require.NoError(t, err)
			cachedSSZ, err := db.stateDiffCache.getAnchor(0).MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, localSSZ, cachedSSZ)

			// rest of the cache should be nil
			for i := 1; i < len(exponents)-1; i++ {
				require.IsNil(t, db.stateDiffCache.getAnchor(i))
			}

			// skip last level as it does not get cached
			for i := len(exponents) - 2; i > 0; i-- {
				slot = primitives.Slot(math.PowerOf2(uint64(exponents[i])))
				st, _ := createState(t, slot, v)
				err = db.saveStateByDiff(context.Background(), st)
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
					require.DeepSSZEqual(t, localSSZ, anchorSSZ)
				}
			}

			// moving to a new tree should invalidate the cache except for level 0
			twoTo21 := math.PowerOf2(21)
			slot = primitives.Slot(twoTo21)
			st, _ = createState(t, slot, v)
			err = db.saveStateByDiff(context.Background(), st)
			require.NoError(t, err)
			localCache = make([]state.ReadOnlyBeaconState, len(exponents)-1)
			localCache[0] = st

			// level 0 should be the same
			localSSZ, err = localCache[0].MarshalSSZ()
			require.NoError(t, err)
			cachedSSZ, err = db.stateDiffCache.getAnchor(0).MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, localSSZ, cachedSSZ)

			// rest of the cache should be nil
			for i := 1; i < len(exponents)-1; i++ {
				require.IsNil(t, db.stateDiffCache.getAnchor(i))
			}
		})
	}
}

func TestStateDiff_EncodingAndDecoding(t *testing.T) {
	for v := range version.All() {
		t.Run(version.String(v), func(t *testing.T) {
			st, enc := createState(t, 0, v) // this has addKey called inside
			stDecoded, err := decodeStateSnapshot(enc)
			require.NoError(t, err)
			st1ssz, err := st.MarshalSSZ()
			require.NoError(t, err)
			st2ssz, err := stDecoded.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, st1ssz, st2ssz)
		})
	}
}

func createState(t *testing.T, slot primitives.Slot, v int) (state.ReadOnlyBeaconState, []byte) {
	p := params.BeaconConfig()
	var st state.BeaconState
	var err error
	switch v {
	case version.Phase0:
		st, err = util.NewBeaconState()
		require.NoError(t, err)
		err = st.SetFork(&ethpb.Fork{
			PreviousVersion: p.GenesisForkVersion,
			CurrentVersion:  p.GenesisForkVersion,
			Epoch:           0,
		})
		require.NoError(t, err)
	case version.Altair:
		st, err = util.NewBeaconStateAltair()
		require.NoError(t, err)
		err = st.SetFork(&ethpb.Fork{
			PreviousVersion: p.GenesisForkVersion,
			CurrentVersion:  p.AltairForkVersion,
			Epoch:           p.AltairForkEpoch,
		})
		require.NoError(t, err)
	case version.Bellatrix:
		st, err = util.NewBeaconStateBellatrix()
		require.NoError(t, err)
		err = st.SetFork(&ethpb.Fork{
			PreviousVersion: p.AltairForkVersion,
			CurrentVersion:  p.BellatrixForkVersion,
			Epoch:           p.BellatrixForkEpoch,
		})
		require.NoError(t, err)
	case version.Capella:
		st, err = util.NewBeaconStateCapella()
		require.NoError(t, err)
		err = st.SetFork(&ethpb.Fork{
			PreviousVersion: p.BellatrixForkVersion,
			CurrentVersion:  p.CapellaForkVersion,
			Epoch:           p.CapellaForkEpoch,
		})
		require.NoError(t, err)
	case version.Deneb:
		st, err = util.NewBeaconStateDeneb()
		require.NoError(t, err)
		err = st.SetFork(&ethpb.Fork{
			PreviousVersion: p.CapellaForkVersion,
			CurrentVersion:  p.DenebForkVersion,
			Epoch:           p.DenebForkEpoch,
		})
		require.NoError(t, err)
	case version.Electra:
		st, err = util.NewBeaconStateElectra()
		require.NoError(t, err)
		err = st.SetFork(&ethpb.Fork{
			PreviousVersion: p.DenebForkVersion,
			CurrentVersion:  p.ElectraForkVersion,
			Epoch:           p.ElectraForkEpoch,
		})
		require.NoError(t, err)
	case version.Fulu:
		st, err = util.NewBeaconStateFulu()
		require.NoError(t, err)
		err = st.SetFork(&ethpb.Fork{
			PreviousVersion: p.ElectraForkVersion,
			CurrentVersion:  p.FuluForkVersion,
			Epoch:           p.FuluForkEpoch,
		})
		require.NoError(t, err)
	default:
		t.Fatalf("unsupported version: %d", v)
	}

	err = st.SetSlot(slot)
	require.NoError(t, err)
	slashings := make([]uint64, 8192)
	slashings[0] = uint64(rand.Intn(10))
	err = st.SetSlashings(slashings)
	require.NoError(t, err)
	stssz, err := st.MarshalSSZ()
	require.NoError(t, err)
	enc, err := addKey(v, stssz)
	require.NoError(t, err)
	return st, enc
}

func setOffsetInDB(s *Store, offset uint64) error {
	err := s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bbolt.ErrBucketNotFound
		}

		offsetBytes := bucket.Get(offsetKey)
		if offsetBytes != nil {
			return fmt.Errorf("offset already set to %d", binary.LittleEndian.Uint64(offsetBytes))
		}

		offsetBytes = make([]byte, 8)
		binary.LittleEndian.PutUint64(offsetBytes, offset)
		if err := bucket.Put(offsetKey, offsetBytes); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	sdCache, err := newStateDiffCache(s)
	if err != nil {
		return err
	}
	s.stateDiffCache = sdCache
	return nil
}

func setDefaultStateDiffExponents() {
	setStateDiffExponents([]int{21, 18, 16, 13, 11, 9, 5})
}

func setStateDiffExponents(exponents []int) {
	globalFlags := flags.GlobalFlags{StateDiffExponents: exponents}
	flags.Init(&globalFlags)
}
