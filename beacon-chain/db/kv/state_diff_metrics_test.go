package kv

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	bolt "go.etcd.io/bbolt"
)

func metricValue(t *testing.T, metric any) *dto.Metric {
	t.Helper()
	m := new(dto.Metric)
	require.NoError(t, metric.(prometheus.Metric).Write(m))
	return m
}

func histogramCount(t *testing.T, metric any) uint64 {
	t.Helper()
	return metricValue(t, metric).GetHistogram().GetSampleCount()
}

func counterValue(t *testing.T, metric prometheus.Metric) float64 {
	t.Helper()
	return metricValue(t, metric).GetCounter().GetValue()
}

func gaugeValue(t *testing.T, metric prometheus.Metric) float64 {
	t.Helper()
	return metricValue(t, metric).GetGauge().GetValue()
}

func metricTestExponents(t *testing.T) {
	t.Helper()
	previous := *flags.Get()
	t.Cleanup(func() { flags.Init(&previous) })
	setStateDiffExponents([]int{7, 6, 5})
}

func TestStateDiffMetricErrorReasons(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"frontier", ErrAboveArchiveFrontier, "above_archive_frontier"},
		{"offset", ErrSlotBeforeOffset, "slot_before_offset"},
		{"corruption", ErrStateDiffCorrupted, "corrupted"},
		{"state missing", ErrNotFoundState, "not_found"},
		{"snapshot missing", errSnapshotNotFound, "not_found"},
		{"cancel", context.Canceled, "canceled"},
		{"deadline", context.DeadlineExceeded, "canceled"},
		{"other", errors.New("other"), "other"},
		{"wrapped", fmt.Errorf("wrapped: %w", ErrStateDiffCorrupted), "corrupted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, stateDiffErrorReason(tc.err))
		})
	}
	before := counterValue(t, stateDiffErrors.WithLabelValues("save", "other"))
	recordStateDiffError("save", nil)
	require.Equal(t, before, counterValue(t, stateDiffErrors.WithLabelValues("save", "other")))
}

func TestStateDiffMetricBuckets(t *testing.T) {
	for _, metric := range []prometheus.Metric{
		stateDiffGetAnchorStateCacheHitReadTime,
		stateDiffGetAnchorStateCacheMissTime,
		stateDiffGetAnchorStateDBReadTime,
		stateReadingTime,
		stateSavingTime,
	} {
		buckets := metricValue(t, metric).GetHistogram().GetBucket()
		require.Equal(t, len(stateDiffMillisecondsBuckets), len(buckets))
		for i, bucket := range buckets {
			require.Equal(t, stateDiffMillisecondsBuckets[i], bucket.GetUpperBound())
		}
	}
	require.Equal(t, float64(16384), stateDiffBytesBuckets[0])
	require.Equal(t, float64(1<<30), stateDiffBytesBuckets[len(stateDiffBytesBuckets)-1])
}

