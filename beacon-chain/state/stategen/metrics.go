package stategen

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var stategenMillisecondsBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 120000, 300000}
var migrateToColdMillisecondsBuckets = []float64{10, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 300000, 900000, 1800000, 3600000, 7200000, 14400000}
var replayBlocksCountBuckets = []float64{0, 1, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 32768, 131072, 524288}

var (
	migrateToColdMilliseconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "migrate_to_cold_milliseconds",
		Help:    "Milliseconds for one MigrateToCold call after acquiring the migration lock.",
		Buckets: migrateToColdMillisecondsBuckets,
	}, []string{"mode"})
	migrateToColdFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "migrate_to_cold_failures_total",
		Help: "MigrateToCold calls that returned an error.",
	}, []string{"mode"})
	migrateToColdSkipped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "migrate_to_cold_skipped_total",
		Help: "MigrateToCold calls skipped while archive regeneration is pending.",
	})
	migrateToColdStatesSaved = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "migrate_to_cold_states_saved_total",
		Help: "States saved to cold storage by MigrateToCold.",
	}, []string{"mode"})
	migrateToColdStateSource = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "migrate_to_cold_state_source_total",
		Help: "Where hdiff cold migration got each boundary state.",
	}, []string{"source"})
	migrateToColdProcessSlots = promauto.NewCounter(prometheus.CounterOpts{
		Name: "migrate_to_cold_process_slots_total",
		Help: "Boundary states advanced through empty slots before saving.",
	})
	migrateToColdLagSlots = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "migrate_to_cold_lag_slots",
		Help: "Finalized slot minus the cold-migration cursor at the start of the last migration (negative means the cursor regressed).",
	})
	migrateToColdMigratedSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "migrate_to_cold_migrated_slot",
		Help: "Current cold-migration cursor slot.",
	})
	migrateToColdQueued = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "migrate_to_cold_queued",
		Help: "MigrateToCold calls waiting for the migration lock.",
	})
	stategenArchivePending = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "stategen_archive_pending",
		Help: "Whether archive state regeneration is pending.",
	})
	stategenLoadStateMilliseconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "stategen_load_state_milliseconds",
		Help:    "Milliseconds to load an uncached state by root, by source.",
		Buckets: stategenMillisecondsBuckets,
	}, []string{"source"})
	replayBlockCount = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "replay_blocks_count",
			Help:    "The number of blocks to replay to generate a state",
			Buckets: replayBlocksCountBuckets,
		},
	)
	replayBlocksSummary = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "replay_blocks_milliseconds",
			Help:    "Time it took to replay blocks",
			Buckets: stategenMillisecondsBuckets,
		},
	)
	replayToSlotSummary = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "replay_to_slot_milliseconds",
			Help:    "Time it took to replay to slot",
			Buckets: stategenMillisecondsBuckets,
		},
	)
	saveStateToColdSummary = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "save_state_to_cold_milliseconds",
			Help:    "Time it took to save a state to the DB during migration to cold",
			Buckets: stategenMillisecondsBuckets,
		},
	)
)

func observeMigrateToCold(mode string, start time.Time, err error) {
	if err != nil {
		migrateToColdFailures.WithLabelValues(mode).Inc()
		return
	}
	migrateToColdMilliseconds.WithLabelValues(mode).Observe(float64(time.Since(start).Milliseconds()))
}

func boolToFloat(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
