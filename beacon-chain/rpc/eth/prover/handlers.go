package prover

import (
	"encoding/json"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// SubmitExecutionProof handles POST requests to /eth/v1/prover/execution_proofs.
// It receives execution proofs from provers and broadcasts them to the P2P network.
func (s *Server) SubmitExecutionProof(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "prover.SubmitExecutionProof")
	defer span.End()

	var req structs.SignedExecutionProof
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	proof, err := req.ToConsensus()
	if err != nil {
		httputil.HandleError(w, "Invalid execution proof: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.Broadcaster.Broadcast(ctx, proof); err != nil {
		httputil.HandleError(w, "Could not broadcast execution proof: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.WithFields(map[string]any{
		"validatorIndex":        proof.GetValidatorIndex(),
		"proofType":             hexutil.Encode(proof.Message.ProofType),
		"proofDataSize":         len(proof.Message.ProofData),
		"newPayloadRequestRoot": hexutil.Encode(proof.Message.PublicInput.NewPayloadRequestRoot),
		"signature":             hexutil.Encode(proof.Signature),
	}).Info("Gossiped new execution proof")

	w.WriteHeader(http.StatusOK)
}
