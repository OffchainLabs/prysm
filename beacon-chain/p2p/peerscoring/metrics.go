package peerscoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var badResponsesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "p2p_bad_responses_total",
	Help: "Bad-response strikes recorded against peers, by reporting source.",
}, []string{"source"})
