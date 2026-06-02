package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"google.golang.org/protobuf/types/known/emptypb"
)

// SignExecutionProof signs an execution proof with an active validator's private key.
func (s *Server) SignExecutionProof(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.rpc.SignExecutionProof")
	defer span.End()

	if s.validatorService == nil {
		httputil.HandleError(w, "Validator service not ready", http.StatusServiceUnavailable)
		return
	}

	if !s.walletInitialized {
		httputil.HandleError(w, "No wallet found", http.StatusServiceUnavailable)
		return
	}

	keyManager, err := s.validatorService.Keymanager()
	if err != nil {
		httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Decode request body
	var request structs.ExecutionProofRequest
	err = json.NewDecoder(r.Body).Decode(&request)
	switch {
	case errors.Is(err, io.EOF):
		httputil.HandleError(w, "No data submitted", http.StatusBadRequest)
		return
	case err != nil:
		httputil.HandleError(w, "Could not decode request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate request data
	if request.Data == nil {
		httputil.HandleError(w, "data is required", http.StatusBadRequest)
		return
	}

	if request.Data.PublicInput == nil {
		httputil.HandleError(w, "public_input is required", http.StatusBadRequest)
		return
	}

	// Get a random active validator
	selectedPubkey, selectedIndex, err := s.validatorService.RandomActiveValidator()
	if err != nil {
		httputil.HandleError(w, "Failed to select active validator: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Build the ExecutionProof proto message
	executionProof := &ethpb.ExecutionProof{
		ProofData: request.Data.ProofData,
		ProofType: []byte{request.Data.ProofType},
		PublicInput: &ethpb.PublicInput{
			NewPayloadRequestRoot: request.Data.PublicInput.NewPayloadRequestRoot,
		},
	}

	// Get current epoch for domain data
	genesisResponse, err := s.nodeClient.Genesis(ctx, &emptypb.Empty{})
	if err != nil {
		httputil.HandleError(w, fmt.Errorf("genesis: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	genesisTime := genesisResponse.GenesisTime.AsTime()
	currentSlot := slots.CurrentSlot(genesisTime)
	currentEpoch := slots.ToEpoch(currentSlot)

	// Get domain data for execution proof
	domainReq := &ethpb.DomainRequest{
		Epoch:  currentEpoch,
		Domain: params.BeaconConfig().DomainExecutionProof[:],
	}

	domainResp, err := s.beaconNodeValidatorClient.DomainData(ctx, domainReq)
	if err != nil {
		httputil.HandleError(w, fmt.Errorf("domain data: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	if domainResp == nil {
		httputil.HandleError(w, "Domain data is nil", http.StatusInternalServerError)
		return
	}

	// Compute the full signing root by combining the object root with the domain
	// This follows the same pattern as other signing operations (e.g., attestations)
	proofRoot, err := blocks.ExecutionProofHashTreeRoot(executionProof)
	if err != nil {
		httputil.HandleError(w, fmt.Errorf("execution proof hash tree root: %w", err).Error(), http.StatusInternalServerError)
		return
	}
	signingRoot, err := signing.ComputeSigningRootForRoot(proofRoot, domainResp.SignatureDomain)
	if err != nil {
		httputil.HandleError(w, fmt.Errorf("compute signing root: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	// Sign using keymanager
	signature, err := keyManager.Sign(ctx, &validatorpb.SignRequest{
		PublicKey:       selectedPubkey[:],
		SigningRoot:     signingRoot[:],
		SignatureDomain: domainResp.SignatureDomain,
		SigningSlot:     currentSlot,
	})
	if err != nil {
		httputil.HandleError(w, fmt.Errorf("sign: %w", err).Error(), http.StatusInternalServerError)
		return
	}

	// Build response
	response := &structs.SignedExecutionProofResponse{
		Data: &structs.SignedExecutionProof{
			Message: &structs.ExecutionProof{
				ProofData: request.Data.ProofData,
				ProofType: request.Data.ProofType,
				PublicInput: &structs.PublicInput{
					NewPayloadRequestRoot: request.Data.PublicInput.NewPayloadRequestRoot,
				},
			},
			ValidatorIndex: uint64(selectedIndex),
			Signature:      signature.Marshal(),
		},
	}

	httputil.WriteJson(w, response)
}
