package kv

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	bolt "go.etcd.io/bbolt"
)

var (
	stateDiffMillisecondsBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 120000}
	stateDiffBytesBuckets        = prometheus.ExponentialBuckets(16384, 2, 17)

	stateDiffSaveTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "state_diff_save_milliseconds",
		Help:    "Milliseconds to persist one state-diff tree entry, by level.",
		Buckets: stateDiffMillisecondsBuckets,
	}, []string{"level"})
	stateDiffSavePhaseTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "state_diff_save_phase_milliseconds",
		Help:    "Milliseconds spent in each phase of a state-diff tree write.",
		Buckets: stateDiffMillisecondsBuckets,
	}, []string{"level", "phase"})
	stateDiffSaveBytes = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "state_diff_save_bytes",
		Help:    "Bytes written for one state-diff tree entry.",
		Buckets: stateDiffBytesBuckets,
	}, []string{"level"})
	stateDiffSavedBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "state_diff_saved_bytes_total",
		Help: "Bytes written to the state-diff tree, by level and component.",
	}, []string{"level", "component"})
	stateDiffOverwrites = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "state_diff_overwrites_total",
		Help: "State-diff tree entries written over an existing key; same=false means the bytes changed.",
	}, []string{"level", "same"})
	stateDiffLastSavedSlot = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "state_diff_last_saved_slot",
		Help: "Slot of the most recent state-diff tree write at each level.",
	}, []string{"level"})
	stateDiffTreeOffset = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "state_diff_tree_offset_slot",
		Help: "Current offset (anchor slot) of the state-diff tree.",
	})
	stateDiffReadTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "state_diff_read_milliseconds",
		Help:    "Milliseconds to reconstruct a state from the state-diff tree on successful reads, including anchor loads during saves and startup validation.",
		Buckets: stateDiffMillisecondsBuckets,
	}, []string{"level"})
	stateDiffReadDiffsApplied = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "state_diff_read_diffs_applied_total",
		Help: "Diffs successfully applied while reading states from the state-diff tree.",
	}, []string{"level"})
	stateDiffReadBase = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "state_diff_read_base_total",
		Help: "Base source for state-diff reads, including internal anchor and startup reads; counted when obtained before diffs are applied.",
	}, []string{"source"})
	stateDiffSnapshotLoadTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "state_diff_snapshot_load_milliseconds",
		Help:    "Milliseconds to read and decode a level-0 snapshot from the DB, including startup cache population.",
		Buckets: stateDiffMillisecondsBuckets,
	})
	stateDiffApplyTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "state_diff_apply_milliseconds",
		Help:    "Milliseconds for one hdiff.ApplyDiff during a state-diff tree read.",
		Buckets: stateDiffMillisecondsBuckets,
	})
	stateDiffHTRVerifyTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "state_diff_htr_verify_milliseconds",
		Help:    "Milliseconds to hash a state-diff tree state when verifying it against its block.",
		Buckets: stateDiffMillisecondsBuckets,
	})
	stateDiffAnchorMemo = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "state_diff_anchor_memo_total",
		Help: "Decoded-anchor memo lookups.",
	}, []string{"result"})
	stateDiffAnchorDecodeTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "state_diff_anchor_decode_milliseconds",
		Help:    "Milliseconds to decompress and decode a cached anchor.",
		Buckets: stateDiffMillisecondsBuckets,
	})
	stateDiffAnchorMemoEntries = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "state_diff_anchor_memo_entries",
		Help: "Decoded anchor states held in the memo.",
	})
	stateDiffStartupTime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "state_diff_startup_milliseconds",
		Help: "Milliseconds spent rebuilding and validating the state-diff cache at startup.",
	}, []string{"phase"})
	stateDiffErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "state_diff_errors_total",
		Help: "Failed state-diff tree reads and writes, by reason.",
	}, []string{"op", "reason"})
	stateDiffPrunedKeys = promauto.NewCounter(prometheus.CounterOpts{
		Name: "state_diff_pruned_keys_total",
		Help: "State-diff keys deleted by pruning.",
	})
	stateDiffReanchors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "state_diff_reanchors_total",
		Help: "Times the pruned state-diff tree was re-anchored.",
	})
	stateDiffHotSnapshotTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "state_diff_hot_snapshot_milliseconds",
		Help:    "Milliseconds for hot-state snapshot operations.",
		Buckets: stateDiffMillisecondsBuckets,
	}, []string{"op"})
)

func init() {
	for _, op := range []string{"save", "read"} {
		for _, reason := range []string{"above_archive_frontier", "slot_before_offset", "corrupted", "not_found", "canceled", "other"} {
			stateDiffErrors.WithLabelValues(op, reason)
		}
	}
	for _, result := range []string{"hit", "miss"} {
		stateDiffAnchorMemo.WithLabelValues(result)
	}
}

func levelLabel(level int) string { return strconv.Itoa(level) }

func msSince(start time.Time) float64 { return float64(time.Since(start)) / float64(time.Millisecond) }

func recordStateDiffError(op string, err error) {
	if err != nil {
		stateDiffErrors.WithLabelValues(op, stateDiffErrorReason(err)).Inc()
	}
}

func stateDiffErrorReason(err error) string {
	switch {
	case errors.Is(err, ErrAboveArchiveFrontier):
		return "above_archive_frontier"
	case errors.Is(err, ErrSlotBeforeOffset):
		return "slot_before_offset"
	case errors.Is(err, ErrStateDiffCorrupted), errors.Is(err, snappy.ErrCorrupt):
		return "corrupted"
	case errors.Is(err, ErrNotFoundState), errors.Is(err, errSnapshotNotFound):
		return "not_found"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled"
	default:
		return "other"
	}
}

func setOffsetGauge(offset uint64) { stateDiffTreeOffset.Set(float64(offset)) }

type treeEntry struct {
	key, value []byte
}

// putTreeEntry checks previous values inside the transaction, then writes the entry.
func putTreeEntry(bucket *bolt.Bucket, entries ...treeEntry) (overwrite, same bool, err error) {
	same = true
	for _, entry := range entries {
		old := bucket.Get(entry.key)
		if old != nil {
			overwrite = true
		}
		if !bytes.Equal(old, entry.value) {
			same = false
		}
	}
	for _, entry := range entries {
		if err := bucket.Put(entry.key, entry.value); err != nil {
			return false, false, err
		}
	}
	return overwrite, same && overwrite, nil
}

func recordTreeWrite(level int, slot uint64, components map[string]int, overwrite, same bool) int {
	label := levelLabel(level)
	total := 0
	for component, size := range components {
		total += size
		stateDiffSavedBytes.WithLabelValues(label, component).Add(float64(size))
	}
	stateDiffSaveBytes.WithLabelValues(label).Observe(float64(total))
	stateDiffLastSavedSlot.WithLabelValues(label).Set(float64(slot))
	if overwrite {
		stateDiffOverwrites.WithLabelValues(label, strconv.FormatBool(same)).Inc()
	}
	return total
}
