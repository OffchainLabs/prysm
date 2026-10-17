package rest

import (
	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Outcomes recorded by nodeResponseTotal.
const (
	outcomeMatched  = "matched"
	outcomeFallback = "fallback"
	outcomeError    = "error"
)

// No endpoint label: endpoints embed the slot, so the cardinality is unbounded.
var nodeResponseTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "validator",
		Name:      "beacon_node_response_total",
		Help:      "Beacon node responses by how the multi-handler used them within a single read round.",
	},
	[]string{"host", "outcome"},
)

// recordResponse credits a host with an outcome, redacting any credentials in the host.
func recordResponse(host, outcome string) {
	nodeResponseTotal.WithLabelValues(api.RedactEndpoint(host), outcome).Inc()
}