func TestStateDiffMetricWritesAndOverwrites(t *testing.T) {
	metricTestExponents(t)
	db := setupDB(t)
	require.NoError(t, setOffsetInDB(db, 0))

	for _, tc := range []struct {
		slot  primitives.Slot
		level int
		parts map[string]string
		phase []string
	}{
		{0, 0, map[string]string{"snapshot": ""}, []string{"encode", "write", "cache"}},
		{64, 1, map[string]string{"state": stateSuffix, "validators": validatorSuffix, "balances": balancesSuffix}, []string{"anchor", "diff", "write", "cache"}},
	} {
		t.Run(fmt.Sprintf("level_%d", tc.level), func(t *testing.T) {
			st, _ := createState(t, tc.slot, version.Phase0)
			label := levelLabel(tc.level)
			beforeSave := histogramCount(t, stateDiffSaveTime.WithLabelValues(label))
			beforeBytes := histogramCount(t, stateDiffSaveBytes.WithLabelValues(label))
			beforeByteSum := metricValue(t, stateDiffSaveBytes.WithLabelValues(label)).GetHistogram().GetSampleSum()
			beforePhase := make(map[string]uint64)
			beforeComponents := make(map[string]float64)
			for _, phase := range tc.phase {
				beforePhase[phase] = histogramCount(t, stateDiffSavePhaseTime.WithLabelValues(label, phase))
			}
			for component := range tc.parts {
				beforeComponents[component] = counterValue(t, stateDiffSavedBytes.WithLabelValues(label, component))
			}
			require.NoError(t, db.saveStateByDiff(t.Context(), st))
			require.Equal(t, beforeSave+1, histogramCount(t, stateDiffSaveTime.WithLabelValues(label)))
			require.Equal(t, beforeBytes+1, histogramCount(t, stateDiffSaveBytes.WithLabelValues(label)))
			for _, phase := range tc.phase {
				require.Equal(t, beforePhase[phase]+1, histogramCount(t, stateDiffSavePhaseTime.WithLabelValues(label, phase)))
			}
			total := 0
			require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
				bucket := tx.Bucket(stateDiffBucket)
				for component, suffix := range tc.parts {
					key := append(makeKeyForStateDiffTree(tc.level, uint64(tc.slot)), suffix...)
					size := len(bucket.Get(key))
					require.Equal(t, true, size > 0)
					total += size
					require.Equal(t, beforeComponents[component]+float64(size), counterValue(t, stateDiffSavedBytes.WithLabelValues(label, component)))
				}
				return nil
			}))
			require.Equal(t, beforeByteSum+float64(total), metricValue(t, stateDiffSaveBytes.WithLabelValues(label)).GetHistogram().GetSampleSum())
			require.Equal(t, float64(tc.slot), gaugeValue(t, stateDiffLastSavedSlot.WithLabelValues(label)))

			beforeSame := counterValue(t, stateDiffOverwrites.WithLabelValues(label, "true"))
			require.NoError(t, db.saveStateByDiff(t.Context(), st))
			require.Equal(t, beforeSame+1, counterValue(t, stateDiffOverwrites.WithLabelValues(label, "true")))
			require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
				key := append(makeKeyForStateDiffTree(tc.level, uint64(tc.slot)), tc.parts[firstComponent(tc.level)]...)
				return tx.Bucket(stateDiffBucket).Put(key, []byte("tampered"))
			}))
			beforeChanged := counterValue(t, stateDiffOverwrites.WithLabelValues(label, "false"))
			require.NoError(t, db.saveStateByDiff(t.Context(), st))
			require.Equal(t, beforeChanged+1, counterValue(t, stateDiffOverwrites.WithLabelValues(label, "false")))
		})
	}
}

func firstComponent(level int) string {
	if level == 0 {
		return "snapshot"
	}
	return "state"
}

