package validator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/rpc/eth/shared"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ProduceBlockV4 requests a beacon node to produce a valid GLOAS block.
// This is the GLOAS-specific block production endpoint that returns a block
// containing a signed execution payload bid instead of the full payload.
//
// The execution payload envelope is cached by the beacon node and can be
// retrieved via GetExecutionPayloadEnvelope.
//
// Endpoint: GET /eth/v4/validator/blocks/{slot}
func (s *Server) ProduceBlockV4(w http.ResponseWriter, r *http.Request) {
	_, span := trace.StartSpan(r.Context(), "validator.ProduceBlockV4")
	defer span.End()

	if shared.IsSyncing(r.Context(), w, s.SyncChecker, s.HeadFetcher, s.TimeFetcher, s.OptimisticModeFetcher) {
		return
	}

	// Parse path parameters
	segments := strings.Split(r.URL.Path, "/")
	rawSlot := segments[len(segments)-1]

	slot, valid := shared.ValidateUint(w, "slot", rawSlot)
	if !valid {
		return
	}

	// Parse query parameters
	rawRandaoReveal := r.URL.Query().Get("randao_reveal")
	rawGraffiti := r.URL.Query().Get("graffiti")
	rawSkipRandaoVerification := r.URL.Query().Get("skip_randao_verification")

	var bbFactor *wrapperspb.UInt64Value
	rawBbFactor, bbValue, ok := shared.UintFromQuery(w, r, "builder_boost_factor", false)
	if !ok {
		return
	}
	if rawBbFactor != "" {
		bbFactor = &wrapperspb.UInt64Value{Value: bbValue}
	}

	// Parse randao reveal
	var randaoReveal []byte
	if rawSkipRandaoVerification == "true" {
		// TODO: Use infinite signature constant
		randaoReveal = make([]byte, 96)
	} else {
		// TODO: Decode randao reveal from hex
		_ = rawRandaoReveal
	}

	// Parse graffiti
	var graffiti []byte
	if rawGraffiti != "" {
		// TODO: Decode graffiti from hex
	}

	// TODO: Implement GLOAS-specific block production
	//
	// This handler should:
	// 1. Verify the slot is in the GLOAS fork
	// 2. Call v1alpha1 server's getGloasBeaconBlock
	// 3. Format response with GLOAS-specific headers
	// 4. Return the block (the envelope is cached server-side)

	_ = bbFactor
	_ = graffiti
	_ = randaoReveal
	_ = slot

	httputil.HandleError(w, "ProduceBlockV4 not yet implemented", http.StatusNotImplemented)
}

// handleProduceGloasV4 handles the response formatting for GLOAS blocks.
func handleProduceGloasV4(w http.ResponseWriter, isSSZ bool, block *eth.BeaconBlockGloas, payloadValue, consensusBlockValue string) {
	// TODO: Implement GLOAS response handling
	//
	// Similar to handleProduceFuluV3 but for GLOAS blocks.
	// The response should NOT include the execution payload envelope,
	// as that is retrieved separately.

	if isSSZ {
		// TODO: SSZ serialize the GLOAS block
		httputil.HandleError(w, "SSZ response not yet implemented for GLOAS", http.StatusNotImplemented)
		return
	}

	// JSON response
	// TODO: Convert GLOAS block to JSON struct
	resp := &structs.ProduceBlockV3Response{
		Version:                 version.String(version.Gloas),
		ExecutionPayloadBlinded: false, // GLOAS blocks don't have blinded concept in same way
		ExecutionPayloadValue:   payloadValue,
		ConsensusBlockValue:     consensusBlockValue,
		Data:                    nil, // TODO: Marshal block to JSON
	}

	httputil.WriteJson(w, resp)
}

// GetExecutionPayloadEnvelope retrieves a cached execution payload envelope.
// Validators call this after receiving a GLOAS block to get the envelope
// they need to sign and broadcast.
//
// Endpoint: GET /eth/v1/validator/execution_payload_envelope/{slot}/{builder_index}
func (s *Server) GetExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.ExecutionPayloadEnvelope")
	defer span.End()

	// Parse path parameters
	segments := strings.Split(r.URL.Path, "/")
	if len(segments) < 2 {
		httputil.HandleError(w, "missing slot and builder_index in path", http.StatusBadRequest)
		return
	}

	rawSlot := segments[len(segments)-2]
	rawBuilderIndex := segments[len(segments)-1]

	slot, valid := shared.ValidateUint(w, "slot", rawSlot)
	if !valid {
		return
	}

	builderIndex, err := strconv.ParseUint(rawBuilderIndex, 10, 64)
	if err != nil {
		httputil.HandleError(w, errors.Wrap(err, "invalid builder_index").Error(), http.StatusBadRequest)
		return
	}

	// Build gRPC request
	req := &eth.ExecutionPayloadEnvelopeRequest{
		Slot:         primitives.Slot(slot),
		BuilderIndex: primitives.BuilderIndex(builderIndex),
	}

	// TODO: The V1Alpha1Server needs to implement the ExecutionPayloadEnvelope method
	// from the BeaconNodeValidatorServer interface. Currently it's defined but the
	// interface may need updating to include this method.
	//
	// Once implemented, uncomment:
	// resp, err := s.V1Alpha1Server.ExecutionPayloadEnvelope(ctx, req)
	// if err != nil {
	//     // Map gRPC error codes to HTTP status codes
	//     if status.Code(err) == codes.NotFound {
	//         httputil.HandleError(w, err.Error(), http.StatusNotFound)
	//     } else {
	//         httputil.HandleError(w, err.Error(), http.StatusInternalServerError)
	//     }
	//     return
	// }
	//
	// // Format and return response
	// // - Support both JSON and SSZ based on Accept header
	// // - Set version header
	// w.Header().Set(api.VersionHeader, version.String(version.Gloas))
	// httputil.WriteJson(w, &structs.GetExecutionPayloadEnvelopeResponse{
	//     Version: version.String(version.Gloas),
	//     Data:    envelopeProtoToJSON(resp.Envelope),
	// })

	_ = ctx
	_ = req

	httputil.HandleError(w, "ExecutionPayloadEnvelope not yet implemented", http.StatusNotImplemented)
}

