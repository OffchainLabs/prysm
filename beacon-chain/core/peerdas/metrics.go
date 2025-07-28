package peerdas

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var dataColumnComputationTime = promauto.NewHistogram(
	prometheus.HistogramOpts{
		Name:    "beacon_data_column_sidecar_computation_milliseconds",
		Help:    "Captures the time taken to compute data column sidecars from blobs.",
		Buckets: []float64{25, 50, 100, 250, 500, 750, 1000},
	},
)
