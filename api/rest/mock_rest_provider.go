package rest

import (
	"bytes"
	"context"
	"net/http"
)

// MockRestProvider implements RestConnectionProvider for testing.
type MockRestProvider struct {
	MockClient  *http.Client
	MockHandler Handler
	MockHosts   []string
	HostIndex   int
}

func (m *MockRestProvider) HttpClient() *http.Client { return m.MockClient }
func (m *MockRestProvider) Handler() Handler         { return m.MockHandler }
func (m *MockRestProvider) CurrentHost() string {
	if len(m.MockHosts) > 0 {
		return m.MockHosts[m.HostIndex%len(m.MockHosts)]
	}
	return ""
}
func (m *MockRestProvider) Hosts() []string            { return m.MockHosts }
func (m *MockRestProvider) SwitchHost(index int) error { m.HostIndex = index; return nil }

// MockHandler implements Handler for testing.
type MockHandler struct {
	MockHost   string
	MockClient *http.Client
}

func (m *MockHandler) Get(_ context.Context, _ string, _ any) error { return nil }
func (m *MockHandler) GetStatusCode(_ context.Context, _ string) (int, error) {
	return http.StatusOK, nil
}
func (m *MockHandler) GetSSZ(_ context.Context, _ string) ([]byte, http.Header, error) {
	return nil, nil, nil
}
func (m *MockHandler) Post(_ context.Context, _ string, _ map[string]string, _ *bytes.Buffer, _ any) error {
	return nil
}
func (m *MockHandler) PostSSZ(_ context.Context, _ string, _ map[string]string, _ *bytes.Buffer) ([]byte, http.Header, error) {
	return nil, nil, nil
}
func (m *MockHandler) HttpClient() *http.Client { return m.MockClient }
func (m *MockHandler) Host() string             { return m.MockHost }
func (m *MockHandler) SwitchHost(host string)   { m.MockHost = host }
