// Package custody provides common custody-related metrics
package custody

import (
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// EarliestAvailableSlotP2P tracks the earliest available slot in the p2p service
	EarliestAvailableSlotP2P = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "custody_earliest_available_slot_p2p",
		Help: "The earliest available slot tracked by the p2p service for custody purposes",
	})

	// EarliestAvailableSlotDB tracks the earliest available slot in the database
	EarliestAvailableSlotDB = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "custody_earliest_available_slot_db",
		Help: "The earliest available slot tracked by the database for custody purposes",
	})
)

// UpdateP2PMetric updates the P2P earliest available slot metric
func UpdateP2PMetric(slot primitives.Slot) {
	EarliestAvailableSlotP2P.Set(float64(slot))
}

// UpdateDBMetric updates the DB earliest available slot metric
func UpdateDBMetric(slot primitives.Slot) {
	EarliestAvailableSlotDB.Set(float64(slot))
}
