package execution

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	newPayloadLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "new_payload_v1_latency_milliseconds",
			Help:    "Captures RPC latency for newPayloadV1 in milliseconds",
			Buckets: []float64{25, 50, 100, 200, 500, 1000, 2000, 4000},
		},
	)
	getPayloadLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "get_payload_v1_latency_milliseconds",
			Help:    "Captures RPC latency for getPayloadV1 in milliseconds",
			Buckets: []float64{25, 50, 100, 200, 500, 1000, 2000, 4000},
		},
	)
	forkchoiceUpdatedLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "forkchoice_updated_v1_latency_milliseconds",
			Help:    "Captures RPC latency for forkchoiceUpdatedV1 in milliseconds",
			Buckets: []float64{25, 50, 100, 200, 500, 1000, 2000, 4000},
		},
	)
	getBlobsV2Latency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "get_blobs_v2_latency_milliseconds",
			Help:    "Captures RPC latency for getBlobsV2 in milliseconds",
			Buckets: []float64{25, 50, 100, 200, 500, 1000, 2000, 4000},
		},
	)
	getBlobsV3RequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_engine_getBlobsV3_requests_total",
		Help: "Total number of engine_getBlobsV3 requests sent",
	})
	getBlobsV3CompleteResponsesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_engine_getBlobsV3_complete_responses_total",
		Help: "Total number of complete engine_getBlobsV3 successful responses received",
	})
	getBlobsV3PartialResponsesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_engine_getBlobsV3_partial_responses_total",
		Help: "Total number of engine_getBlobsV3 partial responses received",
	})
	getBlobsV3EmptyResponsesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_engine_getBlobsV3_empty_responses_total",
		Help: "Total number of engine_getBlobsV3 responses received with no included blobs",
	})
	getBlobsV3Latency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "beacon_engine_getBlobsV3_request_duration_seconds",
			Help:    "Duration of engine_getBlobsV3 requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
	hasBlobsRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "beacon_engine_hasBlobs_requests_total",
		Help: "Total number of engine_hasBlobs requests sent",
	})
	hasBlobsLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "beacon_engine_hasBlobs_request_duration_seconds",
			Help:    "Duration of engine_hasBlobs requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
	errParseCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_parse_error_count",
		Help: "The number of errors that occurred while parsing execution payload",
	})
	errInvalidRequestCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_invalid_request_count",
		Help: "The number of errors that occurred due to invalid request",
	})
	errMethodNotFoundCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_method_not_found_count",
		Help: "The number of errors that occurred due to method not found",
	})
	errInvalidParamsCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_invalid_params_count",
		Help: "The number of errors that occurred due to invalid params",
	})
	errInternalCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_internal_error_count",
		Help: "The number of errors that occurred due to internal error",
	})
	errUnknownPayloadCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_unknown_payload_count",
		Help: "The number of errors that occurred due to unknown payload",
	})
	errInvalidForkchoiceStateCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_invalid_forkchoice_state_count",
		Help: "The number of errors that occurred due to invalid forkchoice state",
	})
	errInvalidPayloadAttributesCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_invalid_payload_attributes_count",
		Help: "The number of errors that occurred due to invalid payload attributes",
	})
	errServerErrorCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_server_error_count",
		Help: "The number of errors that occurred due to server error",
	})
	reconstructedExecutionPayloadCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "reconstructed_execution_payload_count",
		Help: "Count the number of execution payloads that are reconstructed using JSON-RPC from payload headers",
	})
	errRequestTooLargeCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_payload_bodies_count",
		Help: "The number of requested payload bodies is too large",
	})
	// engineRequestLatency times every engine op at the engineTransport seam,
	// labeled by endpoint and transport (json-rpc vs ssz-http) so the two wire
	// transports can be compared for the same logical call.
	engineRequestLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "engine_request_latency_milliseconds",
			Help:    "Engine API request latency per endpoint and transport",
			Buckets: []float64{25, 50, 100, 200, 500, 1000, 2000, 4000},
		},
		[]string{"method", "transport"},
	)
	// engineBodySize records the wire size of engine request/response bodies,
	// labeled by endpoint, transport, and direction. Both transports populate it:
	// ssz-http via observeSSZBody (engine_ssz.go), json-rpc via the size
	// round-tripper (engine_jsonrpc_size.go), so the two are directly comparable.
	engineBodySize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "engine_body_size_bytes",
			Help:    "Engine API request/response body size in bytes per endpoint, transport and direction",
			Buckets: prometheus.ExponentialBuckets(256, 4, 10), // 256 B .. 64 MiB
		},
		[]string{"method", "transport", "direction"},
	)
	// engineSSZHTTPFallbackCount counts how often SSZ-over-HTTP selection fell
	// back to JSON-RPC for a connection (flag on but the client could not be built
	// or the EL served no v2 surface). Selection is sticky per connection, so each
	// increment is one connection that ran on JSON-RPC despite the flag.
	engineSSZHTTPFallbackCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "engine_ssz_http_fallback_count",
		Help: "The number of connections that fell back from SSZ-over-HTTP to JSON-RPC",
	})
)
