package beacon

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	publishBlockV2Duration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "publish_block_v2_duration_milliseconds",
			Help:    "Duration of publishBlockV2 endpoint processing in milliseconds",
			Buckets: []float64{1, 5, 20, 100, 500, 1000, 2000, 5000},
		},
	)
)
