package validator

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	pqSlotOffsetBuckets = []float64{0.25, 0.5, 1, 1.5, 2, 2.5, 3, 3.5, 4, 4.5, 5, 5.5, 6, 6.5, 7, 7.5, 8, 9, 10, 11, 12}

	pqAggregateReadyOffset = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "pq_committee_aggregate_ready_slot_offset_seconds",
		Help:    "Time into the slot at which the committee aggregate of a local aggregator was ready.",
		Buckets: pqSlotOffsetBuckets,
	})
	pqAggregateDueMargin = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "pq_committee_aggregate_due_margin_seconds",
		Help:    "Time left before the aggregate was due for its aggregator position when it was ready, negative when late.",
		Buckets: []float64{-4, -2, -1, -0.5, -0.25, 0, 0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4},
	})
	pqAggregateLate = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pq_committee_aggregate_late_total",
		Help: "Committee aggregates of a local aggregator that were ready after they were due.",
	})
	pqProposalPackDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "pq_proposal_pack_duration_seconds",
		Help:    "Time block production spent choosing and joining vote slot proofs.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1, 1.5, 2, 3},
	})
	pqProposalPackedOffset = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "pq_proposal_packed_slot_offset_seconds",
		Help:    "Time into the proposal slot at which the attestations and their proof were packed.",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4, 6},
	})
)
