package verification

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	blobVerificationProposerSignatureCache = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blob_verification_proposer_signature_cache",
			Help: "BlobSidecar proposer signature cache result.",
		},
		[]string{"result"},
	)
	columnVerificationProposerSignatureCache = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "data_column_verification_proposer_signature_cache",
			Help: "DataColumnSidecar proposer signature cache result.",
		},
		[]string{"result"},
	)
	dataColumnSidecarInclusionProofVerificationHistogram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "beacon_data_column_sidecar_inclusion_proof_verification_milliseconds",
			Help:    "Captures the time taken to verify data column sidecar inclusion proof.",
			Buckets: []float64{5, 10, 50, 100, 150, 250, 500, 1000, 2000},
		},
	)
	dataColumnBatchKZGVerificationHistogram = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "beacon_kzg_verification_data_column_batch_milliseconds",
			Help:    "Captures the time taken for batched data column kzg verification.",
			Buckets: []float64{5, 10, 50, 100, 150, 250, 500, 1000, 2000},
		},
	)
)
