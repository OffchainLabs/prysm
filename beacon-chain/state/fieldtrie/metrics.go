package fieldtrie

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	fieldTrieRecomputeIndicesSummary = promauto.NewSummaryVec(prometheus.SummaryOpts{
		Name: "field_trie_recompute_indices",
		Help: "Distribution of the number of changed indices per RecomputeTrie call.",
	}, []string{"field"})
)
