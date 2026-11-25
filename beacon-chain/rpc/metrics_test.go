package rpc

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestHTTPRequestLatencyBuckets(t *testing.T) {
	var metric dto.Metric
	require.NoError(t, httpRequestLatency.WithLabelValues("test", "200", "GET").(prometheus.Metric).Write(&metric))

	var bounds []float64
	for _, bucket := range metric.GetHistogram().GetBucket() {
		bounds = append(bounds, bucket.GetUpperBound())
	}
	require.DeepEqual(t, []float64{0.001, 0.01, 0.025, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300}, bounds)
}
