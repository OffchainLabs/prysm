package pqpool

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	durationBuckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.55, 0.6, 0.65, 0.7, 0.75, 1, 1.5, 2, 3, 4, 6, 8, 12}

	// Kinds are committee, vote_slot and join.
	proofDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pq_aggregation_duration_seconds",
		Help:    "Wall time to build one attestation proof, with the wait for the prover, by kind of proof.",
		Buckets: durationBuckets,
	}, []string{"kind"})
	proofClaims = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "pq_aggregation_claims",
		Help:    "Signatures covered by one built attestation proof, by kind of proof.",
		Buckets: []float64{1, 8, 32, 64, 128, 192, 256, 384, 512, 1024, 2048, 4096, 8192},
	}, []string{"kind"})
	voteSlotProofLead = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "pq_vote_slot_proof_ready_before_proposal_seconds",
		Help:    "Time between the first proof of a vote slot being ready and the start of the earliest slot that can include it, negative when late.",
		Buckets: []float64{-6, -3, -1, 0, 1, 2, 3, 4, 6, 8, 10, 12, 13, 13.5, 14, 14.5, 15, 16, 18, 21, 24},
	})
)

func observeProof(kind string, start time.Time, claims float64) {
	proofDuration.WithLabelValues(kind).Observe(time.Since(start).Seconds())
	proofClaims.WithLabelValues(kind).Observe(claims)
}
