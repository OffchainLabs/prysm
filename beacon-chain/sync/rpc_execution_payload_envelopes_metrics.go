package sync

type executionPayloadEnvelopeRPCResult string

const (
	executionPayloadEnvelopeRPCResultServed              executionPayloadEnvelopeRPCResult = "served"
	executionPayloadEnvelopeRPCResultInvalid             executionPayloadEnvelopeRPCResult = "invalid"
	executionPayloadEnvelopeRPCResultRateLimited         executionPayloadEnvelopeRPCResult = "rate_limited"
	executionPayloadEnvelopeRPCResultResourceUnavailable executionPayloadEnvelopeRPCResult = "resource_unavailable"
	executionPayloadEnvelopeRPCResultError               executionPayloadEnvelopeRPCResult = "error"
)
