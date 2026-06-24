package rest

import (
	"context"
	"net/http"
	"strings"
	"testing"

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
