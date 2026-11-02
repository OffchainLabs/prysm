package beacon_api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/validator/client/beacon-api/mock"
	"go.uber.org/mock/gomock"
)

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
	handler.EXPECT().Request(
		gomock.Any(),
		livenessEndpoint,
		nil,
		bytes.NewBuffer(marshalledIndexes),
		&livenessResponseJson,
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
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

	// Real multi-host handler.
	newLivenessNode := func(t *testing.T, isLive bool) string {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			resp := structs.GetLivenessResponse{Data: []*structs.Liveness{{Index: "1", IsLive: isLive}}}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("encode: %v", err)
			}
		}))
		t.Cleanup(server.Close)

		return server.URL
	}

	t.Run("multi host: first node not live, second live", func(t *testing.T) {
		provider, err := rest.NewRestConnectionProvider(newLivenessNode(t, false) + "," + newLivenessNode(t, true))
		require.NoError(t, err)

		validatorClient := &beaconApiValidatorClient{handler: provider.Handler()}
		liveness, err := validatorClient.liveness(t.Context(), 42, []string{"1"})
		require.NoError(t, err)
		require.Equal(t, 1, len(liveness.Data))
		assert.Equal(t, true, liveness.Data[0].IsLive)
	})

	t.Run("multi host: hung node does not block the read", func(t *testing.T) {
		previous := livenessFallbackWait
		livenessFallbackWait = 100 * time.Millisecond
		t.Cleanup(func() { livenessFallbackWait = previous })

		release := make(chan struct{})
		hung := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}))
		t.Cleanup(hung.Close)
		t.Cleanup(func() { close(release) }) // Runs before hung.Close, which waits for in-flight requests.

		provider, err := rest.NewRestConnectionProvider(hung.URL + "," + newLivenessNode(t, false))
		require.NoError(t, err)

		validatorClient := &beaconApiValidatorClient{handler: provider.Handler()}
		start := time.Now()
		liveness, err := validatorClient.liveness(t.Context(), 42, []string{"1"})
		require.NoError(t, err)
		require.Equal(t, 1, len(liveness.Data))
		assert.Equal(t, false, liveness.Data[0].IsLive)
		assert.Equal(t, true, time.Since(start) < time.Second, "the hung node must be cut off at the fallback wait")
	})
}

func TestGetLiveness_Invalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := t.Context()

	handler := mock.NewMockHandler(ctrl)
	handler.EXPECT().Request(
		gomock.Any(),
		livenessEndpoint,
		nil,
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
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
		statusPerHost := []structs.SyncStatusResponseData{{HeadSlot: "10", IsSyncing: true}, {HeadSlot: "3200", IsSyncing: false}}
		validatorClient := beaconApiValidatorClient{handler: newSyncStatusNodesHandler(t, statusPerHost)}

		status, err := validatorClient.syncedNodeStatus(t.Context())
		require.NoError(t, err)
		assert.Equal(t, false, status.IsSyncing)
		assert.Equal(t, "3200", status.HeadSlot)
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

func TestGetIsOptimistic(t *testing.T) {
	t.Run("multi host: first node optimistic, second not optimistic", func(t *testing.T) {
		statusPerHost := []structs.SyncStatusResponseData{{IsOptimistic: true}, {}}
		validatorClient := beaconApiValidatorClient{handler: newSyncStatusNodesHandler(t, statusPerHost)}

		isOptimistic, err := validatorClient.isOptimistic(t.Context())
		require.NoError(t, err)
		assert.Equal(t, false, isOptimistic)
	})
}
