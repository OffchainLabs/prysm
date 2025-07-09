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
	dataColumnBuckets     = []float64{3, 5, 7, 9, 11, 13}
	dataColumnSaveLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "data_column_storage_save_latency",
		Help:    "Latency of DataColumnSidecar storage save operations in milliseconds",
		Buckets: dataColumnBuckets,
	})
	dataColumnFetchLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "data_column_storage_get_latency",
		Help:    "Latency of DataColumnSidecar storage get operations in milliseconds",
		Buckets: dataColumnBuckets,
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
)
