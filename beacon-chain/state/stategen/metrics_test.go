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
	for _, metric := range []any{replayBlockCount, replayBlocksSummary, replayToSlotSummary, saveStateToColdSummary, stategenLoadStateMilliseconds.WithLabelValues(loadStateSourceDB), migrateToColdMilliseconds.WithLabelValues(migrationModeHdiff)} {
		require.NotNil(t, readStateGenMetric(t, metric).Histogram)
	}
	require.Equal(t, len(replayBlocksCountBuckets), len(readStateGenMetric(t, replayBlockCount).Histogram.Bucket))
	require.Equal(t, len(stategenMillisecondsBuckets), len(readStateGenMetric(t, replayBlocksSummary).Histogram.Bucket))

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, name := range []string{
		"replay_blocks_milliseconds",
		"replay_to_slot_milliseconds",
		"save_state_to_cold_milliseconds",
		"replay_blocks_count",
		"stategen_load_state_milliseconds",
		"migrate_to_cold_milliseconds",
	} {
		t.Run(name, func(t *testing.T) {
			for _, family := range families {
				if family.GetName() == name {
					require.Equal(t, dto.MetricType_HISTOGRAM, family.GetType())
					return
				}
			}
			t.Fatalf("metric %s was not gathered", name)
		})
	}
}

func TestObserveMigrateToCold(t *testing.T) {
	beforeSuccess := readStateGenMetric(t, migrateToColdMilliseconds.WithLabelValues(migrationModeLegacy)).Histogram.GetSampleCount()
	beforeFailure := readStateGenMetric(t, migrateToColdFailures.WithLabelValues(migrationModeLegacy)).Counter.GetValue()
	observeMigrateToCold(migrationModeLegacy, time.Now(), nil, false)
	observeMigrateToCold(migrationModeLegacy, time.Now(), errUnknownBlock, false)
	observeMigrateToCold(migrationModeLegacy, time.Now(), nil, true)
	require.Equal(t, beforeSuccess+1, readStateGenMetric(t, migrateToColdMilliseconds.WithLabelValues(migrationModeLegacy)).Histogram.GetSampleCount())
	require.Equal(t, beforeFailure+1, readStateGenMetric(t, migrateToColdFailures.WithLabelValues(migrationModeLegacy)).Counter.GetValue())
	require.Equal(t, float64(1), boolToFloat(true))
	require.Equal(t, float64(0), boolToFloat(false))
}
