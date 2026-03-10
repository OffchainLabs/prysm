package validator

import (
	"net/http"

	"github.com/OffchainLabs/prysm/v7/network/httputil"
)

// ProduceBlockV4 requests a beacon node to produce a valid Gloas block.
//
// TODO: Implement Gloas-specific block production.
// Endpoint: GET /eth/v4/validator/blocks/{slot}
func (s *Server) ProduceBlockV4(w http.ResponseWriter, r *http.Request) {
	httputil.HandleError(w, "ProduceBlockV4 not yet implemented", http.StatusNotImplemented)
}

// ExecutionPayloadEnvelope retrieves a cached execution payload envelope.
//
// TODO: Implement envelope retrieval from cache.
// Endpoint: GET /eth/v1/validator/execution_payload_envelope/{slot}/{builder_index}
func (s *Server) ExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	httputil.HandleError(w, "ExecutionPayloadEnvelope not yet implemented", http.StatusNotImplemented)
}

// PublishExecutionPayloadEnvelope broadcasts a signed execution payload envelope.
//
// TODO: Implement envelope validation and broadcast.
// Endpoint: POST /eth/v1/beacon/execution_payload_envelope
func (s *Server) PublishExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	httputil.HandleError(w, "PublishExecutionPayloadEnvelope not yet implemented", http.StatusNotImplemented)
}