func TestStateDiffMetricReadsAndErrors(t *testing.T) {
	metricTestExponents(t)
	db := setupDB(t)
	require.NoError(t, setOffsetInDB(db, 0))
	for _, slot := range []primitives.Slot{0, 64, 96} {
		st, _ := createState(t, slot, version.Phase0)
		require.NoError(t, db.saveStateByDiff(t.Context(), st))
	}
	snapshotBefore := counterValue(t, stateDiffReadBase.WithLabelValues("snapshot_cache"))
	_, err := db.stateByDiff(t.Context(), 0)
	require.NoError(t, err)
	require.Equal(t, snapshotBefore+1, counterValue(t, stateDiffReadBase.WithLabelValues("snapshot_cache")))
	readBefore := histogramCount(t, stateDiffReadTime.WithLabelValues("2"))
	anchorBefore := counterValue(t, stateDiffReadBase.WithLabelValues("anchor_cache"))
	diffsBefore := counterValue(t, stateDiffReadDiffsApplied.WithLabelValues("2"))
	applyBefore := histogramCount(t, stateDiffApplyTime)
	_, err = db.stateByDiff(t.Context(), 96)
	require.NoError(t, err)
	require.Equal(t, readBefore+1, histogramCount(t, stateDiffReadTime.WithLabelValues("2")))
	require.Equal(t, anchorBefore+1, counterValue(t, stateDiffReadBase.WithLabelValues("anchor_cache")))
	require.Equal(t, diffsBefore+1, counterValue(t, stateDiffReadDiffsApplied.WithLabelValues("2")))
	require.Equal(t, applyBefore+1, histogramCount(t, stateDiffApplyTime))

	db.stateDiffCache.clearAnchors()
	loadBefore := histogramCount(t, stateDiffSnapshotLoadTime)
	dbBefore := counterValue(t, stateDiffReadBase.WithLabelValues("snapshot_db"))
	diffsBefore = counterValue(t, stateDiffReadDiffsApplied.WithLabelValues("2"))
	applyBefore = histogramCount(t, stateDiffApplyTime)
	_, err = db.stateByDiff(t.Context(), 96)
	require.NoError(t, err)
	require.Equal(t, loadBefore+1, histogramCount(t, stateDiffSnapshotLoadTime))
	require.Equal(t, dbBefore+1, counterValue(t, stateDiffReadBase.WithLabelValues("snapshot_db")))
	require.Equal(t, diffsBefore+2, counterValue(t, stateDiffReadDiffsApplied.WithLabelValues("2")))
	require.Equal(t, applyBefore+2, histogramCount(t, stateDiffApplyTime))

	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		key := append(makeKeyForStateDiffTree(2, 96), stateSuffix...)
		return tx.Bucket(stateDiffBucket).Delete(key)
	}))
	missingBefore := counterValue(t, stateDiffErrors.WithLabelValues("read", "not_found"))
	_, err = db.stateByDiff(t.Context(), 96)
	require.ErrorIs(t, err, ErrNotFoundState)
	require.Equal(t, missingBefore+1, counterValue(t, stateDiffErrors.WithLabelValues("read", "not_found")))

	db.stateDiffCache.setOffset(128)
	offsetBefore := counterValue(t, stateDiffErrors.WithLabelValues("read", "slot_before_offset"))
	_, err = db.stateByDiff(t.Context(), 96)
	require.ErrorIs(t, err, ErrSlotBeforeOffset)
	require.Equal(t, offsetBefore+1, counterValue(t, stateDiffErrors.WithLabelValues("read", "slot_before_offset")))
	require.Equal(t, float64(128), gaugeValue(t, stateDiffTreeOffset))
}

func TestStateDiffMetricPublicReadMissingHistory(t *testing.T) {
	metricTestExponents(t)
	db := setupDB(t)
	require.NoError(t, setOffsetInDB(db, 0))

	t.Run("missing snapshot", func(t *testing.T) {
		before := counterValue(t, stateDiffErrors.WithLabelValues("read", "not_found"))
		_, err := db.StateBySlotFromDiffTree(t.Context(), 0)
		require.ErrorIs(t, err, ErrNotFound)
		require.ErrorIs(t, err, ErrNotFoundState)
		require.ErrorIs(t, err, errSnapshotNotFound)
		require.Equal(t, before+1, counterValue(t, stateDiffErrors.WithLabelValues("read", "not_found")))
	})

	t.Run("missing diff", func(t *testing.T) {
		st, _ := createState(t, 0, version.Phase0)
		require.NoError(t, db.saveFullSnapshot(st))
		_, err := db.StateBySlotFromDiffTree(t.Context(), 32)
		require.ErrorIs(t, err, ErrNotFoundState)
	})
}

func TestStateDiffMetricAnchorMemo(t *testing.T) {
	metricTestExponents(t)
	db := setupDB(t)
	require.NoError(t, setOffsetInDB(db, 0))
	st, _ := createState(t, 0, version.Phase0)
	require.NoError(t, db.saveFullSnapshot(st))
	cache := db.stateDiffCache
	require.Equal(t, float64(0), gaugeValue(t, stateDiffAnchorMemoEntries))

	missBefore := counterValue(t, stateDiffAnchorMemo.WithLabelValues("miss"))
	decodeBefore := histogramCount(t, stateDiffAnchorDecodeTime)
	require.NotNil(t, cache.getAnchor(0))
	require.Equal(t, missBefore+1, counterValue(t, stateDiffAnchorMemo.WithLabelValues("miss")))
	require.Equal(t, decodeBefore+1, histogramCount(t, stateDiffAnchorDecodeTime))
	require.Equal(t, float64(1), gaugeValue(t, stateDiffAnchorMemoEntries))
	hitBefore := counterValue(t, stateDiffAnchorMemo.WithLabelValues("hit"))
	require.NotNil(t, cache.getAnchor(0))
	require.Equal(t, hitBefore+1, counterValue(t, stateDiffAnchorMemo.WithLabelValues("hit")))
	cache.Lock()
	cache.memoDrop(0)
	cache.Unlock()
	require.Equal(t, float64(0), gaugeValue(t, stateDiffAnchorMemoEntries))
	require.NotNil(t, cache.getAnchor(0))
	cache.clearAnchors()
	require.Equal(t, float64(0), gaugeValue(t, stateDiffAnchorMemoEntries))
}

