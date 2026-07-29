package rest

import "net/http"

// MockRestProvider implements RestConnectionProvider for testing.
type MockRestProvider struct {
	MockHandler Handler
	MockClient  *http.Client
	MockHosts   []string
}

func (m *MockRestProvider) HttpClient() *http.Client { return m.MockClient }
func (m *MockRestProvider) Handler() Handler         { return m.MockHandler }
func (m *MockRestProvider) Hosts() []string          { return m.MockHosts }
