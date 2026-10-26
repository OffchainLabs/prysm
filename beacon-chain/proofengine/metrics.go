package proofengine

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var verificationDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "execution_proof_verification_milliseconds",
		Help:    "Time to verify an EIP-8025 execution proof with the zkVM verifier, for proofs that pass verification.",
		Buckets: prometheus.LinearBuckets(10, 10, 10), // 10ms to 100ms.
	},
	[]string{"proof_type"},
)
