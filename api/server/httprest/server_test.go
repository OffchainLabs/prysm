package httprest

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/OffchainLabs/prysm/v6/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
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
	go func() {
		require.LogsContain(t, hook, "Starting HTTP server")
		require.LogsDoNotContain(t, hook, "Starting API middleware")
	}()
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
