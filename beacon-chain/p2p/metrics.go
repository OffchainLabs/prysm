package p2p

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// EarliestAvailableSlotMetric tracks the earliest available slot in the p2p service
	EarliestAvailableSlotMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "custody_earliest_available_slot_p2p",
		Help: "The earliest available slot tracked by the p2p service for custody purposes",
	})
)