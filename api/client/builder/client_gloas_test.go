package builder

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
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
		got, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, nil)
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
		got, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, nil)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, want.Message.Value, got.Message.Value)
		require.DeepEqual(t, want.Message.ParentBlockHash, got.Message.ParentBlockHash)
	})

	t.Run("no bid", func(t *testing.T) {
		c := gloasBidClient(t, http.StatusNoContent, "", nil)
		got, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, nil)
		require.NoError(t, err)
		require.IsNil(t, got)
	})

	t.Run("unexpected content type errors with status and body", func(t *testing.T) {
		html := []byte("<!doctype html><html><head><title>Buildoor</title></head></html>")
		c := gloasBidClient(t, http.StatusOK, "text/html; charset=utf-8", html)
		got, err := c.GetExecutionPayloadBid(ctx, slot, parentHash, parentRoot, pubkey, nil)
		require.IsNil(t, got)
		require.ErrorContains(t, "unexpected Content-Type", err)
		require.ErrorContains(t, "text/html", err)
		require.ErrorContains(t, "Buildoor", err)
	})
}
