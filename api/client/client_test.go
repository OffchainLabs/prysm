package client

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
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

func TestGet(t *testing.T) {
	t.Run("max body size", func(t *testing.T) {
		tests := []struct {
			name    string
			limit   int64
			body    string
			wantErr bool
		}{
			{name: "empty", limit: 3},
			{name: "below limit", limit: 3, body: "ab"},
			{name: "at limit", limit: 3, body: "abc"},
			{name: "one byte over limit", limit: 3, body: "abcd", wantErr: true},
			{name: "over limit", limit: 3, body: "abcdef", wantErr: true},
			{name: "zero limit empty", limit: 0},
			{name: "zero limit nonempty", limit: 0, body: "a", wantErr: true},
			{name: "maximum limit", limit: math.MaxInt64, body: "abc"},
		}
		for _, chunked := range []bool{false, true} {
			name := "content length"
			if chunked {
				name = "chunked"
			}
			t.Run(name, func(t *testing.T) {
				for _, tt := range tests {
					t.Run(tt.name, func(t *testing.T) {
						srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
							if chunked {
								w.(http.Flusher).Flush()
							}
							_, err := w.Write([]byte(tt.body))
							if err != nil {
								t.Errorf("write response: %v", err)
							}
						}))
						defer srv.Close()
						cl, err := NewClient(srv.URL, WithMaxBodySize(tt.limit))
						require.NoError(t, err)
						body, err := cl.Get(context.Background(), "/")
						if tt.wantErr {
							require.ErrorIs(t, err, ErrResponseBodyTooLarge)
							require.IsNil(t, body)
							return
						}
						require.NoError(t, err)
						require.Equal(t, tt.body, string(body))
					})
				}
			})
		}
	})
	t.Run("read error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "3")
			_, err := w.Write([]byte("ab"))
			if err != nil {
				t.Errorf("write response: %v", err)
			}
		}))
		defer srv.Close()
		cl, err := NewClient(srv.URL, WithMaxBodySize(3))
		require.NoError(t, err)
		body, err := cl.Get(context.Background(), "/")
		require.ErrorContains(t, "error reading http response body", err)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		require.IsNil(t, body)
	})
	t.Run("bounded read", func(t *testing.T) {
		response := &responseBody{Reader: strings.NewReader(strings.Repeat("a", 1024))}
		cl, err := NewClient("http://localhost:3500", WithMaxBodySize(3), WithRoundTripper(&responseTransport{body: response}))
		require.NoError(t, err)
		body, err := cl.Get(context.Background(), "/")
		require.ErrorIs(t, err, ErrResponseBodyTooLarge)
		require.IsNil(t, body)
		require.Equal(t, 1020, response.Len())
		require.Equal(t, true, response.closed)
	})
}

func TestNewClient_MaxBodySize(t *testing.T) {
	for _, limit := range []int64{-1, math.MinInt64} {
		t.Run(fmt.Sprintf("limit %d", limit), func(t *testing.T) {
			cl, err := NewClient("http://localhost:3500", WithMaxBodySize(limit))
			require.ErrorContains(t, "max body size must not be negative", err)
			require.IsNil(t, cl)
		})
	}
}

type responseBody struct {
	*strings.Reader
	closed bool
}

func (b *responseBody) Close() error {
	b.closed = true
	return nil
}

type responseTransport struct {
	body *responseBody
}

func (tr *responseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: tr.body, ContentLength: -1}, nil
}
