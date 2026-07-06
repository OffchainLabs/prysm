package rest

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

type testResponse struct {
	Host string `json:"host"`
}

// newTestHandler builds a *handler pointing at the given base URL.
func newTestHandler(host string) *handler {
	return newHandler(http.Client{Timeout: 5 * time.Second}, host)
}

func multi(t *testing.T, hosts ...string) *multiHandler {
	handlers := make([]*handler, len(hosts))
	for i, h := range hosts {
		handlers[i] = newTestHandler(h)
	}
	mh, err := newMultiHandler(handlers)
	require.NoError(t, err)
	return mh
}

func TestNewMultiHandler_rejectsEmpty(t *testing.T) {
	_, err := newMultiHandler(nil)
	require.ErrorContains(t, "at least one handler", err)
}

func jsonServer(t *testing.T, delay time.Duration, status int, hits *int32) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", api.JsonMediaType)
		w.WriteHeader(status)
		if status >= 200 && status < 300 {
			_, _ = w.Write([]byte(`{"host":"` + r.Host + `"}`))
		} else {
			_, _ = w.Write([]byte(`{"code":` + http.StatusText(status) + `,"message":"err"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMultiHandler_Get_PrimaryServesWithoutFanout(t *testing.T) {
	var primaryHits, secondaryHits int32
	primary := jsonServer(t, 0, http.StatusOK, &primaryHits)
	secondary := jsonServer(t, 0, http.StatusOK, &secondaryHits)

	mh := multi(t, primary.URL, secondary.URL)
	var resp testResponse
	require.NoError(t, mh.Get(context.Background(), "/x", &resp))
	assert.NotEqual(t, "", resp.Host)
	assert.Equal(t, int32(1), atomic.LoadInt32(&primaryHits), "primary should serve the read")
	assert.Equal(t, int32(0), atomic.LoadInt32(&secondaryHits), "secondary should not be contacted while the primary is healthy")
}

func TestMultiHandler_Get_SucceedsWhenOnlyOneHealthy(t *testing.T) {
	bad := jsonServer(t, 0, http.StatusInternalServerError, nil)
	good := jsonServer(t, 0, http.StatusOK, nil)

	mh := multi(t, bad.URL, good.URL)
	var resp testResponse
	require.NoError(t, mh.Get(context.Background(), "/x", &resp))
	assert.NotEqual(t, "", resp.Host)
}

func TestMultiHandler_Get_AllFail(t *testing.T) {
	bad1 := jsonServer(t, 0, http.StatusInternalServerError, nil)
	bad2 := jsonServer(t, 0, http.StatusBadGateway, nil)

	mh := multi(t, bad1.URL, bad2.URL)
	var resp testResponse
	err := mh.Get(context.Background(), "/x", &resp)
	require.NotNil(t, err)
}

func TestMultiHandler_Post_BroadcastToAll(t *testing.T) {
	var hits1, hits2 int32
	bn1 := jsonServer(t, 0, http.StatusOK, &hits1)
	bn2 := jsonServer(t, 0, http.StatusOK, &hits2)

	mh := multi(t, bn1.URL, bn2.URL)
	var resp testResponse
	body := bytes.NewBufferString(`{"a":1}`)
	require.NoError(t, mh.Post(context.Background(), "/submit", nil, body, &resp))

	// Both nodes must receive the submission. The second may arrive slightly
	// after Post returns (it runs on a detached context), so allow a brief wait.
	require.NoError(t, waitFor(func() bool {
		return atomic.LoadInt32(&hits1) == 1 && atomic.LoadInt32(&hits2) == 1
	}))
}

func TestMultiHandler_Post_SucceedsIfAnyAccepts(t *testing.T) {
	bad := jsonServer(t, 0, http.StatusInternalServerError, nil)
	good := jsonServer(t, 0, http.StatusOK, nil)

	mh := multi(t, bad.URL, good.URL)
	var resp testResponse
	body := bytes.NewBufferString(`{"a":1}`)
	require.NoError(t, mh.Post(context.Background(), "/submit", nil, body, &resp))
}

func TestMultiHandler_Post_AllFail(t *testing.T) {
	bad1 := jsonServer(t, 0, http.StatusInternalServerError, nil)
	bad2 := jsonServer(t, 0, http.StatusBadGateway, nil)

	mh := multi(t, bad1.URL, bad2.URL)
	var resp testResponse
	body := bytes.NewBufferString(`{"a":1}`)
	err := mh.Post(context.Background(), "/submit", nil, body, &resp)
	require.NotNil(t, err)
}

func TestMultiHandler_GetStatusCode_AnyReady(t *testing.T) {
	syncing := jsonServer(t, 0, http.StatusPartialContent, nil) // 206
	ready := jsonServer(t, 0, http.StatusOK, nil)               // 200

	mh := multi(t, syncing.URL, ready.URL)
	code, err := mh.GetStatusCode(context.Background(), "/eth/v1/node/health")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)
}

func TestMultiHandler_GetStatusCode_NoneReady(t *testing.T) {
	syncing := jsonServer(t, 0, http.StatusPartialContent, nil)         // 206
	unavailable := jsonServer(t, 0, http.StatusServiceUnavailable, nil) // 503

	mh := multi(t, syncing.URL, unavailable.URL)
	code, err := mh.GetStatusCode(context.Background(), "/eth/v1/node/health")
	require.NoError(t, err)
	assert.NotEqual(t, http.StatusOK, code)
}

// In a mixed fleet where one node accepts SSZ and another rejects it with 406,
// PostSSZ must surface the 406 so the caller can re-broadcast via JSON, rather
// than hiding it behind the accepting node's success.
func TestMultiHandler_PostSSZ_MixedFleetSurfaces406(t *testing.T) {
	accepts := sszServer(t, "ok") // 200 octet-stream
	rejects := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", api.JsonMediaType)
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = w.Write([]byte(`{"code":406,"message":"needs json"}`))
	}))
	defer rejects.Close()

	mh := multi(t, accepts.URL, rejects.URL)
	_, _, err := mh.PostSSZ(context.Background(), "/publish", nil, bytes.NewBufferString("ssz"))
	require.NotNil(t, err)
	assert.Equal(t, true, errors.Is(err, &httputil.DefaultJsonError{Code: http.StatusNotAcceptable}),
		"a node's 406 must surface so the caller falls back to JSON")
}

func TestRestConnectionProvider_UsesMultiHandler(t *testing.T) {
	provider, err := NewRestConnectionProvider("http://localhost:3500")
	require.NoError(t, err)
	_, ok := provider.Handler().(*multiHandler)
	assert.Equal(t, true, ok, "single endpoint should still use a *multiHandler (it short-circuits internally)")

	multiProvider, err := NewRestConnectionProvider("http://host1:3500,http://host2:3500")
	require.NoError(t, err)
	_, ok = multiProvider.Handler().(*multiHandler)
	assert.Equal(t, true, ok, "multiple endpoints should use a *multiHandler")
}

// waitFor polls cond for up to a second.
func waitFor(cond func() bool) error {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	if cond() {
		return nil
	}
	return context.DeadlineExceeded
}
