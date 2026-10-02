package prover

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v7/config/features"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/sirupsen/logrus"
)

// SubmitExecutionProof broadcasts a prover's signed execution proof envelope on
// the `execution_proof` gossip topic.
func (s *Server) SubmitExecutionProof(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "prover.SubmitExecutionProof")
	defer span.End()

	if !features.Get().EnableExecutionProofs {
		httputil.HandleError(w, "Node is not execution proof-aware, restart it with --zkvm", http.StatusServiceUnavailable)
		return
	}

	var req structs.SignedExecutionProofEnvelope
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	signed, err := req.ToConsensus()
	if err != nil {
		httputil.HandleError(w, "Invalid execution proof envelope: "+err.Error(), http.StatusBadRequest)
		return
	}
	envelope, err := blocks.NewROSignedExecutionProofEnvelope(signed)
	if err != nil {
		httputil.HandleError(w, "Invalid execution proof envelope: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.Broadcaster.Broadcast(ctx, signed); err != nil {
		httputil.HandleError(w, "Could not broadcast execution proof: "+err.Error(), http.StatusInternalServerError)
		return
	}

	blockRoot := envelope.BeaconBlockRoot()
	log.WithFields(logrus.Fields{
		"blockRoot": fmt.Sprintf("%#x", bytesutil.Trunc(blockRoot[:])),
		"proofType": envelope.ProofType(),
		"prover":    signed.ValidatorIndex,
		"proofSize": len(signed.Message.ProofData),
	}).Info("Broadcast execution proof")

	w.WriteHeader(http.StatusOK)
}

// GetExecutionProofs returns the verified execution proofs this node holds for
// a beacon block, keyed by proof type.
func (s *Server) GetExecutionProofs(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "prover.GetExecutionProofs")
	defer span.End()

	rawRoot := r.PathValue("block_root")
	blockRoot, valid := shared.ValidateHex(w, "block_root", rawRoot, fieldparams.RootLength)
	if !valid {
		return
	}

	proofs := s.ExecutionProofCache.Get([32]byte(blockRoot))
	data := make([]*structs.SignedExecutionProofEnvelope, 0, len(proofs))
	for _, proof := range proofs {
		data = append(data, structs.SignedExecutionProofEnvelopeFromConsensus(proof.SignedExecutionProofEnvelope))
	}

	httputil.WriteJson(w, &struct {
		Data []*structs.SignedExecutionProofEnvelope `json:"data"`
	}{Data: data})
}
