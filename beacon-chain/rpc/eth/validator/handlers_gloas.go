package validator

import (
	"net/http"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
// Endpoint: GET /eth/v1/validator/execution_payload_envelope/{slot}
func (s *Server) ExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.ExecutionPayloadEnvelope")
	defer span.End()

	rawSlot := r.PathValue("slot")
	if rawSlot == "" {
		httputil.HandleError(w, "slot is required in URL params", http.StatusBadRequest)
		return
	}
	slot, err := strconv.ParseUint(rawSlot, 10, 64)
	if err != nil {
		httputil.HandleError(w, "invalid slot: "+err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.V1Alpha1Server.GetExecutionPayloadEnvelope(ctx, &ethpb.ExecutionPayloadEnvelopeRequest{
		Slot: primitives.Slot(slot),
	})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.NotFound:
				httputil.HandleError(w, st.Message(), http.StatusNotFound)
			case codes.InvalidArgument:
				httputil.HandleError(w, st.Message(), http.StatusBadRequest)
			default:
				httputil.HandleError(w, st.Message(), http.StatusInternalServerError)
			}
			return
		}
		httputil.HandleError(w, "could not get execution payload envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonEnvelope, err := structs.ExecutionPayloadEnvelopeFromConsensus(resp.Envelope)
	if err != nil {
		httputil.HandleError(w, "could not convert envelope to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httputil.WriteJson(w, &structs.GetValidatorExecutionPayloadEnvelopeResponse{
		Version: version.String(version.Gloas),
		Data:    jsonEnvelope,
	})
}
