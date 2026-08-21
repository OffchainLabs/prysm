package coverage

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	coverageLowSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "execution_payload_envelope_coverage_low_slot",
		Help: "Inclusive lower bound of the published execution payload envelope coverage interval.",
	})
	coverageHighSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "execution_payload_envelope_coverage_high_slot",
		Help: "Exclusive upper bound of the published execution payload envelope coverage interval.",
	})
	anchorShrinksTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_payload_envelope_anchor_shrinks_total",
		Help: "Number of coverage upper-anchor shrinks or discards caused by reorgs.",
	})
	migrationPagesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_payload_envelope_migration_pages_total",
		Help: "Number of committed coverage scan pages (migration, lower and upper extension).",
	})
	migrationEntriesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_payload_envelope_migration_entries_total",
		Help: "Number of block slot index candidates examined by coverage scans.",
	})
	migrationFirstPublishSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "execution_payload_envelope_migration_first_publish_seconds",
		Help: "Seconds from runtime start to the first non-empty coverage publication.",
	})
	migrationDurationSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "execution_payload_envelope_migration_duration_seconds",
		Help: "Seconds from runtime start until the coverage lower bound first reached the retention floor.",
	})
)
