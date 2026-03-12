package beacon

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetExecutionPayloadEnvelope retrieves a full execution payload envelope by beacon block root.
// The blinded envelope is fetched from the DB and the full execution payload is reconstructed
// from the EL via eth_getBlockByHash.
func (s *Server) GetExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetExecutionPayloadEnvelope")
	defer span.End()

	blockID := r.PathValue("block_root")
	if blockID == "" {
		httputil.HandleError(w, "block_root is required in URL params", http.StatusBadRequest)
		return
	}

	root, err := s.Blocker.BlockRoot(ctx, []byte(blockID))
	if !shared.WriteBlockRootFetchError(w, err) {
		return
	}

	blinded, err := s.BeaconDB.ExecutionPayloadEnvelope(ctx, root)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			httputil.HandleError(w, "execution payload envelope not found", http.StatusNotFound)
			return
		}
		httputil.HandleError(w, "could not retrieve execution payload envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}
	full, err := s.ExecutionReconstructor.ReconstructExecutionPayloadEnvelope(ctx, blinded)
	if err != nil {
		httputil.HandleError(w, "could not reconstruct execution payload envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(api.VersionHeader, version.String(version.Gloas))

	if httputil.RespondWithSsz(r) {
		sszBytes, err := full.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "could not marshal envelope to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, sszBytes)
		return
	}

	isOptimistic, err := s.OptimisticModeFetcher.IsOptimisticForRoot(ctx, root)
	if err != nil {
		httputil.HandleError(w, "could not check optimistic status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	finalized := s.FinalizationFetcher.IsFinalized(ctx, root)

	jsonEnvelope, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(full)
	if err != nil {
		httputil.HandleError(w, "could not convert envelope to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
	httputil.WriteJson(w, &structs.GetExecutionPayloadEnvelopeResponse{
		Version:             version.String(version.Gloas),
		ExecutionOptimistic: isOptimistic,
		Finalized:           finalized,
		Data:                jsonEnvelope,
	})
}

// PublishExecutionPayloadEnvelope broadcasts a signed execution payload envelope.
//
// Endpoint: POST /eth/v1/beacon/execution_payload_envelope
func (s *Server) PublishExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishExecutionPayloadEnvelope")
	defer span.End()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.HandleError(w, "could not read request body: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var jsonEnvelope structs.SignedExecutionPayloadEnvelope
	if err := json.Unmarshal(body, &jsonEnvelope); err != nil {
		httputil.HandleError(w, "could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	consensus, err := jsonEnvelope.ToConsensus()
	if err != nil {
		httputil.HandleError(w, "invalid signed execution payload envelope: "+err.Error(), http.StatusBadRequest)
		return
	}

	if _, err := s.V1Alpha1ValidatorServer.PublishExecutionPayloadEnvelope(ctx, consensus); err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument:
				httputil.HandleError(w, st.Message(), http.StatusBadRequest)
			default:
				httputil.HandleError(w, st.Message(), http.StatusInternalServerError)
			}
			return
		}
		httputil.HandleError(w, "could not publish execution payload envelope: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
