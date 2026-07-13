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
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"go.uber.org/mock/gomock"
)

func testSubmitBuilderPreferencesRequest() *ethpb.SubmitBuilderPreferencesRequest {
	return &ethpb.SubmitBuilderPreferencesRequest{
		ValidatorPubkey: bytes.Repeat([]byte{0xde}, 48),
		Request: &ethpb.BuilderPreferencesRequestV1{
			Preferences: &ethpb.BuilderPreferencesV1{MaxExecutionPayment: 1_000_000},
			Auth: &ethpb.SignedRequestAuthV1{
				Message: &ethpb.RequestAuthV1{
					Data: []byte("https://builder.example.com"),
					Slot: 32,
				},
				Signature: bytes.Repeat([]byte{0x01}, 96),
			},
		},
	}
}

func builderPreferencesEndpointFor(in *ethpb.SubmitBuilderPreferencesRequest) string {
	return "/eth/v1/validator/builder_preferences/" + hexutil.Encode(in.ValidatorPubkey)
}

func TestSubmitBuilderPreferences_Valid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	in := testSubmitBuilderPreferencesRequest()
	sszBody, err := in.Request.MarshalSSZ()
	require.NoError(t, err)

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		builderPreferencesEndpointFor(in),
		map[string]string{"Eth-Consensus-Version": "gloas"},
		bytes.NewBuffer(sszBody),
	).Return(nil, nil, nil).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	require.NoError(t, client.submitBuilderPreferences(t.Context(), in))
}

func TestSubmitBuilderPreferences_FallsBackToJSONOn415(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	in := testSubmitBuilderPreferencesRequest()
	jsonBody, err := json.Marshal(structs.BuilderPreferencesRequestV1FromConsensus(in.Request))
	require.NoError(t, err)

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		builderPreferencesEndpointFor(in),
		gomock.Any(),
		gomock.Any(),
	).Return(nil, nil, &httputil.DefaultJsonError{Code: http.StatusUnsupportedMediaType, Message: "unsupported media type"}).Times(1)
	handler.EXPECT().Post(
		gomock.Any(),
		builderPreferencesEndpointFor(in),
		map[string]string{"Eth-Consensus-Version": "gloas"},
		bytes.NewBuffer(jsonBody),
		nil,
	).Return(nil).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	require.NoError(t, client.submitBuilderPreferences(t.Context(), in))
}

func TestSubmitBuilderPreferences_NonMediaTypeErrorNoFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	in := testSubmitBuilderPreferencesRequest()
	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().PostSSZ(
		gomock.Any(),
		builderPreferencesEndpointFor(in),
		map[string]string{"Eth-Consensus-Version": "gloas"},
		gomock.Any(),
	).Return(nil, nil, errors.New("foo error")).Times(1)

	client := &beaconApiValidatorClient{handler: handler}
	err := client.submitBuilderPreferences(t.Context(), in)
	assert.ErrorContains(t, "foo error", err)
}

func TestSubmitBuilderPreferences_Nil(t *testing.T) {
	client := &beaconApiValidatorClient{}
	err := client.submitBuilderPreferences(t.Context(), &ethpb.SubmitBuilderPreferencesRequest{})
	assert.ErrorContains(t, "is nil", err)
}
