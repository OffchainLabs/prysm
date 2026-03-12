package gloas

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	gloasBuilderPendingPaymentsProcessedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gloas_builder_pending_payments_processed_total",
			Help: "The number of builder pending payments promoted into the builder pending withdrawal queue.",
		},
	)
	gloasBuilderDepositsProcessedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gloas_builder_deposits_processed_total",
			Help: "The number of builder-related deposit requests processed.",
		},
	)
	gloasBuilderExitsProcessedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gloas_builder_exits_processed_total",
			Help: "The number of processed builder exits.",
		},
	)
)
