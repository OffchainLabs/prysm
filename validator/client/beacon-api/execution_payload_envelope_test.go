package beacon_api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"github.com/OffchainLabs/prysm/v7/validator/client/cache"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

func testProtoEnvelope() *ethpb.ExecutionPayloadEnvelope {
	return &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadGloas{
			ParentHash:    bytesutil.PadTo([]byte("parent"), 32),
			FeeRecipient:  bytesutil.PadTo([]byte("fee"), 20),
			StateRoot:     bytesutil.PadTo([]byte("state"), 32),
			ReceiptsRoot:  bytesutil.PadTo([]byte("receipts"), 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    bytesutil.PadTo([]byte("randao"), 32),
			BaseFeePerGas: bytesutil.PadTo([]byte{1}, 32),
			BlockHash:     bytesutil.PadTo([]byte("blockhash"), 32),
			Transactions:  [][]byte{},
			Withdrawals:   []*enginev1.Withdrawal{},
			SlotNumber:    primitives.Slot(100),
		},
		ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
		BuilderIndex:          primitives.BuilderIndex(42),
		BeaconBlockRoot:       bytesutil.PadTo([]byte("beacon-root"), 32),
		ParentBeaconBlockRoot: bytesutil.PadTo([]byte("parent-beacon-root"), 32),
	}
}

func TestGetExecutionPayloadEnvelope_CachedHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockHandler(ctrl)
	// No Get expectation: cache hit must skip the HTTP call.

	envelope := testProtoEnvelope()
	client := &beaconApiValidatorClient{
		handler:       handler,
		envelopeCache: cache.NewExecutionPayloadEnvelopeCache(),
	}
	client.envelopeCache.Add(100, envelope, nil, nil)

	full, err := client.getExecutionPayloadEnvelope(t.Context(), 100, bytesutil.ToBytes32(envelope.BeaconBlockRoot))
	require.NoError(t, err)
	require.NotNil(t, full)
	assert.Equal(t, primitives.BuilderIndex(42), full.BuilderIndex)

	// Peek must leave the entry in the cache so the publish path can read blob data.
	cached, _, _ := client.envelopeCache.Peek(100)
	require.NotNil(t, cached)
}

// A cached envelope for a different block root must not be returned for the requested block.
func TestGetExecutionPayloadEnvelope_CachedRootMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockHandler(ctrl)
	// No Get expectation: the root mismatch must error before any HTTP call.

	envelope := testProtoEnvelope()
	client := &beaconApiValidatorClient{
		handler:       handler,
		envelopeCache: cache.NewExecutionPayloadEnvelopeCache(),
	}
	client.envelopeCache.Add(100, envelope, nil, nil)

	full, err := client.getExecutionPayloadEnvelope(t.Context(), 100, [32]byte{})
	require.ErrorContains(t, "does not match requested block", err)
	require.IsNil(t, full)
}

