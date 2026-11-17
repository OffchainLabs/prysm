package lookup

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	pathDiffTree         = "diff_tree"
	pathReplay           = "replay"
	missDisabled         = "disabled"
	missNotBoundary      = "not_boundary"
	missSlotBeforeOffset = "slot_before_offset"
	missNotFound         = "not_found"
	missError            = "error"
	missNilState         = "nil_state"
)

var (
	stateBySlotMilliseconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "state_by_slot_milliseconds",
			Help:    "Milliseconds to produce a state by slot via the state-diff tree or block replay, excluding availability checks and failed tree attempts.",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 120000, 300000},
		},
		[]string{"path"},
	)
	stateFromDiffTreeMiss = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "state_from_diff_tree_miss_total",
			Help: "Slot lookups that could not be served from the state-diff tree.",
		},
		[]string{"reason"},
	)
)

func init() {
	for _, reason := range []string{missDisabled, missNotBoundary, missSlotBeforeOffset, missNotFound, missError, missNilState} {
		stateFromDiffTreeMiss.WithLabelValues(reason)
	}
}
