package backfill

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/sync"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
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
			Name:    "backfill_batch_time_waiting",
			Help:    "Time batch waited for a suitable peer.",
			Buckets: []float64{50, 100, 300, 1000, 2000},
		},
	)
	backfillBatchTimeRoundtrip = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_time_roundtrip",
			Help:    "Total time to import batch, from first scheduled to imported.",
			Buckets: []float64{400, 800, 1600, 3200, 6400, 12800},
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
			Name: "backfill_blocks_bytes_downloaded",
			Help: "BeaconBlock bytes downloaded from peers for backfill.",
		},
	)
	blockDownloadMs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_blocks_time_download",
			Help:    "BeaconBlock download time, in ms.",
			Buckets: []float64{100, 300, 1000, 2000, 4000, 8000},
		},
	)
	blockVerifyMs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_time_verify",
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
			Name: "backfill_blobs_bytes_downloaded",
			Help: "BlobSidecar bytes downloaded from peers for backfill.",
		},
	)
	blobSidecarDownloadMs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_blobs_time_download",
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
			Name: "backfill_data_column_sidecar_bytes_downloaded",
			Help: "DataColumnSidecar bytes downloaded from peers for backfill.",
		},
	)
	dataColumnSidecarDownloadMs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_columns_time_download",
			Help:    "DataColumnSidecars download time, in ms.",
			Buckets: []float64{100, 300, 1000, 2000, 4000, 8000},
		},
	)
	dataColumnSidecarVerifyMs = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "backfill_batch_columns_time_verify",
			Help:    "DataColumnSidecars verification time, in ms.",
			Buckets: []float64{100, 300, 1000, 2000, 4000, 8000},
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