func TestStateDiffMetricStartup(t *testing.T) {
	metricTestExponents(t)
	reset := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer reset()
	dir := t.TempDir()
	db, err := NewKVStore(t.Context(), dir)
	require.NoError(t, err)
	st, _ := createState(t, 0, version.Phase0)
	require.NoError(t, db.initializeStateDiff(0, st))
	for _, slot := range []primitives.Slot{64, 128} {
		st, _ := createState(t, slot, version.Phase0)
		require.NoError(t, db.saveStateByDiff(t.Context(), st))
	}
	require.NoError(t, db.Close())
	reopened, err := NewKVStore(t.Context(), dir)
	require.NoError(t, err)
	defer func() { require.NoError(t, reopened.Close()) }()
	require.Equal(t, true, gaugeValue(t, stateDiffStartupTime.WithLabelValues("populate_cache")) > 0)
	require.Equal(t, true, gaugeValue(t, stateDiffStartupTime.WithLabelValues("validate_cache")) > 0)
	require.Equal(t, float64(0), gaugeValue(t, stateDiffTreeOffset))
	require.Equal(t, float64(128), gaugeValue(t, stateDiffLastSavedSlot.WithLabelValues("0")))
	require.Equal(t, float64(64), gaugeValue(t, stateDiffLastSavedSlot.WithLabelValues("1")))
}

func TestStateDiffMetricPruning(t *testing.T) {
	previous := *flags.Get()
	t.Cleanup(func() { flags.Init(&previous) })
	db := setupPrunableStateDiffTree(t, 384)
	beforeKeys := counterValue(t, stateDiffPrunedKeys)
	beforeReanchors := counterValue(t, stateDiffReanchors)
	deleted := drainStateDiffPruning(t, db, 320, 2)
	require.Equal(t, beforeKeys+float64(deleted), counterValue(t, stateDiffPrunedKeys))
	require.Equal(t, beforeReanchors+1, counterValue(t, stateDiffReanchors))
	require.Equal(t, float64(256), gaugeValue(t, stateDiffTreeOffset))
}

func TestStateDiffMetricHotSnapshots(t *testing.T) {
	db := setupDB(t)
	st, _ := createState(t, 0, version.Phase0)
	beforeHot := histogramCount(t, stateDiffHotSnapshotTime.WithLabelValues("save_hot"))
	beforeResume := histogramCount(t, stateDiffHotSnapshotTime.WithLabelValues("save_archive_resume"))
	beforeClear := histogramCount(t, stateDiffHotSnapshotTime.WithLabelValues("clear"))
	require.NoError(t, db.SaveHotStateSnapshot(t.Context(), st, [32]byte{'A'}))
	require.NoError(t, db.SaveArchiveResumeSnapshot(t.Context(), st, [32]byte{'B'}))
	require.NoError(t, db.ClearHotStateSnapshots(t.Context()))
	require.Equal(t, beforeHot+1, histogramCount(t, stateDiffHotSnapshotTime.WithLabelValues("save_hot")))
	require.Equal(t, beforeResume+1, histogramCount(t, stateDiffHotSnapshotTime.WithLabelValues("save_archive_resume")))
	require.Equal(t, beforeClear+1, histogramCount(t, stateDiffHotSnapshotTime.WithLabelValues("clear")))
}
