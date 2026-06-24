package httprest

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/urfave/cli/v2"
)

func TestServer_StartStop(t *testing.T) {
	hook := logTest.NewGlobal()

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	ctx := cli.NewContext(&app, set, nil)

	port := ctx.Int(flags.HTTPServerPort.Name)
	portStr := fmt.Sprintf("%d", port) // Convert port to string
	host := ctx.String(flags.HTTPServerHost.Name)
	address := net.JoinHostPort(host, portStr)
	handler := http.NewServeMux()
	opts := []Option{
		WithHTTPAddr(address),
		WithRouter(handler),
	}

	g, err := New(t.Context(), opts...)
	require.NoError(t, err)

	g.Start()
	require.Eventually(t, func() bool {
		foundStart := false
		for _, entry := range hook.AllEntries() {
			if strings.Contains(entry.Message, "Starting HTTP server") {
				foundStart = true
			}
			if strings.Contains(entry.Message, "Starting API middleware") {
				return false
			}
		}
		return foundStart
	}, time.Second, 10*time.Millisecond)
	err = g.Stop()
	require.NoError(t, err)
}

func TestServer_NilHandler_NotFoundHandlerRegistered(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	ctx := cli.NewContext(&app, set, nil)

	handler := http.NewServeMux()
	port := ctx.Int(flags.HTTPServerPort.Name)
	portStr := fmt.Sprintf("%d", port) // Convert port to string
	host := ctx.String(flags.HTTPServerHost.Name)
	address := net.JoinHostPort(host, portStr)

	opts := []Option{
		WithHTTPAddr(address),
		WithRouter(handler),
	}

	g, err := New(t.Context(), opts...)
	require.NoError(t, err)

	writer := httptest.NewRecorder()
	g.cfg.router.ServeHTTP(writer, &http.Request{Method: "GET", Host: "localhost", URL: &url.URL{Path: "/foo"}})
	assert.Equal(t, http.StatusNotFound, writer.Code)
}

func TestServer_TimeoutHandlerBypassesSSE(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc(eventStreamPath, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("stream-open"))
		require.NoError(t, err)
	})

	g, err := New(t.Context(),
		WithHTTPAddr("127.0.0.1:0"),
		WithRouter(handler),
		WithTimeout(5*time.Millisecond),
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, eventStreamPath, nil)
	writer := httptest.NewRecorder()
	g.server.Handler.ServeHTTP(writer, req)

	assert.Equal(t, http.StatusOK, writer.Code)
	assert.Equal(t, "stream-open", writer.Body.String())
}

func TestServer_TimeoutHandlerStillAppliesToNonSSE(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/foo", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("ok"))
		require.NoError(t, err)
	})

	g, err := New(t.Context(),
		WithHTTPAddr("127.0.0.1:0"),
		WithRouter(handler),
		WithTimeout(5*time.Millisecond),
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	writer := httptest.NewRecorder()
	g.server.Handler.ServeHTTP(writer, req)

	assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
	assert.Equal(t, true, strings.Contains(writer.Body.String(), "request timed out"))
}
