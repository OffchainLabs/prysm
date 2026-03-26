package fieldtrie

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// FieldReferences tracks reference counts per field type. Shared field
// references are set externally; overlay trie counts are managed
// internally by CopyTrie via Inc/Dec with runtime finalizers.
var FieldReferences = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "field_references",
	Help: "The number of states a particular field is shared with.",
}, []string{"state"})
