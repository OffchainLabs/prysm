package peerbuddy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestHandler(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET "+Path, Handler())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Do not follow redirects so they can be asserted on.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(t *testing.T, path string) (*http.Response, string) {
		resp, err := client.Get(srv.URL + path)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		return resp, string(body)
	}

	t.Run("index", func(t *testing.T) {
		resp, body := get(t, Path)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, true, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html"))
		assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
		assert.Equal(t, true, strings.Contains(body, "PeerBuddy"))
	})

	t.Run("script asset", func(t *testing.T) {
		resp, body := get(t, Path+"app.js")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, true, strings.Contains(resp.Header.Get("Content-Type"), "javascript"))
		assert.Equal(t, true, len(body) > 0)
	})

	t.Run("stylesheet asset", func(t *testing.T) {
		resp, _ := get(t, Path+"app.css")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, true, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/css"))
	})

	t.Run("missing asset", func(t *testing.T) {
		resp, _ := get(t, Path+"missing.txt")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("nested path", func(t *testing.T) {
		resp, _ := get(t, Path+"nested/app.js")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("index alias redirects to the root", func(t *testing.T) {
		resp, _ := get(t, Path+"index.html")
		assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
		assert.Equal(t, "./", resp.Header.Get("Location"))
	})

	t.Run("missing trailing slash redirects", func(t *testing.T) {
		resp, _ := get(t, strings.TrimSuffix(Path, "/"))
		assert.Equal(t, true, resp.StatusCode >= 300 && resp.StatusCode < 400, "status %d", resp.StatusCode)
		assert.Equal(t, Path, resp.Header.Get("Location"))
	})
}
