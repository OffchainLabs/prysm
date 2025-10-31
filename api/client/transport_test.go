package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
)

type noopTransport struct{}

func (*noopTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, nil
}

func TestRoundTrip(t *testing.T) {
	tr := &CustomHeadersTransport{base: &noopTransport{}, headers: map[string][]string{"key1": []string{"value1", "value2"}, "key2": []string{"value3"}}}
	req := httptest.NewRequest("GET", "http://foo", nil)
	_, err := tr.RoundTrip(req)
	require.NoError(t, err)
	assert.DeepEqual(t, []string{"value1", "value2"}, req.Header.Values("key1"))
	assert.DeepEqual(t, []string{"value3"}, req.Header.Values("key2"))
}
