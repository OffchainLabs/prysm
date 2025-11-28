package kv

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// EarliestAvailableSlotMetric tracks the earliest available slot in the database
	EarliestAvailableSlotMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "custody_earliest_available_slot_db",
		Help: "The earliest available slot tracked by the database for custody purposes",
	})
)