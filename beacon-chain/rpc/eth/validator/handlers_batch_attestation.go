package validator

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) SubmitBatchAttestation(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitBatchAttestation")
	defer span.End()

	var source structs.BatchAttestation
	if err := json.NewDecoder(r.Body).Decode(&source); err != nil {
		if errors.Is(err, io.EOF) {
			httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		} else {
			httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		}
		return
	}

	batch, err := source.ToConsensus()
	if err != nil {
		httputil.HandleError(w, "Could not convert request batch attestation to consensus: "+err.Error(), http.StatusBadRequest)
		return
	}
	if s.V1Alpha1Server == nil {
		httputil.HandleError(w, "Batch attestation server is unavailable", http.StatusInternalServerError)
		return
	}
	if _, err = s.V1Alpha1Server.ProposeBatchAttestation(ctx, batch); err != nil {
		if st, ok := status.FromError(err); ok {
			httputil.HandleError(w, st.Message(), batchAttestationHTTPStatus(st.Code()))
			return
		}
		httputil.HandleError(w, "Could not submit batch attestation: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func batchAttestationHTTPStatus(code codes.Code) int {
	switch code {
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
