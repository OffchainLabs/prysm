package doublylinkedtree

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

var (
	log = logrus.WithField("prefix", "forkchoice-doublylinkedtree")

	headSlotNumber = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "doublylinkedtree_head_slot",
			Help: "The slot number of the current head.",
		},
	)
	nodeCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "doublylinkedtree_node_count",
			Help: "The number of nodes in the doubly linked tree based store structure.",
		},
	)
	headChangesCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_head_changed_count",
			Help: "The number of times head changes.",
		},
	)
	calledHeadCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_head_requested_count",
			Help: "The number of times someone called head.",
		},
	)
	processedBlockCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_block_processed_count",
			Help: "The number of times a block is processed for fork choice.",
		},
	)
	processedAttestationCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_attestation_processed_count",
			Help: "The number of times an attestation is processed for fork choice.",
		},
	)
	prunedCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_pruned_count",
			Help: "The number of times pruning happened.",
		},
	)
	safeHeadSlotNumber = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "doublylinkedtree_safe_head_slot",
			Help: "The slot number of the current safe head.",
		},
	)
	safeHeadChangesCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "doublylinkedtree_safe_head_changed_count",
			Help: "The number of times safe head changes.",
		},
	)
	safeHeadReorgCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "safe_head_reorgs_total",
		Help: "Count the number of safe head reorgs",
	})
	safeHeadReorgDistance = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "safe_head_reorg_distance",
			Help:    "Captures distance of safe head reorgs. Distance is defined as the number of blocks between the old safe head and the new safe head",
			Buckets: []float64{1, 2, 4, 8, 16, 32, 64},
		},
	)
	safeHeadReorgDepth = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "safe_head_reorg_depth",
			Help:    "Captures depth of safe head reorgs. Depth is defined as the number of blocks between the safe heads and the common ancestor",
			Buckets: []float64{1, 2, 4, 8, 16, 32},
		},
	)
)
