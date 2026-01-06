package peerdas

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	dataColumnComputationTime = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "beacon_data_column_sidecar_computation_milliseconds",
			Help:    "Captures the time taken to compute data column sidecars from blobs.",
			Buckets: []float64{25, 50, 100, 250, 500, 750, 1000},
		},
	)

	cellsAndProofsFromStructuredComputationTime = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "cells_and_proofs_from_structured_computation_milliseconds",
			Help:    "Captures the time taken to compute cells and proofs from structured computation.",
			Buckets: []float64{10, 20, 30, 40, 50, 100, 200},
		},
	)
)
