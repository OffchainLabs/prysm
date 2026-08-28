package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func testExecutionPayloadBid() *eth.SignedExecutionPayloadBid {
	return &eth.SignedExecutionPayloadBid{
		Message: &eth.ExecutionPayloadBid{
			ParentBlockHash:       bytes.Repeat([]byte{1}, 32),
			ParentBlockRoot:       bytes.Repeat([]byte{2}, 32),
			BlockHash:             bytes.Repeat([]byte{3}, 32),
			PrevRandao:            bytes.Repeat([]byte{4}, 32),
			FeeRecipient:          bytes.Repeat([]byte{5}, 20),
			GasLimit:              30000000,
			BuilderIndex:          7,
			Slot:                  123,
			Value:                 1000,
			ExecutionPayment:      500,
			BlobKzgCommitments:    [][]byte{},
			ExecutionRequestsRoot: bytes.Repeat([]byte{6}, 32),
		},
		Signature: bytes.Repeat([]byte{7}, 96),
	}
}

func testRequestAuth() *eth.SignedRequestAuth {
	return &eth.SignedRequestAuth{
		Message:   &eth.RequestAuth{Data: []byte("http://builder.example"), Slot: 123},
		Signature: bytes.Repeat([]byte{9}, 96),
	}
}

func gloasBidClient(t *testing.T, status int, contentType string, body []byte) *Client {
	hc := &http.Client{
		Transport: roundtrip(func(r *http.Request) (*http.Response, error) {
			if r.Body != nil {
				require.NoError(t, r.Body.Close())
			}
			require.Equal(t, http.MethodPost, r.Method)
			h := http.Header{}
			if contentType != "" {
				h.Set("Content-Type", contentType)
			}
			return &http.Response{
				StatusCode: status,
				Header:     h,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Request:    r,
			}, nil
		}),
	}
	return &Client{hc: hc, baseURL: &url.URL{Host: "localhost:3500", Scheme: "http"}}
}

