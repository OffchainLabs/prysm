package execution

import (
	"context"
	"io"
	"net/http"
)

// This file records JSON-RPC engine request/response body sizes into
// engine_body_size_bytes{transport="json-rpc"}, the counterpart to observeSSZBody
// on the ssz-http path, so the two transports' wire sizes are comparable in one
// metric. The JSON-RPC client (geth's rpc.Client) marshals internally and does not
// expose the bytes, so the sizes are taken at the HTTP layer: an RPCClient wrapper
// tags each engine_* call's context with its canonical method label, and a
// RoundTripper on the engine connection reads that label to attribute the sizes.
// eth_* calls share the connection but carry no label, so they are not recorded.

// engineMethodLabelCtxKey marks a request context with the canonical engine method
// label (engineLabelingClient sets it; engineSizeRoundTripper reads it).
type engineMethodLabelCtxKey struct{}

// jsonEngineMethodLabel maps the versioned engine_* JSON-RPC method names onto the
// same per-endpoint labels the ssz-http path uses (engine_transport.go), so the
// engine_body_size_bytes series line up across transports.
var jsonEngineMethodLabel = map[string]string{
	NewPayloadMethod:          methodNewPayload,
	NewPayloadMethodV2:        methodNewPayload,
	NewPayloadMethodV3:        methodNewPayload,
	NewPayloadMethodV4:        methodNewPayload,
	NewPayloadMethodV5:        methodNewPayload,
	ForkchoiceUpdatedMethod:   methodForkchoiceUpdated,
	ForkchoiceUpdatedMethodV2: methodForkchoiceUpdated,
	ForkchoiceUpdatedMethodV3: methodForkchoiceUpdated,
	ForkchoiceUpdatedMethodV4: methodForkchoiceUpdated,
	GetPayloadMethod:          methodGetPayload,
	GetPayloadMethodV2:        methodGetPayload,
	GetPayloadMethodV3:        methodGetPayload,
	GetPayloadMethodV4:        methodGetPayload,
	GetPayloadMethodV5:        methodGetPayload,
	GetPayloadMethodV6:        methodGetPayload,
	GetBlobsV1:                methodGetBlobs,
	GetBlobsV2:                methodGetBlobsV2,
	GetPayloadBodiesByHashV1:  methodGetPayloadBodiesByHash,
	GetPayloadBodiesByHashV2:  methodGetPayloadBodiesByHash,
	GetPayloadBodiesByRangeV1: methodGetPayloadBodiesByRange,
	GetPayloadBodiesByRangeV2: methodGetPayloadBodiesByRange,
}

// engineLabelingClient wraps an RPCClient so each engine_* call's context carries
// its canonical method label. Non-engine_* methods (eth_*) are left untouched.
type engineLabelingClient struct {
	RPCClient
}

func (c *engineLabelingClient) CallContext(ctx context.Context, result any, method string, args ...any) error {
	if label, ok := jsonEngineMethodLabel[method]; ok {
		ctx = context.WithValue(ctx, engineMethodLabelCtxKey{}, label)
	}
	return c.RPCClient.CallContext(ctx, result, method, args...)
}

// engineSizeRoundTripper records the JSON-RPC engine request/response body sizes
// for requests carrying an engineMethodLabelCtxKey. The request size comes from
// Content-Length (set by the rpc client); the response size from a counting reader,
// so neither body is buffered. Unlabeled requests pass through unmeasured.
type engineSizeRoundTripper struct {
	base http.RoundTripper
}

func (rt *engineSizeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	label, ok := req.Context().Value(engineMethodLabelCtxKey{}).(string)
	if !ok || label == "" {
		return rt.base.RoundTrip(req)
	}
	if req.ContentLength > 0 {
		engineBodySize.WithLabelValues(label, transportJSON, directionRequest).Observe(float64(req.ContentLength))
	}
	resp, err := rt.base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	resp.Body = &countingReadCloser{ReadCloser: resp.Body, label: label}
	return resp, nil
}

// countingReadCloser counts bytes read from a JSON-RPC engine response body and
// records the total into engine_body_size_bytes once (on first EOF or Close).
type countingReadCloser struct {
	io.ReadCloser
	label    string
	n        int64
	recorded bool
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.n += int64(n)
	if err == io.EOF {
		c.record()
	}
	return n, err
}

func (c *countingReadCloser) Close() error {
	c.record()
	return c.ReadCloser.Close()
}

func (c *countingReadCloser) record() {
	if c.recorded {
		return
	}
	c.recorded = true
	engineBodySize.WithLabelValues(c.label, transportJSON, directionResponse).Observe(float64(c.n))
}
