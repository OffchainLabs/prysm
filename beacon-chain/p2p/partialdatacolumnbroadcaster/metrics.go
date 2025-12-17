package partialdatacolumnbroadcaster

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	partialMessageUsefulCellsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "beacon_partial_message_useful_cells_total",
		Help: "Number of useful cells received via a partial message",
	}, []string{"column_index"})

	partialMessageCellsReceivedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "beacon_partial_message_cells_received_total",
		Help: "Number of total cells received via a partial message",
	}, []string{"column_index"})
)
