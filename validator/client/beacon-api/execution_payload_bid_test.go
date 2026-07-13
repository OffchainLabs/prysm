package beacon_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

func testSignedExecutionPayloadBid() *ethpb.SignedExecutionPayloadBid {
	return &ethpb.SignedExecutionPayloadBid{
		Message: &ethpb.ExecutionPayloadBid{
			ParentBlockHash:       bytes.Repeat([]byte{0x11}, 32),
			ParentBlockRoot:       bytes.Repeat([]byte{0x22}, 32),
			BlockHash:             bytes.Repeat([]byte{0x33}, 32),
			PrevRandao:            bytes.Repeat([]byte{0x44}, 32),
			FeeRecipient:          bytes.Repeat([]byte{0xab}, 20),
			GasLimit:              30_000_000,
			BuilderIndex:          7,
			Slot:                  32,
			Value:                 1_000,
			ExecutionPayment:      500,
			BlobKzgCommitments:    [][]byte{},
			ExecutionRequestsRoot: bytes.Repeat([]byte{0x55}, 32),
		},
		Signature: bytes.Repeat([]byte{0x01}, 96),
	}
}

func TestSubmitSignedExecutionPayloadBid_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bid := testSignedExecutionPayloadBid()
	sszBody, err := bid.MarshalSSZ()
	require.NoError(t, err)

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		executionPayloadBidEndpoint,
		map[string]string{"Eth-Consensus-Version": "gloas"},
		bytes.NewBuffer(sszBody),
	).Return(nil, nil, nil).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	require.NoError(t, client.submitSignedExecutionPayloadBid(t.Context(), bid))
}

func TestSubmitSignedExecutionPayloadBid_FallsBackToJSONOn415(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bid := testSignedExecutionPayloadBid()
	jsonBody, err := json.Marshal(structs.SignedExecutionPayloadBidFromConsensus(bid))
	require.NoError(t, err)

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		executionPayloadBidEndpoint,
		gomock.Any(),
		gomock.Any(),
	).Return(nil, nil, &httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType, Message: "unsupported media type"}).Times(1)
	handler.EXPECT().Post(
		gomock.Any(),
		executionPayloadBidEndpoint,
		map[string]string{"Eth-Consensus-Version": "gloas"},
		bytes.NewBuffer(jsonBody),
		nil,
	).Return(nil).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	require.NoError(t, client.submitSignedExecutionPayloadBid(t.Context(), bid))
}

func TestSubmitSignedExecutionPayloadBid_NonMediaTypeErrorNoFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		executionPayloadBidEndpoint,
		map[string]string{"Eth-Consensus-Version": "gloas"},
		gomock.Any(),
	).Return(nil, nil, errors.New("foo error")).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	err := client.submitSignedExecutionPayloadBid(t.Context(), testSignedExecutionPayloadBid())
	assert.ErrorContains(t, "foo error", err)
}

func TestSubmitSignedExecutionPayloadBid_Nil(t *testing.T) {
	client := &beaconApiValidatorClient{}
	err := client.submitSignedExecutionPayloadBid(t.Context(), &ethpb.SignedExecutionPayloadBid{})
	assert.ErrorContains(t, "is nil", err)
}
