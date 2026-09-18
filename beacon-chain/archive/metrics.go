package archive

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
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
)