func TestClient_GetExecutionPayloadBid(t *testing.T) {
	ctx := t.Context()
	slot := primitives.Slot(123)
	var parentHash, parentRoot [32]byte
	var pubkey [48]byte
	want := testExecutionPayloadBid()

	t.Run("json response", func(t *testing.T) {
		body, err := json.Marshal(struct {
			Data *structs.SignedExecutionPayloadBid `json:"data"`
		}{Data: structs.SignedExecutionPayloadBidFromConsensus(want)})
		require.NoError(t, err)
		c := gloasBidClient(t, http.StatusOK, api.JsonMediaType, body)
		got, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, testRequestAuth())
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, want.Message.Slot, got.Message.Slot)
		require.Equal(t, want.Message.Value, got.Message.Value)
		require.DeepEqual(t, want.Signature, got.Signature)
	})

	t.Run("ssz response", func(t *testing.T) {
		body, err := want.MarshalSSZ()
		require.NoError(t, err)
		c := gloasBidClient(t, http.StatusOK, api.OctetStreamMediaType, body)
		got, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, testRequestAuth())
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, want.Message.Value, got.Message.Value)
		require.DeepEqual(t, want.Message.ParentBlockHash, got.Message.ParentBlockHash)
	})

	t.Run("no bid", func(t *testing.T) {
		c := gloasBidClient(t, http.StatusNoContent, "", nil)
		got, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, testRequestAuth())
		require.NoError(t, err)
		require.IsNil(t, got)
	})

	t.Run("nil auth errors", func(t *testing.T) {
		c := gloasBidClient(t, http.StatusOK, api.JsonMediaType, nil)
		_, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, nil)
		require.ErrorContains(t, "nil signed request auth", err)
	})

	t.Run("advertises the shorter of client timeout and context deadline", func(t *testing.T) {
		jsonBody, err := json.Marshal(struct {
			Data *structs.SignedExecutionPayloadBid `json:"data"`
		}{Data: structs.SignedExecutionPayloadBidFromConsensus(want)})
		require.NoError(t, err)
		hc := &http.Client{
			Timeout: 100 * time.Millisecond,
			Transport: roundtrip(func(r *http.Request) (*http.Response, error) {
				require.NoError(t, r.Body.Close())
				timeoutMs, err := strconv.ParseInt(r.Header.Get("X-Timeout-Ms"), 10, 64)
				require.NoError(t, err)
				require.Equal(t, true, timeoutMs > 0 && timeoutMs <= 100)
				h := http.Header{}
				h.Set("Content-Type", api.JsonMediaType)
				return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(bytes.NewReader(jsonBody)), Request: r}, nil
			}),
		}
		c := &Client{hc: hc, baseURL: &url.URL{Host: "localhost:3500", Scheme: "http"}}
		dctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		_, err = c.GetExecutionPayloadBid(dctx, slot, parentHash, parentRoot, pubkey, testRequestAuth())
		require.NoError(t, err)
	})

	t.Run("required headers are sent", func(t *testing.T) {
		jsonBody, err := json.Marshal(struct {
			Data *structs.SignedExecutionPayloadBid `json:"data"`
		}{Data: structs.SignedExecutionPayloadBidFromConsensus(want)})
		require.NoError(t, err)
		hc := &http.Client{
			Transport: roundtrip(func(r *http.Request) (*http.Response, error) {
				require.NoError(t, r.Body.Close())
				require.Equal(t, "gloas", r.Header.Get(api.VersionHeader))
				require.Equal(t, api.JsonMediaType, r.Header.Get("Content-Type"))
				dateMs, err := strconv.ParseInt(r.Header.Get("Date-Milliseconds"), 10, 64)
				require.NoError(t, err)
				require.Equal(t, true, dateMs > 0)
				timeoutMs, err := strconv.ParseInt(r.Header.Get("X-Timeout-Ms"), 10, 64)
				require.NoError(t, err)
				require.Equal(t, true, timeoutMs > 0 && timeoutMs <= 300)
				h := http.Header{}
				h.Set("Content-Type", api.JsonMediaType)
				return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(bytes.NewReader(jsonBody)), Request: r}, nil
			}),
		}
		c := &Client{hc: hc, baseURL: &url.URL{Host: "localhost:3500", Scheme: "http"}}
		dctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		got, err := c.GetExecutionPayloadBid(dctx, slot, parentHash, parentRoot, pubkey, testRequestAuth())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("unexpected content type errors with status and body", func(t *testing.T) {
		html := []byte("<!doctype html><html><head><title>Buildoor</title></head></html>")
		c := gloasBidClient(t, http.StatusOK, "text/html; charset=utf-8", html)
		got, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, testRequestAuth())
		require.IsNil(t, got)
		require.ErrorContains(t, "unexpected Content-Type", err)
		require.ErrorContains(t, "text/html", err)
		require.ErrorContains(t, "Buildoor", err)
	})

	t.Run("ssz request auth body", func(t *testing.T) {
		auth := &eth.SignedRequestAuth{
			Message:   &eth.RequestAuth{Data: []byte("http://builder.example"), Slot: 5},
			Signature: bytes.Repeat([]byte{9}, 96),
		}
		wantBody, err := auth.MarshalSSZ()
		require.NoError(t, err)
		sszBid, err := want.MarshalSSZ()
		require.NoError(t, err)
		hc := &http.Client{
			Transport: roundtrip(func(r *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, r.Body.Close())
				require.Equal(t, api.OctetStreamMediaType, r.Header.Get("Content-Type"))
				require.DeepEqual(t, wantBody, body)
				h := http.Header{}
				h.Set("Content-Type", api.OctetStreamMediaType)
				return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(bytes.NewReader(sszBid)), Request: r}, nil
			}),
		}
		c := &Client{hc: hc, baseURL: &url.URL{Host: "localhost:3500", Scheme: "http"}, sszEnabled: true}
		got, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, auth)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, want.Message.Value, got.Message.Value)
	})

	t.Run("SSZ rejected falls back to JSON", func(t *testing.T) {
		jsonBody, err := json.Marshal(struct {
			Data *structs.SignedExecutionPayloadBid `json:"data"`
		}{Data: structs.SignedExecutionPayloadBidFromConsensus(want)})
		require.NoError(t, err)
		var reqCount int
		hc := &http.Client{
			Transport: roundtrip(func(r *http.Request) (*http.Response, error) {
				if r.Body != nil {
					require.NoError(t, r.Body.Close())
				}
				reqCount++
				if reqCount == 1 {
					// The builder cannot produce an SSZ response; reject the octet-stream Accept
					// header with a PLAIN-TEXT body (matching real builders, not a JSON error).
					require.Equal(t, api.OctetStreamMediaType, r.Header.Get("Accept"))
					return &http.Response{StatusCode: http.StatusNotAcceptable, Body: io.NopCloser(bytes.NewBufferString("only " + api.JsonMediaType)), Request: r}, nil
				}
				require.Equal(t, api.JsonMediaType, r.Header.Get("Accept"))
				h := http.Header{}
				h.Set("Content-Type", api.JsonMediaType)
				return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(bytes.NewReader(jsonBody)), Request: r}, nil
			}),
		}
		c := &Client{hc: hc, baseURL: &url.URL{Host: "localhost:3500", Scheme: "http"}, sszEnabled: true}
		got, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, testRequestAuth())
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, 2, reqCount)
		require.Equal(t, true, c.sszRejected.Load())
		require.Equal(t, want.Message.Value, got.Message.Value)
	})
}

