package beacon_api

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
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

func TestGetExecutionPayloadEnvelope_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	jsonEnvelope, err := structs.ExecutionPayloadEnvelopeFromConsensus(envelope)
	require.NoError(t, err)

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/validator/execution_payload_envelope/100",
		gomock.Any(),
	).SetArg(
		2,
		structs.GetValidatorExecutionPayloadEnvelopeResponse{
			Version: "gloas",
			Data:    jsonEnvelope,
		},
	).Return(nil)

	client := &beaconApiValidatorClient{handler: handler}
	resp, err := client.getExecutionPayloadEnvelope(t.Context(), 100)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, primitives.BuilderIndex(42), resp.BuilderIndex)
	assert.Equal(t, primitives.Slot(100), resp.Payload.SlotNumber)
	assert.DeepEqual(t, envelope.BeaconBlockRoot, resp.BeaconBlockRoot)
}

func TestGetExecutionPayloadEnvelope_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().Get(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(errors.New("not found"))

	client := &beaconApiValidatorClient{handler: handler}
	_, err := client.getExecutionPayloadEnvelope(t.Context(), 999)
	assert.ErrorContains(t, "not found", err)
}

func TestGetExecutionPayloadEnvelope_NilData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().Get(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).SetArg(
		2,
		structs.GetValidatorExecutionPayloadEnvelopeResponse{
			Version: "gloas",
			Data:    nil,
		},
	).Return(nil)

	client := &beaconApiValidatorClient{handler: handler}
	_, err := client.getExecutionPayloadEnvelope(t.Context(), 100)
	assert.ErrorContains(t, "execution payload envelope data is nil", err)
}

func TestPublishExecutionPayloadEnvelope_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	envelope := testProtoEnvelope()
	signed := &ethpb.SignedExecutionPayloadEnvelope{
		Message:   envelope,
		Signature: bytesutil.PadTo([]byte("sig"), 96),
	}

	jsonEnvelope, err := structs.SignedExecutionPayloadEnvelopeFromConsensus(signed)
	require.NoError(t, err)
	expectedBody, err := json.Marshal(jsonEnvelope)
	require.NoError(t, err)

	handler := mock.NewMockJsonRestHandler(ctrl)
	handler.EXPECT().Post(
		gomock.Any(),
		"/eth/v1/beacon/execution_payload_envelope",
		nil,
		bytes.NewBuffer(expectedBody),
		nil,
	).Return(nil)

	client := &beaconApiValidatorClient{handler: handler}
	resp, err := client.publishExecutionPayloadEnvelope(t.Context(), signed)
	require.NoError(t, err)
	require.NotNil(t, resp)
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
	handler.EXPECT().Post(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(errors.New("server error"))

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
