package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OffchainLabs/prysm/v6/api/server/structs"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
)

const forkEndpoint = "/eth/v1/beacon/states/head/fork"

func TestGetFork_Nominal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	stateForkResponseJson := structs.GetStateForkResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	expected := structs.GetStateForkResponse{
		Data: &structs.Fork{
			PreviousVersion: "0x1",
			CurrentVersion:  "0x2",
			Epoch:           "3",
		},
	}

	ctx := t.Context()

	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		forkEndpoint,
		&stateForkResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		expected,
	).Times(1)

	validatorClient := beaconApiValidatorClient{
		jsonRestHandler: jsonRestHandler,
	}

	fork, err := validatorClient.fork(ctx)
	require.NoError(t, err)
	assert.DeepEqual(t, &expected, fork)
}

func TestGetFork_Invalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	ctx := t.Context()

	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		forkEndpoint,
		gomock.Any(),
	).Return(
		errors.New("custom error"),
	).Times(1)

	validatorClient := beaconApiValidatorClient{
		jsonRestHandler: jsonRestHandler,
	}

	_, err := validatorClient.fork(ctx)
	require.ErrorContains(t, "custom error", err)
}

const headersEndpoint = "/eth/v1/beacon/headers"

func TestGetHeaders_Nominal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	blockHeadersResponseJson := structs.GetBlockHeadersResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	expected := structs.GetBlockHeadersResponse{
		Data: []*structs.SignedBeaconBlockHeaderContainer{
			{
				Header: &structs.SignedBeaconBlockHeader{
					Message: &structs.BeaconBlockHeader{
						Slot: "42",
					},
				},
			},
		},
	}

	ctx := t.Context()

	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		headersEndpoint,
		&blockHeadersResponseJson,
	).Return(
		nil,
	).SetArg(
		2,
		expected,
	).Times(1)

	validatorClient := beaconApiValidatorClient{
		jsonRestHandler: jsonRestHandler,
	}

	headers, err := validatorClient.headers(ctx)
	require.NoError(t, err)
	assert.DeepEqual(t, &expected, headers)
}

func TestGetHeaders_Invalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	ctx := t.Context()

	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		headersEndpoint,
		gomock.Any(),
	).Return(
		errors.New("custom error"),
	).Times(1)

	validatorClient := beaconApiValidatorClient{
		jsonRestHandler: jsonRestHandler,
	}

	_, err := validatorClient.headers(ctx)
	require.ErrorContains(t, "custom error", err)
}

const livenessEndpoint = "/eth/v1/validator/liveness/42"

func TestGetLiveness_Nominal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	livenessResponseJson := structs.GetLivenessResponse{}

	indexes := []string{"1", "2"}
	marshalledIndexes, err := json.Marshal(indexes)
	require.NoError(t, err)

	expected := structs.GetLivenessResponse{
		Data: []*structs.Liveness{
			{
				Index:  "1",
				IsLive: true,
			},
			{
				Index:  "2",
				IsLive: false,
			},
		},
	}

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		livenessEndpoint,
		nil,
		bytes.NewBuffer(marshalledIndexes),
		&livenessResponseJson,
	).SetArg(
		4,
		expected,
	).Return(
		nil,
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	liveness, err := validatorClient.liveness(ctx, 42, indexes)

	require.NoError(t, err)
	assert.DeepEqual(t, &expected, liveness)
}

func TestGetLiveness_Invalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)
	jsonRestHandler.EXPECT().Post(
		gomock.Any(),
		livenessEndpoint,
		nil,
		gomock.Any(),
		gomock.Any(),
	).Return(
		errors.New("custom error"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	_, err := validatorClient.liveness(ctx, 42, nil)

	require.ErrorContains(t, "custom error", err)
}

const syncingEndpoint = "/eth/v1/node/syncing"

func TestGetIsSyncing_Nominal(t *testing.T) {
	testCases := []struct {
		name      string
		isSyncing bool
	}{
		{
			name:      "Syncing",
			isSyncing: true,
		},
		{
			name:      "Not syncing",
			isSyncing: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			syncingResponseJson := structs.SyncStatusResponse{}
			jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

			expected := structs.SyncStatusResponse{
				Data: &structs.SyncStatusResponseData{
					IsSyncing: testCase.isSyncing,
				},
			}

			ctx := t.Context()

			jsonRestHandler.EXPECT().Get(
				gomock.Any(),
				syncingEndpoint,
				&syncingResponseJson,
			).Return(
				nil,
			).SetArg(
				2,
				expected,
			).Times(1)

			validatorClient := beaconApiValidatorClient{
				jsonRestHandler: jsonRestHandler,
			}

			isSyncing, err := validatorClient.isSyncing(ctx)
			require.NoError(t, err)
			assert.Equal(t, testCase.isSyncing, isSyncing)
		})
	}
}

func TestGetIsSyncing_Invalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	syncingResponseJson := structs.SyncStatusResponse{}
	jsonRestHandler := mock.NewMockJsonRestHandler(ctrl)

	ctx := t.Context()

	jsonRestHandler.EXPECT().Get(
		gomock.Any(),
		syncingEndpoint,
		&syncingResponseJson,
	).Return(
		errors.New("custom error"),
	).Times(1)

	validatorClient := beaconApiValidatorClient{
		jsonRestHandler: jsonRestHandler,
	}

	isSyncing, err := validatorClient.isSyncing(ctx)
	assert.Equal(t, true, isSyncing)
	assert.ErrorContains(t, "failed to get syncing status", err)
}
