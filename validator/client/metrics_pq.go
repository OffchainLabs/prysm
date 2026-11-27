package client

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	pqAttestationSigningTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "validator",
		Name:      "pq_attestation_signing_seconds",
		Help:      "Time to sign one PQ attestation with XMSS, with the signing protection write.",
		Buckets:   []float64{0.005, 0.01, 0.02, 0.03, 0.05, 0.075, 0.1, 0.15, 0.2, 0.3, 0.5, 0.75, 1, 1.5, 2, 4},
	})
	pqAttestationPublishedOffset = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "validator",
		Name:      "pq_attestation_published_slot_offset_seconds",
		Help:      "Time into the slot at which the beacon node accepted a PQ attestation.",
		Buckets:   []float64{0.5, 1, 1.5, 2, 2.25, 2.5, 2.75, 3, 3.25, 3.5, 3.75, 4, 4.5, 5, 6, 8, 12},
	})
	pqAggregatePublishedOffset = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "validator",
		Name:      "pq_aggregate_published_slot_offset_seconds",
		Help:      "Time into the slot at which the beacon node accepted a PQ aggregate and proof.",
		Buckets:   []float64{4, 5, 6, 6.5, 7, 7.25, 7.5, 7.75, 8, 8.5, 9, 9.5, 10, 11, 12},
	})
)
