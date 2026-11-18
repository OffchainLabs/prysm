package peerscoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var strikesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "p2p_peer_strikes_total",
	Help: "Strikes recorded against peers, by reporting source.",
}, []string{"source"})
