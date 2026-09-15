package confirmation

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// Registered in New so nodes without the feature flag don't expose FCR metrics.
var registerMetricsOnce sync.Once

var (
	// Standardized metrics for fast confirmation.
	fastConfirmationSlot = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_fast_confirmation_slot",
		Help: "Slot of the most recent confirmed block",
	})
	fastConfirmationReorgsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "beacon_fast_confirmation_reorgs_total",
		Help: "Total number of confirmed block reorganizations",
	})
	fastConfirmationFallbacksTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "beacon_fast_confirmation_fallbacks_total",
		Help: "Total number of fallbacks to finality",
	})
	fastConfirmationRestartsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "beacon_fast_confirmation_restarts_total",
		Help: "Total number of restarts from a safe unrealized justified block",
	})

	// Prysm-specific metrics for fast confirmation.
	fastConfirmationDistance = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_fast_confirmation_distance_slots",
		Help: "Slots between the current slot and the most recent confirmed block",
	})
	fastConfirmationDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "beacon_fast_confirmation_duration_milliseconds",
		Help:    "Time to run on_fast_confirmation per slot, by code path",
		Buckets: []float64{1, 5, 10, 50, 100, 250, 500, 1000, 2000},
	}, []string{"path"})
)

func registerMetrics() {
	registerMetricsOnce.Do(func() {
		prometheus.MustRegister(
			fastConfirmationSlot,
			fastConfirmationReorgsTotal,
			fastConfirmationFallbacksTotal,
			fastConfirmationRestartsTotal,
			fastConfirmationDistance,
			fastConfirmationDuration,
		)
	})
}