func TestClient_SubmitSignedBeaconBlock_SSZFallback(t *testing.T) {
	ctx := t.Context()
	sb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockGloas())
	require.NoError(t, err)
	var reqCount int
	hc := &http.Client{
		Transport: roundtrip(func(r *http.Request) (*http.Response, error) {
			require.NoError(t, r.Body.Close())
			reqCount++
			if reqCount == 1 {
				// The builder does not support SSZ; reject the octet-stream body.
				require.Equal(t, api.OctetStreamMediaType, r.Header.Get("Content-Type"))
				return &http.Response{StatusCode: http.StatusUnsupportedMediaType, Body: io.NopCloser(bytes.NewBufferString("ssz not supported")), Request: r}, nil
			}
			require.Equal(t, api.JsonMediaType, r.Header.Get("Content-Type"))
			return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(bytes.NewReader(nil)), Request: r}, nil
		}),
	}
	c := &Client{hc: hc, baseURL: &url.URL{Host: "localhost:3500", Scheme: "http"}, sszEnabled: true}
	require.NoError(t, c.SubmitSignedBeaconBlock(ctx, sb))
	require.Equal(t, 2, reqCount)
	require.Equal(t, true, c.sszRejected.Load())
}

func TestClient_SubmitBuilderPreferences_SSZFallback(t *testing.T) {
	ctx := t.Context()
	var pubkey [48]byte
	req := &eth.BuilderPreferencesRequest{
		Preferences: &eth.BuilderPreferences{MaxExecutionPayment: 1000},
		Auth: &eth.SignedRequestAuth{
			Message:   &eth.RequestAuth{Data: []byte("http://builder.example"), Slot: 5},
			Signature: bytes.Repeat([]byte{9}, 96),
		},
	}
	var reqCount int
	hc := &http.Client{
		Transport: roundtrip(func(r *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, r.Body.Close())
			reqCount++
			if reqCount == 1 {
				// The builder does not support SSZ; reject the octet-stream body with a
				// PLAIN-TEXT body, matching real Commit Boost (not a JSON error object).
				require.Equal(t, api.OctetStreamMediaType, r.Header.Get("Content-Type"))
				return &http.Response{StatusCode: http.StatusUnsupportedMediaType, Body: io.NopCloser(bytes.NewBufferString("Expected request with `Content-Type: " + api.JsonMediaType + "`")), Request: r}, nil
			}
			require.Equal(t, api.JsonMediaType, r.Header.Get("Content-Type"))
			var decoded struct {
				Preferences struct {
					MaxExecutionPayment string `json:"max_execution_payment"`
				} `json:"preferences"`
			}
			require.NoError(t, json.Unmarshal(body, &decoded))
			require.Equal(t, "1000", decoded.Preferences.MaxExecutionPayment)
			return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(bytes.NewReader(nil)), Request: r}, nil
		}),
	}
	c := &Client{hc: hc, baseURL: &url.URL{Host: "localhost:3500", Scheme: "http"}, sszEnabled: true}
	require.NoError(t, c.SubmitBuilderPreferences(ctx, pubkey, req))
	require.Equal(t, 2, reqCount)
	require.Equal(t, true, c.sszRejected.Load())
}
