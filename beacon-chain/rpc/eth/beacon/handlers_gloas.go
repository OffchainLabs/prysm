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
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetExecutionPayloadEnvelope retrieves a full execution payload envelope by beacon block root.
func (s *Server) GetExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.GetExecutionPayloadEnvelope")
	defer span.End()

	blockID := r.PathValue("block_id")
	if blockID == "" {
		httputil.HandleError(w, "block_id is required in URL params", http.StatusBadRequest)
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

// PublishSignedExecutionPayloadBid broadcasts a signed execution payload bid to the P2P network.
func (s *Server) PublishSignedExecutionPayloadBid(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "beacon.PublishSignedExecutionPayloadBid")
	defer span.End()

	if shared.IsSyncing(ctx, w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	versionHeader := r.Header.Get(api.VersionHeader)
	if versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
		return
	}

	var signedBid *eth.SignedExecutionPayloadBid
	if httputil.IsRequestSsz(r) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			httputil.HandleError(w, "Could not read request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		signedBid = &eth.SignedExecutionPayloadBid{}
		if err := signedBid.UnmarshalSSZ(body); err != nil {
			httputil.HandleError(w, "Could not unmarshal SSZ: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		var jsonBid structs.SignedExecutionPayloadBid
		if err := json.NewDecoder(r.Body).Decode(&jsonBid); err != nil {
			if errors.Is(err, io.EOF) {
				httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
				return
			}
			httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		var err error
		signedBid, err = jsonBid.ToConsensus()
		if err != nil {
			httputil.HandleError(w, "Could not convert bid to consensus type: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if err := s.Broadcaster.Broadcast(ctx, signedBid); err != nil {
		httputil.HandleError(w, "Could not broadcast execution payload bid: "+err.Error(), http.StatusInternalServerError)
		return
	}
}
