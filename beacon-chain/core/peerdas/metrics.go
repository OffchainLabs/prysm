package peerdas

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var dataColumnComputationTime = promauto.NewHistogram(
	prometheus.HistogramOpts{
		Name:    "data_column_sidecar_computation_milliseconds",
		Help:    "Captures the time taken to compute data column sidecars from blobs.",
		Buckets: []float64{100, 250, 500, 750, 1000, 1500, 2000, 4000, 8000, 12000, 16000},
	},
)
