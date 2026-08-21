package sync

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type executionPayloadEnvelopeRPCResult string

const (
	executionPayloadEnvelopeRPCResultServed              executionPayloadEnvelopeRPCResult = "served"
	executionPayloadEnvelopeRPCResultInvalid             executionPayloadEnvelopeRPCResult = "invalid"
	executionPayloadEnvelopeRPCResultRateLimited         executionPayloadEnvelopeRPCResult = "rate_limited"
	executionPayloadEnvelopeRPCResultResourceUnavailable executionPayloadEnvelopeRPCResult = "resource_unavailable"
	executionPayloadEnvelopeRPCResultError               executionPayloadEnvelopeRPCResult = "error"
	// executionPayloadEnvelopeRPCResultEmptyDomain records a request whose
	// intersection with the envelope domain [max(1, gloasStart), current] is
	// empty: zero chunks and clean EOF, with no coverage state consulted.
	executionPayloadEnvelopeRPCResultEmptyDomain executionPayloadEnvelopeRPCResult = "empty_domain"
)

var (
	envelopeRPCLiveFrontierTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "execution_payload_envelope_rpc_live_frontier_total",
		Help: "Envelopes-by-range live-frontier responses by shape (with_head, without_head, head_only, empty).",
	}, []string{"result"})
	envelopeRPCEmptyDomainTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_payload_envelope_rpc_empty_domain_total",
		Help: "Envelopes-by-range requests whose intersection with the envelope domain was empty.",
	})
	envelopeRPCQuotaTruncatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_payload_envelope_rpc_quota_truncated_total",
		Help: "Envelopes-by-range responses truncated at the item quota inside proven coverage.",
	})
	envelopeRPCServeEpochTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "execution_payload_envelope_rpc_serve_epoch_total",
		Help: "Envelopes-by-range serve-epoch invalidations by outcome (retry, refused).",
	}, []string{"result"})
)
