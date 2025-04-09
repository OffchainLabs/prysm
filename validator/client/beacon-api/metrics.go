package beacon_api

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpActionLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "validator",
			Name:      "http_action_latency_seconds",
			Help:      "Latency of HTTP actions performed against the beacon node in seconds. This metric captures only actions that didn't result in an error.",
			Buckets:   []float64{0.001, 0.01, 0.025, 0.1, 0.25, 1, 2.5, 10},
		},
		[]string{"action"},
	)
	httpActionCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "validator",
			Name:      "http_action_count",
			Help:      "Number of all HTTP actions performed against the beacon node",
		},
		[]string{"action"},
	)
	failedHTTPActionCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "validator",
			Name:      "failed_http_action_count",
			Help:      "Number of failed HTTP actions performed against the beacon node",
		},
		[]string{"action"},
	)
)
