package fieldtrie

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	fieldTrieEntriesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "field_trie_entries",
		Help: "Total number of entries in field tries, by field and component (nodes/overrides).",
	}, []string{"field", "component"})

	fieldTrieCountGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "field_trie_count",
		Help: "Total number of live field trie instances by field and mode (overlay/owned).",
	}, []string{"field", "mode"})

	fieldTrieLeafOverridesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "field_trie_leaf_overrides",
		Help: "Number of leaf-level (level 0) override entries in overlay field tries.",
	}, []string{"field"})

	// FieldTriePromotionCounter counts overlay-to-owned promotions.
	FieldTriePromotionCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "field_trie_promotion_total",
		Help: "Total number of overlay promotions triggered by exceeding the threshold.",
	}, []string{"field"})
)
