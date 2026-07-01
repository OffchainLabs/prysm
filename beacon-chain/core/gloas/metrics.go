package gloas

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	builderPendingPaymentsProcessedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "builder_pending_payments_processed_total",
			Help: "The number of builder pending payments promoted into the builder pending withdrawal queue.",
		},
	)
	builderDepositsProcessedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "builder_deposits_processed_total",
			Help: "The number of builder-related deposit requests processed.",
		},
	)
	builderExitsProcessedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "builder_exits_processed_total",
			Help: "The number of processed builder exits.",
		},
	)
)
