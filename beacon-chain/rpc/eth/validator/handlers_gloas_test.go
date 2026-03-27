package validator

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	mock2 "github.com/OffchainLabs/prysm/v7/testing/mock"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testEnvelope() *ethpb.ExecutionPayloadEnvelope {
	return &ethpb.ExecutionPayloadEnvelope{
		Payload: &enginev1.ExecutionPayloadDeneb{
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
		},
		ExecutionRequests: &enginev1.ExecutionRequests{},
		BuilderIndex:      primitives.BuilderIndex(42),
		BeaconBlockRoot:   bytesutil.PadTo([]byte("beacon-root"), 32),
		Slot:              primitives.Slot(100),
		StateRoot:         bytesutil.PadTo([]byte("envelope-state"), 32),
	}
}

func TestExecutionPayloadEnvelope_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	envelope := testEnvelope()

	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().GetExecutionPayloadEnvelope(
		gomock.Any(),
		&ethpb.ExecutionPayloadEnvelopeRequest{Slot: 100},
	).Return(&ethpb.ExecutionPayloadEnvelopeResponse{Envelope: envelope}, nil)

	server := &Server{V1Alpha1Server: v1alpha1Server}
	req := httptest.NewRequest(http.MethodGet, "/eth/v1/validator/execution_payload_envelope/100", nil)
	req.SetPathValue("slot", "100")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	server.ExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp structs.GetValidatorExecutionPayloadEnvelopeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, version.String(version.Gloas), w.Header().Get(api.VersionHeader))
	assert.Equal(t, version.String(version.Gloas), resp.Version)
	require.NotNil(t, resp.Data)
	assert.Equal(t, "42", resp.Data.BuilderIndex)
	assert.Equal(t, "100", resp.Data.Slot)
	assert.Equal(t, hexutil.Encode(envelope.BeaconBlockRoot), resp.Data.BeaconBlockRoot)
}

func TestExecutionPayloadEnvelope_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)

	v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
	v1alpha1Server.EXPECT().GetExecutionPayloadEnvelope(
		gomock.Any(), gomock.Any(),
	).Return(nil, status.Error(codes.NotFound, "not found for slot 999"))

	server := &Server{V1Alpha1Server: v1alpha1Server}
	req := httptest.NewRequest(http.MethodGet, "/eth/v1/validator/execution_payload_envelope/999", nil)
	req.SetPathValue("slot", "999")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	server.ExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestExecutionPayloadEnvelope_InvalidSlot(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/eth/v1/validator/execution_payload_envelope/abc", nil)
	req.SetPathValue("slot", "abc")
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	server.ExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExecutionPayloadEnvelope_MissingSlot(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/eth/v1/validator/execution_payload_envelope/", nil)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	server.ExecutionPayloadEnvelope(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
