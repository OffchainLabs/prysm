package pq

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	proofDurationBuckets = []float64{0.005, 0.01, 0.015, 0.02, 0.025, 0.03, 0.04, 0.05, 0.075, 0.1, 0.25, 0.5, 0.55, 0.6, 0.65, 0.7, 0.75, 1, 1.5, 2, 3, 4, 6, 8, 12}
	proofClaimsBuckets   = []float64{1, 2, 3, 4, 6, 8, 32, 64, 128, 192, 256, 384, 512, 1024, 2048, 4096, 8192}

	proverQueueWait = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "pq_prover_queue_wait_seconds",
		Help:    "Time a proving job waited for the leanVM prover, which runs one job at a time.",
		Buckets: proofDurationBuckets,
	})
	proverDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pq_prover_duration_seconds",
		Help:    "Time of one leanVM proving call, without the wait for the prover.",
		Buckets: proofDurationBuckets,
	}, []string{"op"})
	proverInputs = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pq_prover_inputs",
		Help:    "Signatures of a prove call or child proofs of a merge call.",
		Buckets: proofClaimsBuckets,
	}, []string{"op"})
	verifyDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pq_proof_verify_duration_seconds",
		Help:    "Time to verify an attestation proof, by where the proof came from.",
		Buckets: proofDurationBuckets,
	}, []string{"source"})
)

func verifyProof(source string, statements []Statement, proof []byte) error {
	start := time.Now()
	err := Proofs.Verify(statements, proof)
	verifyDuration.WithLabelValues(source).Observe(time.Since(start).Seconds())
	return err
}
