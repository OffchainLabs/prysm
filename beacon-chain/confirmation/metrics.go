package confirmation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
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
)
