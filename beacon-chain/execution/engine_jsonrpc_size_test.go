package execution

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// ctxCapturingRPC records the context its CallContext is invoked with.
type ctxCapturingRPC struct {
	RPCClient
	label string
	ok    bool
}

func (c *ctxCapturingRPC) CallContext(ctx context.Context, _ any, _ string, _ ...any) error {
	c.label, c.ok = ctx.Value(engineMethodLabelCtxKey{}).(string)
	return nil
}

// engineLabelingClient tags engine_* calls with their canonical method label and
// leaves eth_* calls untagged.
func TestEngineLabelingClient(t *testing.T) {
	inner := &ctxCapturingRPC{}
	c := &engineLabelingClient{RPCClient: inner}

	require.NoError(t, c.CallContext(context.Background(), nil, NewPayloadMethodV3))
	require.Equal(t, true, inner.ok)
	require.Equal(t, methodNewPayload, inner.label)

	require.NoError(t, c.CallContext(context.Background(), nil, GetPayloadBodiesByRangeV2))
	require.Equal(t, methodGetPayloadBodiesByRange, inner.label)

	inner.ok, inner.label = false, ""
	require.NoError(t, c.CallContext(context.Background(), nil, BlockByHashMethod)) // eth_*, not tagged
	require.Equal(t, false, inner.ok)
}

type fakeRoundTripper struct{ body string }

func (f *fakeRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(f.body))}, nil
}

// engineSizeRoundTripper is transparent: a wrapped (labeled) response body is still
// fully readable, and an unlabeled request passes through unchanged.
func TestEngineSizeRoundTripper_Transparent(t *testing.T) {
	rt := &engineSizeRoundTripper{base: &fakeRoundTripper{body: "0123456789"}}

	ctx := context.WithValue(context.Background(), engineMethodLabelCtxKey{}, methodGetPayload)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://el", nil)
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "0123456789", string(body))

	req2, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://el", nil)
	require.NoError(t, err)
	resp2, err := rt.RoundTrip(req2)
	require.NoError(t, err)
	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, "0123456789", string(body2))
}

// A labeled round-trip records one request- and one response-size observation.
func TestEngineSizeRoundTripper_Records(t *testing.T) {
	rt := &engineSizeRoundTripper{base: &fakeRoundTripper{body: "0123456789"}}
	beforeReq := engineBodySizeCount(t, methodNewPayload, transportJSON, directionRequest)
	beforeResp := engineBodySizeCount(t, methodNewPayload, transportJSON, directionResponse)

	ctx := context.WithValue(context.Background(), engineMethodLabelCtxKey{}, methodNewPayload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://el", strings.NewReader("req-body"))
	require.NoError(t, err)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, beforeReq+1, engineBodySizeCount(t, methodNewPayload, transportJSON, directionRequest))
	require.Equal(t, beforeResp+1, engineBodySizeCount(t, methodNewPayload, transportJSON, directionResponse))
}

// engineBodySizeCount returns the observation count of one engine_body_size_bytes series.
func engineBodySizeCount(t *testing.T, labels ...string) uint64 {
	obs, err := engineBodySize.GetMetricWithLabelValues(labels...)
	require.NoError(t, err)
	m := &dto.Metric{}
	require.NoError(t, obs.(prometheus.Metric).Write(m))
	return m.GetHistogram().GetSampleCount()
}
