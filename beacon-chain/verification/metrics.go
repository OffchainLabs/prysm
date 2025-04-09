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
)
