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

	fieldTrieEntriesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "field_trie_entries",
		Help: "Total number of entries in field tries, by field and component (nodes/overrides).",
	}, []string{"field", "component"})

	fieldTrieCountGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "field_trie_count",
		Help: "Total number of live field trie data allocations by field and mode (owned/overlay).",
	}, []string{"field", "mode"})

	fieldTrieLeafOverridesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "field_trie_leaf_overrides",
		Help: "Number of leaf-level (level 0) override entries in overlay field tries.",
	}, []string{"field"})

	fieldTrieCopyCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "field_trie_copy_total",
		Help: "Total number of CopyTrie calls by field and source mode (owned/overlay).",
	}, []string{"field", "mode"})

	fieldTrieForkCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "field_trie_fork_total",
		Help: "Total number of copy-on-write forks triggered by RecomputeTrie on a shared trie.",
	}, []string{"field", "mode"})

	fieldTriePromotionCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "field_trie_promotion_total",
		Help: "Total number of overlay promotions triggered by exceeding the threshold.",
	}, []string{"field"})
)
