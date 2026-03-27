package fieldtrie

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	fieldTrieNodesBytesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "field_trie_nodes_bytes",
		Help: "Total bytes used by trie nodes for a particular field.",
	}, []string{"field"})

	fieldTrieOverridesBytesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "field_trie_overrides_bytes",
		Help: "Total bytes used by overlay overrides for a particular field.",
	}, []string{"field"})
)
