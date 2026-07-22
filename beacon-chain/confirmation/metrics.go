package confirmation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Standardized metrics for fast confirmation.
	fastConfirmationSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_fast_confirmation_slot",
		Help: "Slot of the most recent confirmed block",
	})
	fastConfirmationReorgsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_fast_confirmation_reorgs_total",
		Help: "Total number of confirmed block reorganizations",
	})
	fastConfirmationFallbacksTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_fast_confirmation_fallbacks_total",
		Help: "Total number of fallbacks to finality",
	})
	fastConfirmationRestartsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_fast_confirmation_restarts_total",
		Help: "Total number of restarts from a safe unrealized justified block",
	})

	// Prysm-specific metrics for fast confirmation.
	fastConfirmationDistance = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "beacon_fast_confirmation_distance_slots",
		Help: "Slots between the current slot and the most recent confirmed block",
	})
	fastConfirmationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "beacon_fast_confirmation_duration_seconds",
		Help:    "Time to run on_fast_confirmation per slot, by code path",
		Buckets: []float64{.001, .005, .01, .05, .1, .25, .5, 1, 2},
	}, []string{"path"})
)
