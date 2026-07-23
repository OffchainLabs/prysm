package rest

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// Errors bubbled up to callers must never contain basic-auth credentials.
func TestHandler_ErrorsRedactCredentials(t *testing.T) {
	const secret = "fake-token-not-real"
	host := "https://eth:" + secret + "@127.0.0.1:1"
	c := NewHandler(http.Client{}, host)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // force client.Do to fail without touching the network

	err := c.Get(ctx, "/eth/v1/node/health", nil)
	require.NotNil(t, err)
	require.Equal(t, false, strings.Contains(err.Error(), secret), "error leaked credentials: %s", err.Error())
	require.Equal(t, true, strings.Contains(err.Error(), "eth:xxxxx"), "error missing redacted host: %s", err.Error())
}

// A plain-text (non-JSON) error body — e.g. the 415 produced by content-type negotiation —
// must still surface as a typed DefaultJsonError carrying the status code, so callers can
// react to it (e.g. fall back to JSON).
func TestPostSSZ_NonJSONErrorBodyIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// http.Error writes a text/plain body.
		http.Error(w, "Unsupported media type: application/octet-stream", http.StatusUnsupportedMediaType)
	}))
	defer srv.Close()

	c := NewHandler(http.Client{}, srv.URL)
	_, _, err := c.PostSSZ(context.Background(), "/eth/v1/test", nil, bytes.NewBuffer([]byte{0x01}))
	require.NotNil(t, err)
	errJson := &httputil.DefaultJsonError{}
	require.Equal(t, true, errors.As(err, &errJson), "expected DefaultJsonError, got %T", err)
	require.Equal(t, http.StatusUnsupportedMediaType, errJson.Code)
}

func TestGetSSZ_NonJSONErrorBodyIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Acceptable", http.StatusNotAcceptable)
	}))
	defer srv.Close()

	c := NewHandler(http.Client{}, srv.URL)
	_, _, err := c.GetSSZ(context.Background(), "/eth/v1/test")
	require.NotNil(t, err)
	errJson := &httputil.DefaultJsonError{}
	require.Equal(t, true, errors.As(err, &errJson), "expected DefaultJsonError, got %T", err)
	require.Equal(t, http.StatusNotAcceptable, errJson.Code)
}

// A JSON error body is decoded into the typed error's fields.
func TestPostSSZ_JSONErrorBodyIsDecoded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", api.JsonMediaType)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":400,"message":"bad request"}`))
	}))
	defer srv.Close()

	c := NewHandler(http.Client{}, srv.URL)
	_, _, err := c.PostSSZ(context.Background(), "/eth/v1/test", nil, bytes.NewBuffer([]byte{0x01}))
	require.NotNil(t, err)
	errJson := &httputil.DefaultJsonError{}
	require.Equal(t, true, errors.As(err, &errJson), "expected DefaultJsonError, got %T", err)
	require.Equal(t, http.StatusBadRequest, errJson.Code)
	require.Equal(t, "bad request", errJson.Message)
}

func TestHandler_ConcurrentHostSwitch(t *testing.T) {
	newServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", api.JsonMediaType)
			_, _ = w.Write([]byte(`{"data":"ok"}`))
		}))
	}
	srv1 := newServer()
	defer srv1.Close()
	srv2 := newServer()
	defer srv2.Close()

	c := newHandler(http.Client{}, srv1.URL)
	errs := make(chan error, 100)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 10 {
				var resp struct {
					Data string `json:"data"`
				}
				if err := c.Get(context.Background(), "/eth/v1/test", &resp); err != nil {
					errs <- err
				}
			}
		})
	}
	for range 100 {
		c.SwitchHost(srv2.URL)
		c.SwitchHost(srv1.URL)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
