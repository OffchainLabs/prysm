package archive

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	archiveOriginSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "archive_origin_slot",
		Help: "Slot of the archive origin state.",
	})
	regeneratedThroughSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "archive_regenerated_through_slot",
		Help: "Highest state-diff tree boundary slot the archive walk has written.",
	})
	regenTargetSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "archive_regeneration_target_slot",
		Help: "Slot the archive walk is currently trying to reach.",
	})
	boundariesSaved = promauto.NewCounter(prometheus.CounterOpts{
		Name: "archive_boundary_states_saved_total",
		Help: "Number of boundary states the archive walk has written to the state-diff tree.",
	})
	blocksReplayed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "archive_blocks_replayed_total",
		Help: "Number of blocks the archive walk has replayed.",
	})
	regenerationPhase = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "archive_regeneration_phase",
		Help: "Current archive regeneration phase (1 = active).",
	}, []string{"phase"})
	boundaryMilliseconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "archive_boundary_milliseconds",
		Help:    "Milliseconds per walked boundary: block replay vs persisting.",
		Buckets: archiveBoundaryMillisecondsBuckets,
	}, []string{"phase"})
	roundFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "archive_round_failures_total",
		Help: "Failed archive regeneration rounds.",
	})
	stateRootChecks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "archive_state_root_checks_total",
		Help: "State root verifications performed by the archive walk.",
	}, []string{"result"})
)

var archiveBoundaryMillisecondsBuckets = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

func setPhase(phase string) {
	for _, p := range []string{"waiting_for_backfill", "walking", "complete"} {
		value := 0.0
		if p == phase {
			value = 1
		}
		regenerationPhase.WithLabelValues(p).Set(value)
	}
}
