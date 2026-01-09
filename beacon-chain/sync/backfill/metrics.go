package backfill

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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
	batchesRemaining = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "backfill_remaining_batches",
			Help: "Backfill remaining batches.",
		},
	)
	batchesImported = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_batches_imported",
			Help: "Number of backfill batches downloaded and imported.",
		},
	)

	backfillBatchTimeWaiting = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_waiting_ms",
			Help:    "Time batch waited for a suitable peer in ms.",
			Buckets: []float64{50, 100, 300, 1000, 2000},
		},
	)
	backfillBatchTimeRoundtrip = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_roundtrip_ms",
			Help:    "Total time to import batch, from first scheduled to imported.",
			Buckets: []float64{1000, 2000, 4000, 6000, 10000},
		},
	)

	blockDownloadCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_blocks_download_count",
			Help: "Number of BeaconBlock values downloaded from peers for backfill.",
		},
	)
	blockDownloadBytesApprox = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_blocks_downloaded_bytes",
			Help: "BeaconBlock bytes downloaded from peers for backfill.",
		},
	)
	blockDownloadMs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_blocks_download_ms",
			Help:    "BeaconBlock download time, in ms.",
			Buckets: []float64{100, 300, 1000, 2000, 4000, 8000},
		},
	)
	blockVerifyMs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_verify_ms",
			Help:    "BeaconBlock verification time, in ms.",
			Buckets: []float64{100, 300, 1000, 2000, 4000, 8000},
		},
	)

	blobSidecarDownloadCount = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_blobs_download_count",
			Help: "Number of BlobSidecar values downloaded from peers for backfill.",
		},
	)
	blobSidecarDownloadBytesApprox = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_blobs_downloaded_bytes",
			Help: "BlobSidecar bytes downloaded from peers for backfill.",
		},
	)
	blobSidecarDownloadMs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_blobs_download_ms",
			Help:    "BlobSidecar download time, in ms.",
			Buckets: []float64{100, 300, 1000, 2000, 4000, 8000},
		},
	)

	dataColumnSidecarDownloadCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "backfill_data_column_sidecar_downloaded",
			Help: "Number of DataColumnSidecar values downloaded from peers for backfill.",
		},
		[]string{"index", "validity"},
	)
	dataColumnSidecarDownloadBytes = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_data_column_sidecar_downloaded_bytes",
			Help: "DataColumnSidecar bytes downloaded from peers for backfill.",
		},
	)
	dataColumnSidecarDownloadMs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_columns_download_ms",
			Help:    "DataColumnSidecars download time, in ms.",
			Buckets: []float64{100, 300, 1000, 2000, 4000, 8000},
		},
	)
	dataColumnSidecarVerifyMs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_columns_verify_ms",
			Help:    "DataColumnSidecars verification time, in ms.",
			Buckets: []float64{3, 5, 10, 20, 100, 200},
		},
	)
)

func blobValidationMetrics(_ blocks.ROBlob) error {
	blobSidecarDownloadCount.Inc()
	return nil
}

func blockValidationMetrics(interfaces.ReadOnlySignedBeaconBlock) error {
	blockDownloadCount.Inc()
	return nil
}

var _ sync.BlobResponseValidation = blobValidationMetrics
var _ sync.BeaconBlockProcessor = blockValidationMetrics
