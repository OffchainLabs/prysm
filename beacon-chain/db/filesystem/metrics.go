package filesystem

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Blobs
	blobBuckets     = []float64{3, 5, 7, 9, 11, 13}
	blobSaveLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "blob_storage_save_latency",
		Help:    "Latency of BlobSidecar storage save operations in milliseconds",
		Buckets: blobBuckets,
	})
	blobFetchLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "blob_storage_get_latency",
		Help:    "Latency of BlobSidecar storage get operations in milliseconds",
		Buckets: blobBuckets,
	})
	blobsPrunedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "blob_pruned",
		Help: "Number of BlobSidecar files pruned.",
	})
	blobsWrittenCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "blob_written",
		Help: "Number of BlobSidecar files written",
	})
	blobDiskCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "blob_disk_count",
		Help: "Approximate number of blob files in storage",
	})
	blobDiskSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "blob_disk_bytes",
		Help: "Approximate number of bytes occupied by blobs in storage",
	})

	// Data columns
	dataColumnSaveLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "data_column_storage_save_latency",
		Help:    "Latency of DataColumnSidecar storage save operations in milliseconds",
		Buckets: []float64{10, 20, 30, 50, 100, 200, 500},
	})
	dataColumnFetchLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "data_column_storage_get_latency",
		Help:    "Latency of DataColumnSidecar storage get operations in milliseconds",
		Buckets: []float64{3, 5, 7, 9, 11, 13},
	})
	dataColumnPrunedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "data_column_pruned",
		Help: "Number of DataColumnSidecar pruned.",
	})
	dataColumnWrittenCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "data_column_written",
		Help: "Number of DataColumnSidecar written",
	})
	dataColumnDiskCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "data_column_disk_count",
		Help: "Approximate number of data columns in storage",
	})
	dataColumnFileSyncLatency = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "data_column_file_sync_latency",
		Help: "Latency of sync operations when saving data columns in milliseconds",
	})
	dataColumnBatchStoreCount = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "data_column_batch_store_count",
		Help: "Number of data columns stored in a batch",
	})
	dataColumnPruneLatency = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "data_column_prune_latency",
		Help: "Latency of data column prune operations in milliseconds",
	})

	// Proofs
	proofSaveLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "proof_storage_save_latency",
		Help:    "Latency of proof storage save operations in milliseconds",
		Buckets: []float64{3, 5, 7, 9, 11, 13, 20, 50},
	})
	proofFetchLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "proof_storage_get_latency",
		Help:    "Latency of proof storage get operations in milliseconds",
		Buckets: []float64{3, 5, 7, 9, 11, 13},
	})
	proofPrunedCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "proof_pruned",
		Help: "Number of proof files pruned.",
	})
	proofWrittenCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "proof_written",
		Help: "Number of proof files written",
	})
	proofDiskCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "proof_disk_count",
		Help: "Approximate number of proof files in storage",
	})
	proofFileSyncLatency = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "proof_file_sync_latency",
		Help: "Latency of sync operations when saving proofs in milliseconds",
	})
	proofPruneLatency = promauto.NewSummary(prometheus.SummaryOpts{
		Name: "proof_prune_latency",
		Help: "Latency of proof prune operations in milliseconds",
	})
)
