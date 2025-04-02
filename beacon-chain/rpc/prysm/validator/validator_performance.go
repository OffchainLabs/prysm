package validator

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/core"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

// GetPerformance is an HTTP handler for GetPerformance.
func (s *Server) GetPerformance(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.GetPerformance")
	defer span.End()

	var req structs.GetValidatorPerformanceRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	computed, rpcError := s.CoreService.ComputeValidatorPerformance(
		ctx,
		&ethpb.ValidatorPerformanceRequest{
			PublicKeys: req.PublicKeys,
			Indices:    req.Indices,
		},
	)
	if rpcError != nil {
		handleHTTPError(w, "Could not compute validator performance: "+rpcError.Err.Error(), core.ErrorReasonToHTTP(rpcError.Reason))
		return
	}
	response := &structs.GetValidatorPerformanceResponse{
		PublicKeys:                    computed.PublicKeys,
		CorrectlyVotedSource:          computed.CorrectlyVotedSource,
		CorrectlyVotedTarget:          computed.CorrectlyVotedTarget, // In altair, when this is true then the attestation was definitely included.
		CorrectlyVotedHead:            computed.CorrectlyVotedHead,
		CurrentEffectiveBalances:      computed.CurrentEffectiveBalances,
		BalancesBeforeEpochTransition: computed.BalancesBeforeEpochTransition,
		BalancesAfterEpochTransition:  computed.BalancesAfterEpochTransition,
		MissingValidators:             computed.MissingValidators,
		InactivityScores:              computed.InactivityScores, // Only populated in Altair
	}
	httputil.WriteJson(w, response)
}

func handleHTTPError(w http.ResponseWriter, message string, code int) {
	errJson := &httputil.DefaultJsonError{
		Message: message,
		Code:    code,
	}
	httputil.WriteError(w, errJson)
}
