package beacon_api

import (
	"context"
	"fmt"
	neturl "net/url"

	"github.com/OffchainLabs/prysm/v7/api/apiutil"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
)

// getExecutionPayloadEnvelope retrieves the execution payload envelope for the given
// slot and builder index. This is called by validators after receiving a GLOAS block
// to get the envelope they need to sign and broadcast.
//
// REST endpoint: GET /eth/v1/validator/execution_payload_envelope/{slot}/{builder_index}
func (c *beaconApiValidatorClient) getExecutionPayloadEnvelope(
	ctx context.Context,
	slot primitives.Slot,
	builderIndex primitives.BuilderIndex,
) (*ethpb.ExecutionPayloadEnvelope, error) {
	// TODO: Implement execution payload envelope retrieval
	//
	// Implementation steps:
	// 1. Build URL with slot and builder_index path parameters
	// 2. Make GET request (support both JSON and SSZ based on Accept header)
	// 3. Parse response
	// 4. Convert to proto type
	// 5. Return envelope

	queryUrl := apiutil.BuildURL(
		fmt.Sprintf("/eth/v1/validator/execution_payload_envelope/%d/%d", slot, builderIndex),
		neturl.Values{},
	)

	_ = queryUrl

	return nil, errors.New("getExecutionPayloadEnvelope not yet implemented")
}

// publishExecutionPayloadEnvelope broadcasts a signed execution payload envelope
// to the beacon node for P2P gossip.
//
// REST endpoint: POST /eth/v1/beacon/execution_payload_envelope
func (c *beaconApiValidatorClient) publishExecutionPayloadEnvelope(
	ctx context.Context,
	envelope *ethpb.SignedExecutionPayloadEnvelope,
) (*empty.Empty, error) {
	// TODO: Implement envelope publishing
	//
	// Implementation steps:
	// 1. Convert proto envelope to JSON struct
	// 2. Serialize to JSON (or SSZ based on Content-Type)
	// 3. POST to /eth/v1/beacon/execution_payload_envelope
	// 4. Handle response (200 = success, 4xx = validation error)

	if envelope == nil || envelope.Message == nil {
		return nil, errors.New("signed envelope cannot be nil")
	}

	return nil, errors.New("publishExecutionPayloadEnvelope not yet implemented")
}

// signedEnvelopeToJSON converts a proto SignedExecutionPayloadEnvelope to its JSON representation.
func signedEnvelopeToJSON(envelope *ethpb.SignedExecutionPayloadEnvelope) (any, error) {
	// TODO: Implement conversion from proto to JSON struct
	//
	// Convert each field:
	// - message.payload: Marshal ExecutionPayloadDeneb to JSON
	// - message.execution_requests: Marshal to JSON
	// - message.builder_index: Format as decimal string
	// - message.beacon_block_root: Hex encode with 0x prefix
	// - message.slot: Format as decimal string
	// - message.blob_kzg_commitments: Hex encode each with 0x prefix
	// - message.state_root: Hex encode with 0x prefix
	// - signature: Hex encode with 0x prefix

	return nil, errors.New("signedEnvelopeToJSON not yet implemented")
}

// envelopeJSONToProto converts a JSON execution payload envelope to proto type.
func envelopeJSONToProto(jsonEnvelope any) (*ethpb.ExecutionPayloadEnvelope, error) {
	// TODO: Implement conversion from JSON to proto
	//
	// Parse each field:
	// - payload: Unmarshal ExecutionPayloadDeneb from JSON
	// - execution_requests: Unmarshal from JSON
	// - builder_index: Parse uint64 from decimal string
	// - beacon_block_root: Hex decode (strip 0x prefix)
	// - slot: Parse uint64 from decimal string
	// - blob_kzg_commitments: Hex decode each (strip 0x prefix)
	// - state_root: Hex decode (strip 0x prefix)

	return nil, errors.New("envelopeJSONToProto not yet implemented")
}

// processGloasBlock handles GLOAS block responses from the beacon node.
// This is called from processBlockJSONResponse when the version is "gloas".
func processGloasBlock(jsonBlock any) (*ethpb.GenericBeaconBlock, error) {
	// TODO: Implement GLOAS block processing
	//
	// Convert the JSON block to proto BeaconBlockGloas:
	// 1. Parse BeaconBlockGloas fields
	// 2. Parse BeaconBlockBodyGloas with signed_execution_payload_bid
	// 3. Parse payload_attestations
	// 4. Return GenericBeaconBlock with Gloas variant

	return nil, errors.New("processGloasBlock not yet implemented")
}

// processBlockSSZResponseGloas handles SSZ-encoded GLOAS block responses.
func processBlockSSZResponseGloas(data []byte) (*ethpb.GenericBeaconBlock, error) {
	// TODO: Implement SSZ deserialization for GLOAS blocks
	//
	// Note: GLOAS blocks don't have a "blinded" variant in the same way
	// as previous forks because the execution payload is always separate.

	return nil, errors.New("processBlockSSZResponseGloas not yet implemented")
}
