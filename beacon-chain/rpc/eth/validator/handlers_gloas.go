package validator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/feed/operation"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
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

// SubmitProposerPreferences submits signed proposer preferences to the node's pool.
func (s *Server) SubmitProposerPreferences(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.SubmitProposerPreferences")
	defer span.End()

	if slots.ToEpoch(s.TimeFetcher.CurrentSlot()) < params.BeaconConfig().GloasForkEpoch {
		httputil.HandleError(w, "Proposer preferences are not supported before the gloas fork", http.StatusBadRequest)
		return
	}

	versionHeader := r.Header.Get(api.VersionHeader)
	if versionHeader == "" {
		httputil.HandleError(w, api.VersionHeader+" header is required", http.StatusBadRequest)
		return
	}

	var req []*structs.SignedProposerPreferences
	err := json.NewDecoder(r.Body).Decode(&req)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req) == 0 {
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	}

	currentEpoch := slots.ToEpoch(s.TimeFetcher.CurrentSlot())
	var failures []*server.IndexedError

	for i, sp := range req {
		consensus, err := sp.ToConsensus()
		if err != nil {
			failures = append(failures, &server.IndexedError{
				Index:   i,
				Message: "Unable to decode SignedProposerPreferences: " + err.Error(),
			})
			continue
		}
		if consensus.Message == nil {
			failures = append(failures, &server.IndexedError{
				Index:   i,
				Message: "Message is nil",
			})
			continue
		}
		proposalSlot := consensus.Message.ProposalSlot
		if slots.ToEpoch(proposalSlot) != currentEpoch+1 {
			failures = append(failures, &server.IndexedError{
				Index:   i,
				Message: fmt.Sprintf("proposal_slot must be in the next epoch: slot %d currentEpoch %d", proposalSlot, currentEpoch),
			})
			continue
		}
		if s.ProposerPreferencesCache.Has(proposalSlot) {
			continue
		}
		s.OperationNotifier.OperationFeed().Send(&feed.Event{
			Type: operation.ProposerPreferencesReceived,
			Data: &operation.ProposerPreferencesReceivedData{
				Preferences: consensus,
			},
		})
		s.ProposerPreferencesCache.Add(proposalSlot, consensus)
		if err := s.Broadcaster.Broadcast(ctx, consensus); err != nil {
			log.WithError(err).Error("Could not broadcast signed proposer preferences")
		}
	}

	if len(failures) > 0 {
		failuresErr := &server.IndexedErrorContainer{
			Code:     http.StatusBadRequest,
			Message:  server.ErrIndexedValidationFail,
			Failures: failures,
		}
		httputil.WriteError(w, failuresErr)
	}
}
