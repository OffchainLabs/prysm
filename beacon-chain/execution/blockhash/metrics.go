package blockhash

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	blockHashComputationTime = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "execution_block_hash_computation_microseconds",
			Help:    "Time taken to compute the execution block hash of a payload locally, in microseconds",
			Buckets: []float64{50, 100, 250, 500, 1000, 2500, 5000, 10000, 50000},
		},
	)
	blockHashMismatchCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "execution_block_hash_mismatch_total",
			Help: "Number of execution payloads whose locally computed block hash did not match the claimed one",
		},
	)
)
