package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestValidHostname(t *testing.T) {
	cases := []struct {
		name    string
		hostArg string
		path    string
		joined  string
		err     error
	}{
		{
			name:    "hostname without port",
			hostArg: "mydomain.org",
			err:     ErrMalformedHostname,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cl, err := NewClient(c.hostArg)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.joined, cl.BaseURL().ResolveReference(&url.URL{Path: c.path}).String())
		})
	}
}

func TestWithAuthenticationToken(t *testing.T) {
	cl, err := NewClient("https://www.offchainlabs.com:3500", WithAuthenticationToken("my token"))
	require.NoError(t, err)
	require.Equal(t, cl.Token(), "my token")
}

func TestBaseURL(t *testing.T) {
	cl, err := NewClient("https://www.offchainlabs.com:3500")
	require.NoError(t, err)
	require.Equal(t, "www.offchainlabs.com", cl.BaseURL().Hostname())
	require.Equal(t, "3500", cl.BaseURL().Port())
}

func TestAcceptOverride(t *testing.T) {
	name := "TestAcceptOverride"
	orig := os.Getenv(envNameOverrideAccept)
	defer func() {
		os.Setenv(envNameOverrideAccept, orig)
	}()
	os.Setenv(envNameOverrideAccept, name)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, name, r.Header.Get("Accept"))
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	b, err := c.Get(t.Context(), "/test")
	require.NoError(t, err)
	require.Equal(t, "ok", string(b))
}