// PublishExecutionPayloadEnvelope broadcasts a signed execution payload envelope.
// Validators call this after signing the envelope to broadcast it to the network.
//
// Endpoint: POST /eth/v1/beacon/execution_payload_envelope
func (s *Server) PublishExecutionPayloadEnvelope(w http.ResponseWriter, r *http.Request) {
	ctx, span := trace.StartSpan(r.Context(), "validator.PublishExecutionPayloadEnvelope")
	defer span.End()

	// Parse request body
	var signedEnvelope structs.SignedExecutionPayloadEnvelope
	if err := json.NewDecoder(r.Body).Decode(&signedEnvelope); err != nil {
		httputil.HandleError(w, errors.Wrap(err, "failed to decode request body").Error(), http.StatusBadRequest)
		return
	}

	// TODO: Convert JSON struct to proto
	// protoEnvelope, err := signedEnvelope.ToProto()
	// if err != nil {
	//     httputil.HandleError(w, err.Error(), http.StatusBadRequest)
	//     return
	// }

	// TODO: Call gRPC server
	// _, err = s.V1Alpha1Server.PublishExecutionPayloadEnvelope(ctx, protoEnvelope)
	// if err != nil {
	//     // Handle different error types (validation errors vs internal errors)
	//     httputil.HandleError(w, err.Error(), http.StatusBadRequest)
	//     return
	// }

	_ = ctx
	_ = signedEnvelope

	httputil.HandleError(w, "PublishExecutionPayloadEnvelope not yet implemented", http.StatusNotImplemented)
}

// ExecutionPayloadEnvelopeJSON represents the JSON structure for an execution payload envelope.
// This is used for REST API serialization.
type ExecutionPayloadEnvelopeJSON struct {
	Payload            json.RawMessage `json:"payload"`
	ExecutionRequests  json.RawMessage `json:"execution_requests"`
	BuilderIndex       string          `json:"builder_index"`
	BeaconBlockRoot    string          `json:"beacon_block_root"`
	Slot               string          `json:"slot"`
	BlobKzgCommitments []string        `json:"blob_kzg_commitments"`
	StateRoot          string          `json:"state_root"`
}

// SignedExecutionPayloadEnvelopeJSON represents the JSON structure for a signed envelope.
type SignedExecutionPayloadEnvelopeJSON struct {
	Message   *ExecutionPayloadEnvelopeJSON `json:"message"`
	Signature string                        `json:"signature"`
}

// ExecutionPayloadEnvelopeResponseJSON is the response wrapper for envelope retrieval.
type ExecutionPayloadEnvelopeResponseJSON struct {
	Version string                        `json:"version"`
	Data    *ExecutionPayloadEnvelopeJSON `json:"data"`
}

// envelopeProtoToJSON converts a proto envelope to JSON representation.
func envelopeProtoToJSON(envelope *eth.ExecutionPayloadEnvelope) (*ExecutionPayloadEnvelopeJSON, error) {
	// TODO: Implement conversion
	//
	// Convert each field:
	// - payload: Marshal ExecutionPayloadDeneb to JSON
	// - execution_requests: Marshal to JSON
	// - builder_index: Convert uint64 to string
	// - beacon_block_root: Hex encode
	// - slot: Convert uint64 to string
	// - blob_kzg_commitments: Hex encode each
	// - state_root: Hex encode

	return nil, fmt.Errorf("envelopeProtoToJSON not yet implemented")
}

// envelopeJSONToProto converts a JSON envelope to proto representation.
func envelopeJSONToProto(envelope *ExecutionPayloadEnvelopeJSON) (*eth.ExecutionPayloadEnvelope, error) {
	// TODO: Implement conversion
	//
	// Parse each field:
	// - payload: Unmarshal from JSON
	// - execution_requests: Unmarshal from JSON
	// - builder_index: Parse uint64 from string
	// - beacon_block_root: Hex decode
	// - slot: Parse uint64 from string
	// - blob_kzg_commitments: Hex decode each
	// - state_root: Hex decode

	return nil, fmt.Errorf("envelopeJSONToProto not yet implemented")
}
