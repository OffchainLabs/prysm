package payloadattestation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var payloadAttestationPoolSize = promauto.NewGauge(
	prometheus.GaugeOpts{
		Name: "payload_attestation_pool_size",
		Help: "The number of unique payload attestation entries currently in the pool.",
	},
)
