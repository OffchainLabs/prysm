package proofengine

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// verificationResult is the outcome of a zkVM verification, as the result label of verificationDuration.
type verificationResult string

const (
	verificationValid   verificationResult = "valid"
	verificationInvalid verificationResult = "invalid"
)

var verificationDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "execution_proof_verification_seconds",
		Help:    "Time to verify an EIP-8025 execution proof with the zkVM verifier.",
		Buckets: prometheus.ExponentialBuckets(0.005, 2, 14), // 5ms to ~41s.
	},
	[]string{"proof_type", "result"},
)
