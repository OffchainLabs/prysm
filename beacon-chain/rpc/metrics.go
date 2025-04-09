package rpc

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_latency_seconds",
			Help:    "Latency of HTTP requests in seconds",
			Buckets: []float64{0.001, 0.01, 0.025, 0.1, 0.25, 1, 2.5, 10},
		},
		[]string{"endpoint", "code", "method"},
	)
	httpRequestCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_count",
			Help: "Number of HTTP requests",
		},
		[]string{"endpoint", "code", "method"},
	)
	httpErrorCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_error_count",
			Help: "Total HTTP errors for beacon node requests",
		},
		[]string{"endpoint", "code", "method"},
	)
)
