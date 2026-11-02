package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
)

const forkEndpoint = "/eth/v1/beacon/states/head/fork"

func TestGetFork_Nominal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	stateForkResponseJson := structs.GetStateForkResponse{}
	handler := mock.NewMockHandler(ctrl)

	expected := structs.GetStateForkResponse{
		Data: &structs.Fork{
			PreviousVersion: "0x1",
			CurrentVersion:  "0x2",
			Epoch:           "3",
		},
	}

	ctx := t.Context()

	handler.EXPECT().Get(
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
		handler: handler,
	}

	fork, err := validatorClient.fork(ctx)
	require.NoError(t, err)
	assert.DeepEqual(t, &expected, fork)
}

func TestGetFork_Invalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockHandler(ctrl)

	ctx := t.Context()

	handler.EXPECT().Get(
		gomock.Any(),
		forkEndpoint,
		gomock.Any(),
	).Return(
		errors.New("custom error"),
	).Times(1)

	validatorClient := beaconApiValidatorClient{
		handler: handler,
	}

	_, err := validatorClient.fork(ctx)
	require.ErrorContains(t, "custom error", err)
}

const headersEndpoint = "/eth/v1/beacon/headers"

func TestGetHeaders_Nominal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	blockHeadersResponseJson := structs.GetBlockHeadersResponse{}
	handler := mock.NewMockHandler(ctrl)

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

	handler.EXPECT().Get(
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
		handler: handler,
	}

	headers, err := validatorClient.headers(ctx)
	require.NoError(t, err)
	assert.DeepEqual(t, &expected, headers)
}

func TestGetHeaders_Invalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	handler := mock.NewMockHandler(ctrl)

	ctx := t.Context()

	handler.EXPECT().Get(
		gomock.Any(),
		headersEndpoint,
		gomock.Any(),
	).Return(
		errors.New("custom error"),
	).Times(1)

	validatorClient := beaconApiValidatorClient{
		handler: handler,
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

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().Post(
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

	validatorClient := &beaconApiValidatorClient{handler: handler}
	liveness, err := validatorClient.liveness(ctx, 42, indexes)

	require.NoError(t, err)
	assert.DeepEqual(t, &expected, liveness)
}

func TestGetLiveness_Invalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().Post(
		gomock.Any(),
		livenessEndpoint,
		nil,
		gomock.Any(),
		gomock.Any(),
	).Return(
		errors.New("custom error"),
	).Times(1)

	validatorClient := &beaconApiValidatorClient{handler: handler}
	_, err := validatorClient.liveness(ctx, 42, nil)

	require.ErrorContains(t, "custom error", err)
}

const syncingEndpoint = "/eth/v1/node/syncing"

func TestSyncedNodeStatus(t *testing.T) {
	t.Run("multi host: first node syncing, second synced", func(t *testing.T) {
		statusPerHost := []structs.SyncStatusResponseData{{IsSyncing: true}, {IsSyncing: false}}
		validatorClient := beaconApiValidatorClient{handler: newSyncStatusNodesHandler(t, statusPerHost)}

		status, err := validatorClient.syncedNodeStatus(t.Context())
		require.NoError(t, err)
		assert.Equal(t, false, status.IsSyncing)
	})
}

// newSyncStatusNodesHandler starts one beacon node test server per entry of statusPerHost,
// each answering `/eth/v1/node/syncing` with the given status, and returns a multi-host
// REST handler querying them in that order.
func newSyncStatusNodesHandler(t *testing.T, statusPerHost []structs.SyncStatusResponseData) rest.Handler {
	hosts := make([]string, 0, len(statusPerHost))
	for _, status := range statusPerHost {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != syncingEndpoint {
				http.NotFound(w, r)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			resp := structs.SyncStatusResponse{Data: &status}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		t.Cleanup(server.Close)
		hosts = append(hosts, server.URL)
	}

	provider, err := rest.NewRestConnectionProvider(strings.Join(hosts, ","))
	require.NoError(t, err)

	return provider.Handler()
}
