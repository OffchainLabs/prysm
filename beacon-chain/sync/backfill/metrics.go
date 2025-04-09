package backfill

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/sync"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
)

var (
	oldestBatch = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "backfill_earliest_wip_slot",
			Help: "Earliest slot that has been assigned to a worker.",
		},
	)
	batchesWaiting = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "backfill_importable_batches_waiting",
			Help: "Number of batches that are ready to be imported once they can be connected to the existing chain.",
		},
	)
	backfillRemainingBatches = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "backfill_remaining_batches",
			Help: "Backfill remaining batches.",
		},
	)
	backfillBatchesImported = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_batches_imported",
			Help: "Number of backfill batches downloaded and imported.",
		},
	)
	backfillBlocksApproximateBytes = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_blocks_bytes_downloaded",
			Help: "BeaconBlock bytes downloaded from peers for backfill.",
		},
	)
	backfillBlobsApproximateBytes = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_blobs_bytes_downloaded",
			Help: "BlobSidecar bytes downloaded from peers for backfill.",
		},
	)
	backfillBlobsDownloadCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_blobs_download_count",
			Help: "Number of BlobSidecar values downloaded from peers for backfill.",
		},
	)
	backfillBlocksDownloadCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_blocks_download_count",
			Help: "Number of BeaconBlock values downloaded from peers for backfill.",
		},
	)
	backfillBatchTimeRoundtrip = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_time_roundtrip",
			Help:    "Total time to import batch, from first scheduled to imported.",
			Buckets: []float64{400, 800, 1600, 3200, 6400, 12800},
		},
	)
	backfillBatchTimeWaiting = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_time_waiting",
			Help:    "Time batch waited for a suitable peer.",
			Buckets: []float64{50, 100, 300, 1000, 2000},
		},
	)
	backfillBatchTimeDownloadingBlocks = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_blocks_time_download",
			Help:    "Time, in milliseconds, batch spent downloading blocks from peer.",
			Buckets: []float64{100, 300, 1000, 2000, 4000, 8000},
		},
	)
	backfillBatchTimeDownloadingBlobs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_blobs_time_download",
			Help:    "Time, in milliseconds, batch spent downloading blobs from peer.",
			Buckets: []float64{100, 300, 1000, 2000, 4000, 8000},
		},
	)
	backfillBatchTimeVerifying = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_time_verify",
			Help:    "Time batch spent downloading blocks from peer.",
			Buckets: []float64{100, 300, 1000, 2000, 4000, 8000},
		},
	)
)

func blobValidationMetrics(_ blocks.ROBlob) error {
	backfillBlobsDownloadCount.Inc()
	return nil
}

func blockValidationMetrics(interfaces.ReadOnlySignedBeaconBlock) error {
	backfillBlocksDownloadCount.Inc()
	return nil
}

var _ sync.BlobResponseValidation = blobValidationMetrics
var _ sync.BeaconBlockProcessor = blockValidationMetrics
