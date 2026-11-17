package stategen

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func readStateGenMetric(t *testing.T, metric any) *dto.Metric {
	t.Helper()
	got := &dto.Metric{}
	require.NoError(t, metric.(prometheus.Metric).Write(got))
	return got
}

func TestStateGenMetricTypesAndBuckets(t *testing.T) {
	for _, metric := range []any{replayBlockCount, replayBlocksSummary, replayToSlotSummary, saveStateToColdSummary, stategenLoadStateMilliseconds.WithLabelValues("db"), migrateToColdMilliseconds.WithLabelValues("hdiff")} {
		require.NotNil(t, readStateGenMetric(t, metric).Histogram)
	}
	require.Equal(t, len(replayBlocksCountBuckets), len(readStateGenMetric(t, replayBlockCount).Histogram.Bucket))
	require.Equal(t, len(stategenMillisecondsBuckets), len(readStateGenMetric(t, replayBlocksSummary).Histogram.Bucket))
}

func TestObserveMigrateToCold(t *testing.T) {
	beforeSuccess := readStateGenMetric(t, migrateToColdMilliseconds.WithLabelValues("legacy")).Histogram.GetSampleCount()
	beforeFailure := readStateGenMetric(t, migrateToColdFailures.WithLabelValues("legacy")).Counter.GetValue()
	observeMigrateToCold("legacy", time.Now(), nil)
	observeMigrateToCold("legacy", time.Now(), errUnknownBlock)
	require.Equal(t, beforeSuccess+1, readStateGenMetric(t, migrateToColdMilliseconds.WithLabelValues("legacy")).Histogram.GetSampleCount())
	require.Equal(t, beforeFailure+1, readStateGenMetric(t, migrateToColdFailures.WithLabelValues("legacy")).Counter.GetValue())
	require.Equal(t, float64(1), boolToFloat(true))
	require.Equal(t, float64(0), boolToFloat(false))
}
