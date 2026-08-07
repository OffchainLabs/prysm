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
	c := newHandler(http.Client{}, host)

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

	c := newHandler(http.Client{}, srv.URL)
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

	c := newHandler(http.Client{}, srv.URL)
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

	c := newHandler(http.Client{}, srv.URL)
	_, _, err := c.PostSSZ(context.Background(), "/eth/v1/test", nil, bytes.NewBuffer([]byte{0x01}))
	require.NotNil(t, err)
	errJson := &httputil.DefaultJsonError{}
	require.Equal(t, true, errors.As(err, &errJson), "expected DefaultJsonError, got %T", err)
	require.Equal(t, http.StatusBadRequest, errJson.Code)
	require.Equal(t, "bad request", errJson.Message)
}

func TestHandler_getRaw(t *testing.T) {
	server := func(status int, contentType, body string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		return srv
	}

	t.Run("returns the raw JSON body on success", func(t *testing.T) {
		srv := server(http.StatusOK, api.JsonMediaType, `{"data":"ok"}`)
		raw, err := newHandler(http.Client{}, srv.URL).getRaw(context.Background(), "/x")
		require.NoError(t, err)
		require.Equal(t, `{"data":"ok"}`, string(raw))
	})

	t.Run("treats an empty 2XX JSON body as an error", func(t *testing.T) {
		srv := server(http.StatusOK, api.JsonMediaType, "")
		_, err := newHandler(http.Client{}, srv.URL).getRaw(context.Background(), "/x")
		require.ErrorContains(t, "empty response body", err)
	})

	t.Run("decodes a JSON error body on a non-2XX status", func(t *testing.T) {
		srv := server(http.StatusBadRequest, api.JsonMediaType, `{"code":400,"message":"bad request"}`)
		_, err := newHandler(http.Client{}, srv.URL).getRaw(context.Background(), "/x")
		require.NotNil(t, err)
		errJson := &httputil.DefaultJsonError{}
		require.Equal(t, true, errors.As(err, &errJson), "expected DefaultJsonError, got %T", err)
		require.Equal(t, http.StatusBadRequest, errJson.Code)
		require.Equal(t, "bad request", errJson.Message)
	})

	t.Run("errors when a non-2XX JSON error body cannot be decoded", func(t *testing.T) {
		srv := server(http.StatusBadRequest, api.JsonMediaType, "not json")
		_, err := newHandler(http.Client{}, srv.URL).getRaw(context.Background(), "/x")
		require.ErrorContains(t, "failed to decode response body into error json", err)
	})

	t.Run("returns a typed error for a non-JSON non-2XX response", func(t *testing.T) {
		srv := server(http.StatusInternalServerError, "text/plain", "boom")
		_, err := newHandler(http.Client{}, srv.URL).getRaw(context.Background(), "/x")
		require.NotNil(t, err)
		errJson := &httputil.DefaultJsonError{}
		require.Equal(t, true, errors.As(err, &errJson), "expected DefaultJsonError, got %T", err)
		require.Equal(t, http.StatusInternalServerError, errJson.Code)
	})

	t.Run("returns no body and no error for a non-JSON 2XX response", func(t *testing.T) {
		srv := server(http.StatusOK, "text/plain", "ignored")
		raw, err := newHandler(http.Client{}, srv.URL).getRaw(context.Background(), "/x")
		require.NoError(t, err)
		require.Equal(t, 0, len(raw))
	})

	t.Run("propagates a body read error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			// Declare a 50-byte JSON body, then close without sending it so the
			// client's ReadAll fails with an unexpected EOF.
			_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 50\r\n\r\n"))
			_ = conn.Close()
		}))
		t.Cleanup(srv.Close)

		_, err := newHandler(http.Client{}, srv.URL).getRaw(context.Background(), "/x")
		require.ErrorContains(t, "failed to read response body", err)
	})
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
