package kv

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var savedBySeenAggregatedCache = promauto.NewCounter(prometheus.CounterOpts{
	Name: "attestation_saved_by_seen_aggregated_cache_total",
	Help: "The number of times an attestation was found only in the seen aggregated cache and not in the regular caches.",
})
