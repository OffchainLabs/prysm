package execution

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/network"
	"github.com/OffchainLabs/prysm/v7/network/authorization"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	dto "github.com/prometheus/client_model/go"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// fallbackCount reads the current value of the SSZ-over-HTTP fallback counter.
func fallbackCount(t *testing.T) float64 {
	m := &dto.Metric{}
	require.NoError(t, engineSSZHTTPFallbackCount.Write(m))
	return m.GetCounter().GetValue()
}

const testCapabilitiesBody = `{"supported_forks":["osaka","amsterdam"],"limits":{"payload.max_bytes":268435456}}`

// h2cServer stands up an HTTP/2 cleartext test server, matching the transport
// enginehttp.Client speaks.
func h2cServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	srv := httptest.NewServer(h2c.NewHandler(h, &http2.Server{}))
	t.Cleanup(srv.Close)
	return srv
}

func bearerEndpoint(url string) network.Endpoint {
	return network.Endpoint{
		Url: url,
		Auth: network.AuthorizationData{
			Method: authorization.Bearer,
			Value:  "0123456789abcdef0123456789abcdef", // raw HS256 secret
		},
	}
}

func TestSelectEngineTransport_FlagOff(t *testing.T) {
	defer features.Init(&features.Flags{})
	features.Init(&features.Flags{EnableEngineSSZHTTP: false})

	srv := h2cServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("capabilities must not be probed when the flag is off")
	})
	s := &Service{}
	s.selectEngineTransport(context.Background(), bearerEndpoint(srv.URL))

	require.IsNil(t, s.sszTransport)
	ie, ok := s.engine().(*instrumentedEngine)
	require.Equal(t, true, ok)
	assert.Equal(t, transportJSON, ie.kind)
}

func TestSelectEngineTransport_Probe(t *testing.T) {
	defer features.Init(&features.Flags{})
	features.Init(&features.Flags{EnableEngineSSZHTTP: true})

	var gotPath string
	srv := h2cServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testCapabilitiesBody))
	})
	s := &Service{}
	s.selectEngineTransport(context.Background(), bearerEndpoint(srv.URL))

	require.NotNil(t, s.sszTransport)
	assert.Equal(t, "/engine/v2/capabilities", gotPath)
	ie, ok := s.engine().(*instrumentedEngine)
	require.Equal(t, true, ok)
	assert.Equal(t, transportSSZ, ie.kind)
}

func TestSelectEngineTransport_Fallback(t *testing.T) {
	defer features.Init(&features.Flags{})
	features.Init(&features.Flags{EnableEngineSSZHTTP: true})

	srv := h2cServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // EL has no engine v2 surface
	})
	before := fallbackCount(t)
	s := &Service{}
	s.selectEngineTransport(context.Background(), bearerEndpoint(srv.URL))

	require.IsNil(t, s.sszTransport)
	ie, ok := s.engine().(*instrumentedEngine)
	require.Equal(t, true, ok)
	assert.Equal(t, transportJSON, ie.kind)
	assert.Equal(t, float64(1), fallbackCount(t)-before)
}
