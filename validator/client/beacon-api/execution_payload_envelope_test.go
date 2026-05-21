package beacon_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
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
		ExecutionRequests:     &enginev1.ExecutionRequests{},
		BuilderIndex:          primitives.BuilderIndex(42),
		BeaconBlockRoot:       bytesutil.PadTo([]byte("beacon-root"), 32),
		ParentBeaconBlockRoot: bytesutil.PadTo([]byte("parent-beacon-root"), 32),
	}
}

func sszHeader() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/octet-stream")
	return h
}

func jsonHeader() http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	return h
}

func TestGetExecutionPayloadEnvelope_CachedHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockJsonRestHandler(ctrl)
	// No Get expectation: cache hit must skip the HTTP call.

	envelope := testProtoEnvelope()
	client := &beaconApiValidatorClient{
		handler:       handler,
		envelopeCache: newExecutionPayloadEnvelopeCache(),
	}
	client.envelopeCache.Add(100, envelope, nil, nil)

	resp, err := client.getExecutionPayloadEnvelope(t.Context(), 100)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, primitives.BuilderIndex(42), resp.BuilderIndex)

	// Peek must leave the entry in the cache so the publish path can read blob data.
	cached, _, _ := client.envelopeCache.peek(100)
	require.NotNil(t, cached)
}

func TestGetExecutionPayloadEnvelope_SSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	body, err := envelope.MarshalSSZ()
	require.NoError(t, err)

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().GetSSZ(
		gomock.Any(),
		"/eth/v1/validator/execution_payload_envelope/100",
	).Return(body, sszHeader(), nil)

	client := &beaconApiValidatorClient{handler: handler}
	resp, err := client.getExecutionPayloadEnvelope(t.Context(), 100)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, primitives.BuilderIndex(42), resp.BuilderIndex)
	assert.Equal(t, primitives.Slot(100), resp.Payload.SlotNumber)
	assert.DeepEqual(t, envelope.BeaconBlockRoot, resp.BeaconBlockRoot)
}

func TestGetExecutionPayloadEnvelope_JSONFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	jsonEnvelope, err := structs.ExecutionPayloadEnvelopeFromConsensus(envelope)
	require.NoError(t, err)
	body, err := json.Marshal(structs.GetValidatorExecutionPayloadEnvelopeResponse{
		Version: "gloas",
		Data:    jsonEnvelope,
	})
	require.NoError(t, err)

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().GetSSZ(
		gomock.Any(),
		"/eth/v1/validator/execution_payload_envelope/100",
	).Return(body, jsonHeader(), nil)

	client := &beaconApiValidatorClient{handler: handler}
	resp, err := client.getExecutionPayloadEnvelope(t.Context(), 100)
	require.NoError(t, err)
	assert.Equal(t, primitives.BuilderIndex(42), resp.BuilderIndex)
}

func TestGetExecutionPayloadEnvelope_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().GetSSZ(
		gomock.Any(), gomock.Any(),
	).Return(nil, nil, errors.New("not found"))

	client := &beaconApiValidatorClient{handler: handler}
	_, err := client.getExecutionPayloadEnvelope(t.Context(), 999)
	assert.ErrorContains(t, "not found", err)
}

func TestGetExecutionPayloadEnvelope_NilData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	body, err := json.Marshal(structs.GetValidatorExecutionPayloadEnvelopeResponse{
		Version: "gloas",
		Data:    nil,
	})
	require.NoError(t, err)

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().GetSSZ(
		gomock.Any(), gomock.Any(),
	).Return(body, jsonHeader(), nil)

	client := &beaconApiValidatorClient{handler: handler}
	_, err = client.getExecutionPayloadEnvelope(t.Context(), 100)
	assert.ErrorContains(t, "execution payload envelope data is nil", err)
}

func TestPublishExecutionPayloadEnvelope_SSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	signed := &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}
	expectedBody, err := signed.MarshalSSZ()
	require.NoError(t, err)

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v1/beacon/execution_payload_envelope",
		nil,
		bytes.NewBuffer(expectedBody),
	).Return(nil, nil, nil)

	client := &beaconApiValidatorClient{handler: handler}
	resp, err := client.publishExecutionPayloadEnvelope(t.Context(), signed)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestPublishExecutionPayloadEnvelope_JSONFallbackOn406(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	signed := &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}
	jsonEnvelope, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(signed)
	require.NoError(t, err)
	expectedJSON, err := json.Marshal(jsonEnvelope)
	require.NoError(t, err)

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil, nil, &httputil.DefaultJsonError{Code: http.StatusNotAcceptable, Message: "not acceptable"})
	handler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/execution_payload_envelope",
		nil,
		bytes.NewBuffer(expectedJSON),
		nil,
	).Return(nil)

	client := &beaconApiValidatorClient{handler: handler}
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

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v1/beacon/execution_payload_envelope",
		nil,
		bytes.NewBuffer(expectedBody),
	).Return(nil, nil, nil)

	client := &beaconApiValidatorClient{
		handler:       handler,
		stateless:     true,
		envelopeCache: newExecutionPayloadEnvelopeCache(),
	}
	client.envelopeCache.Add(primitives.Slot(envelope.Payload.SlotNumber), envelope, [][]byte{blob}, [][]byte{proof})

	resp, err := client.publishExecutionPayloadEnvelope(t.Context(), signed)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Cache must be drained after publish.
	cached, _, _ := client.envelopeCache.peek(primitives.Slot(envelope.Payload.SlotNumber))
	assert.Equal(t, (*ethpb.ExecutionPayloadEnvelope)(nil), cached)
}

func TestPublishExecutionPayloadEnvelope_StatelessSendsContentsWithEmptyBlobs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	signed := &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}
	expectedBody, err := (&ethpb.SignedExecutionPayloadEnvelopeContents{
		SignedExecutionPayloadEnvelope: signed,
	}).MarshalSSZ()
	require.NoError(t, err)

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		"/eth/v1/beacon/execution_payload_envelope",
		nil,
		bytes.NewBuffer(expectedBody),
	).Return(nil, nil, nil)

	client := &beaconApiValidatorClient{
		handler:       handler,
		stateless:     true,
		envelopeCache: newExecutionPayloadEnvelopeCache(),
	}
	// Cache has an entry but no blobs (0-blob slot) — still send Contents.
	client.envelopeCache.Add(primitives.Slot(envelope.Payload.SlotNumber), envelope, nil, nil)

	resp, err := client.publishExecutionPayloadEnvelope(t.Context(), signed)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestPublishExecutionPayloadEnvelope_StatelessCacheMissErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	signed := &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}

	// No PostSSZ/Post expectation — must error before any HTTP call.
	handler := mock.NewMockJsonRestHandler(ctrl)
	client := &beaconApiValidatorClient{
		handler:       handler,
		stateless:     true,
		envelopeCache: newExecutionPayloadEnvelopeCache(),
	}

	_, err := client.publishExecutionPayloadEnvelope(t.Context(), signed)
	assert.ErrorContains(t, "stateless publish: envelope cache miss", err)
}

func TestPublishExecutionPayloadEnvelope_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	signed := &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil, nil, errors.New("server error"))

	client := &beaconApiValidatorClient{handler: handler}
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
