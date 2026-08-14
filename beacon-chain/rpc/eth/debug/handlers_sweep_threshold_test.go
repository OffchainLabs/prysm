package debug

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/sweepthreshold"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const (
	sourceAddress = "0x1111111111111111111111111111111111111111"
	validatorKey  = "0x" +
		"222222222222222222222222222222222222222222222222" +
		"222222222222222222222222222222222222222222222222"
)

func requestBody(threshold string) string {
	return `{"requests":[{"source_address":"` + sourceAddress +
		`","validator_pubkey":"` + validatorKey +
		`","threshold":"` + threshold + `"}]}`
}

func postRequests(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/prysm/v1/debug/beacon/sweep_threshold_requests", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.PostSweepThresholdRequests(w, req)
	return w
}

func TestPostSweepThresholdRequests_Queues(t *testing.T) {
	pool := sweepthreshold.NewMockPool()
	s := &Server{MockSweepThresholdPool: pool}

	w := postRequests(t, s, requestBody("64000000000"))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, pool.Pending())

	var resp SetSweepThresholdRequestsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 1, len(resp.Data))
	require.Equal(t, 1, resp.Pending)
	require.Equal(t, "64000000000", resp.Data[0].Threshold)
	require.Equal(t, sourceAddress, resp.Data[0].SourceAddress)
	require.Equal(t, validatorKey, resp.Data[0].ValidatorPubkey)

	// Queued requests are handed to the proposer exactly once.
	require.Equal(t, 1, len(pool.Drain()))
	require.Equal(t, 0, pool.Pending())
}

// TestPostSweepThresholdRequests_EchoesOnlyWhatWasQueued guards against the response
// echoing the whole queue, which makes two calls look like one call with extra entries.
func TestPostSweepThresholdRequests_EchoesOnlyWhatWasQueued(t *testing.T) {
	pool := sweepthreshold.NewMockPool()
	s := &Server{MockSweepThresholdPool: pool}

	require.Equal(t, http.StatusOK, postRequests(t, s, requestBody("64000000000")).Code)
	w := postRequests(t, s, requestBody("128000000000"))
	require.Equal(t, http.StatusOK, w.Code)

	var resp SetSweepThresholdRequestsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 1, len(resp.Data), "data should hold only this call's request")
	require.Equal(t, "128000000000", resp.Data[0].Threshold)
	require.Equal(t, 2, resp.Pending, "pending should count the whole queue")
}

func TestPostSweepThresholdRequests_BadInput(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
	}{
		{"malformed json", `{`, http.StatusBadRequest},
		{"no requests", `{"requests":[]}`, http.StatusBadRequest},
		{
			"bad source address",
			`{"requests":[{"source_address":"0xdead","validator_pubkey":"` + validatorKey + `","threshold":"1"}]}`,
			http.StatusBadRequest,
		},
		{
			"bad pubkey",
			`{"requests":[{"source_address":"` + sourceAddress + `","validator_pubkey":"0xdead","threshold":"1"}]}`,
			http.StatusBadRequest,
		},
		{"bad threshold", requestBody("not-a-number"), http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := sweepthreshold.NewMockPool()
			w := postRequests(t, &Server{MockSweepThresholdPool: pool}, tt.body)
			require.Equal(t, tt.code, w.Code)
			require.Equal(t, 0, pool.Pending())
		})
	}
}

func TestPostSweepThresholdRequests_OverLimitConflicts(t *testing.T) {
	pool := sweepthreshold.NewMockPool()
	s := &Server{MockSweepThresholdPool: pool}

	limit := params.BeaconConfig().MaxSetSweepThresholdRequestsPerPayload
	for range limit {
		require.Equal(t, http.StatusOK, postRequests(t, s, requestBody("64000000000")).Code)
	}

	w := postRequests(t, s, requestBody("64000000000"))
	require.Equal(t, http.StatusConflict, w.Code)
	require.Equal(t, int(limit), pool.Pending())
}

func TestSweepThresholdRequests_GetAndDelete(t *testing.T) {
	pool := sweepthreshold.NewMockPool()
	s := &Server{MockSweepThresholdPool: pool}
	require.Equal(t, http.StatusOK, postRequests(t, s, requestBody("64000000000")).Code)

	getReq := httptest.NewRequest(http.MethodGet, "/prysm/v1/debug/beacon/sweep_threshold_requests", nil)
	getW := httptest.NewRecorder()
	s.GetSweepThresholdRequests(getW, getReq)
	require.Equal(t, http.StatusOK, getW.Code)

	var resp SetSweepThresholdRequestsResponse
	require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &resp))
	require.Equal(t, 1, len(resp.Data))
	// Reading does not consume the queue.
	require.Equal(t, 1, pool.Pending())

	delReq := httptest.NewRequest(http.MethodDelete, "/prysm/v1/debug/beacon/sweep_threshold_requests", nil)
	delW := httptest.NewRecorder()
	s.DeleteSweepThresholdRequests(delW, delReq)
	require.Equal(t, http.StatusOK, delW.Code)
	require.Equal(t, 0, pool.Pending())
}

func TestSweepThresholdRequests_NoPoolUnavailable(t *testing.T) {
	s := &Server{}

	w := httptest.NewRecorder()
	s.PostSweepThresholdRequests(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(nil)))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	w = httptest.NewRecorder()
	s.GetSweepThresholdRequests(w, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	w = httptest.NewRecorder()
	s.DeleteSweepThresholdRequests(w, httptest.NewRequest(http.MethodDelete, "/", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}
