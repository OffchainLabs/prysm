package beacon_api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestWaitForChainStart_ValidGenesis(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()
	genesisResponseJson := structs.GetGenesisResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/beacon/genesis",
		&genesisResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetGenesisResponse{
			Data: &structs.Genesis{
				GenesisTime:           "1234",
				GenesisValidatorsRoot: "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
			},
		},
	).Times(1)

	genesisProvider := beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler}
	validatorClient := beaconApiValidatorClient{genesisProvider: &genesisProvider}
	resp, err := validatorClient.WaitForChainStart(ctx, &emptypb.Empty{})
	assert.NoError(t, err)

	require.NotNil(t, resp)
	assert.Equal(t, true, resp.Started)
	assert.Equal(t, uint64(1234), resp.GenesisTime)

	expectedRoot, err := hexutil.Decode("0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2")
	require.NoError(t, err)
	assert.DeepEqual(t, expectedRoot, resp.GenesisValidatorsRoot)
}

func TestWaitForChainStart_BadGenesis(t *testing.T) {
	testCases := []struct {
		name         string
		data         *structs.Genesis
		errorMessage string
	}{
		{
			name:         "nil data",
			data:         nil,
			errorMessage: "failed to get genesis data",
		},
		{
			name: "invalid time",
			data: &structs.Genesis{
				GenesisTime:           "foo",
				GenesisValidatorsRoot: "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
			},
			errorMessage: "failed to parse genesis time: foo",
		},
		{
			name: "invalid root",
			data: &structs.Genesis{
				GenesisTime:           "1234",
				GenesisValidatorsRoot: "0xzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
			},
			errorMessage: "failed to decode genesis validators root: ",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := t.Context()
			genesisResponseJson := structs.GetGenesisResponse{}
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				"/eth/v1/beacon/genesis",
				&genesisResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				structs.GetGenesisResponse{
					Data: testCase.data,
				},
			).Times(1)

			genesisProvider := beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler}
			validatorClient := beaconApiValidatorClient{genesisProvider: &genesisProvider}
			_, err := validatorClient.WaitForChainStart(ctx, &emptypb.Empty{})
			assert.ErrorContains(t, testCase.errorMessage, err)
		})
	}
}

func TestWaitForChainStart_JsonResponseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()
	genesisResponseJson := structs.GetGenesisResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/beacon/genesis",
		&genesisResponseJson,
	).Return(
		errors.New("some specific json error"),
	).Times(1)

	genesisProvider := beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler}
	validatorClient := beaconApiValidatorClient{genesisProvider: &genesisProvider}
	_, err := validatorClient.WaitForChainStart(ctx, &emptypb.Empty{})
	assert.ErrorContains(t, "failed to get genesis data", err)
	assert.ErrorContains(t, "some specific json error", err)
}

// For WaitForChainStart, error 404 just means that we keep retrying until the information becomes available
func TestWaitForChainStart_JsonResponseError404(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()
	genesisResponseJson := structs.GetGenesisResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	// First, mock a request that receives a 404 error (which means that the genesis data is not available yet)
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/beacon/genesis",
		&genesisResponseJson,
	).Return(
		&httputil.DefaultJsonError{
			Code:    http.StatusNotFound,
			Message: "404 error",
		},
	).Times(1)

	// After receiving a 404 error, mock a request that actually has genesis data available
	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		"/eth/v1/beacon/genesis",
		&genesisResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		structs.GetGenesisResponse{
			Data: &structs.Genesis{
				GenesisTime:           "1234",
				GenesisValidatorsRoot: "0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2",
			},
		},
	).Times(1)

	genesisProvider := beaconApiGenesisProvider{jsonRestHandler: jsonRestHandler}
	validatorClient := beaconApiValidatorClient{genesisProvider: &genesisProvider}
	resp, err := validatorClient.WaitForChainStart(ctx, &emptypb.Empty{})
	assert.NoError(t, err)

	require.NotNil(t, resp)
	assert.Equal(t, true, resp.Started)
	assert.Equal(t, uint64(1234), resp.GenesisTime)

	expectedRoot, err := hexutil.Decode("0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2")
	require.NoError(t, err)
	assert.DeepEqual(t, expectedRoot, resp.GenesisValidatorsRoot)
}
