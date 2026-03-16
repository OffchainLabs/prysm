package state_native

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	gloasExecutionPayloadAvailabilityRatio = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gloas_execution_payload_availability_ratio",
			Help: "The fraction of execution payload availability bits currently set in the beacon state.",
		},
	)
	gloasBuilderPendingWithdrawalsCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gloas_builder_pending_withdrawals_count",
			Help: "The number of builder pending withdrawals currently stored in the beacon state.",
		},
	)
	gloasBuilderPendingWithdrawalsGwei = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gloas_builder_pending_withdrawals_gwei",
			Help: "The total gwei amount currently stored in builder pending withdrawals.",
		},
	)
	gloasPayloadExpectedWithdrawalsCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gloas_payload_expected_withdrawals_count",
			Help: "The number of expected withdrawals currently cached for the next payload.",
		},
	)
	gloasActiveBuildersCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gloas_active_builders_count",
			Help: "The number of active builders currently tracked in the beacon state.",
		},
	)
	gloasActiveBuildersBalanceGwei = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gloas_active_builders_balance_gwei",
			Help: "The total balance in gwei held by active builders currently tracked in the beacon state.",
		},
	)
)