// Stateful: on a local cache miss the VC fetches the full envelope from the BN.
func TestGetExecutionPayloadEnvelope_StatefulFetchesEnvelope(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	body, err := envelope.MarshalSSZ()
	require.NoError(t, err)

	root := bytesutil.ToBytes32(envelope.BeaconBlockRoot)
	respHeader := http.Header{}
	respHeader.Set("Content-Type", api.OctetStreamMediaType)

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().GetSSZ(
		gomock.Any(),
		fmt.Sprintf("/eth/v1/validator/execution_payload_envelopes/100/%s", hexutil.Encode(root[:])),
	).Return(body, respHeader, nil)

	client := &beaconApiValidatorClient{handler: handler, envelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
	full, err := client.getExecutionPayloadEnvelope(t.Context(), 100, root)
	require.NoError(t, err)
	require.NotNil(t, full)
	assert.Equal(t, primitives.BuilderIndex(42), full.BuilderIndex)
}

// Stateful publish (local cache miss) sends the bare signed envelope with
// Eth-Blob-Data-Included: false.
func TestPublishExecutionPayloadEnvelope_StatefulSendsBareEnvelope(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signed := &ethpb.SignedExecutionPayloadEnvelope{Message: testProtoEnvelope(), Signature: bytesutil.PadTo([]byte("sig"), 96)}
	expectedBody, err := signed.MarshalSSZ()
	require.NoError(t, err)

	expectedHeaders := map[string]string{
		api.VersionHeader:          version.String(version.Gloas),
		api.BlobDataIncludedHeader: "false",
	}
	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v1/beacon/execution_payload_envelopes",
		expectedHeaders,
		bytes.NewBuffer(expectedBody),
	).Return(nil)

	client := &beaconApiValidatorClient{handler: handler, envelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
	resp, err := client.publishExecutionPayloadEnvelope(t.Context(), signed)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestPublishExecutionPayloadEnvelope_StatefulJSONFallbackOn415(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signed := &ethpb.SignedExecutionPayloadEnvelope{Message: testProtoEnvelope(), Signature: bytesutil.PadTo([]byte("sig"), 96)}
	msg, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(signed)
	require.NoError(t, err)
	expectedJSON, err := json.Marshal(msg)
	require.NoError(t, err)

	expectedHeaders := map[string]string{
		api.VersionHeader:          version.String(version.Gloas),
		api.BlobDataIncludedHeader: "false",
	}
	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(&httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType, Message: "unsupported media type"})
	handler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/execution_payload_envelopes",
		expectedHeaders,
		bytes.NewBuffer(expectedJSON),
		nil,
	).Return(nil)

	client := &beaconApiValidatorClient{handler: handler, envelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
	resp, err := client.publishExecutionPayloadEnvelope(t.Context(), signed)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestPublishExecutionPayloadEnvelope_StatelessSendsContents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	signed := &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}
	blob := bytesutil.PadTo([]byte("blob"), 131072)
	proof := bytesutil.PadTo([]byte("proof"), 48)

	expectedBody, err := (&ethpb.SignedExecutionPayloadEnvelopeContents{
		SignedExecutionPayloadEnvelope: signed,
		KzgProofs:                      [][]byte{proof},
		Blobs:                          [][]byte{blob},
	}).MarshalSSZ()
	require.NoError(t, err)

	expectedHeaders := map[string]string{
		api.VersionHeader:          version.String(version.Gloas),
		api.BlobDataIncludedHeader: "true",
	}
	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v1/beacon/execution_payload_envelopes",
		expectedHeaders,
		bytes.NewBuffer(expectedBody),
	).Return(nil)

	client := &beaconApiValidatorClient{
		handler:       handler,
		stateless:     true,
		envelopeCache: cache.NewExecutionPayloadEnvelopeCache(),
	}
	client.envelopeCache.Add(primitives.Slot(envelope.Payload.SlotNumber), envelope, [][]byte{blob}, [][]byte{proof})

	resp, err := client.publishExecutionPayloadEnvelope(t.Context(), signed)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Cache must be drained after publish.
	cached, _, _ := client.envelopeCache.Peek(primitives.Slot(envelope.Payload.SlotNumber))
	assert.Equal(t, (*ethpb.ExecutionPayloadEnvelope)(nil), cached)
}

func TestPublishExecutionPayloadEnvelope_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	signed := &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(errors.New("server error"))

	client := &beaconApiValidatorClient{handler: handler, envelopeCache: cache.NewExecutionPayloadEnvelopeCache()}
	client.envelopeCache.Add(primitives.Slot(envelope.Payload.SlotNumber), envelope, nil, nil)

	_, err := client.publishExecutionPayloadEnvelope(t.Context(), signed)
	assert.ErrorContains(t, "server error", err)
}

func TestEnvelopeRoundTrip(t *testing.T) {
	envelope := testProtoEnvelope()
	jsonEnvelope, err := structs.ExecutionPayloadEnvelopeFromConsensus(envelope)
	require.NoError(t, err)

	roundTripped, err := jsonEnvelope.ToConsensus()
	require.NoError(t, err)

	assert.Equal(t, envelope.BuilderIndex, roundTripped.BuilderIndex)
	assert.Equal(t, envelope.Payload.SlotNumber, roundTripped.Payload.SlotNumber)
	assert.DeepEqual(t, envelope.BeaconBlockRoot, roundTripped.BeaconBlockRoot)
	assert.Equal(t, hexutil.Encode(envelope.Payload.BlockHash), hexutil.Encode(roundTripped.Payload.BlockHash))
}
