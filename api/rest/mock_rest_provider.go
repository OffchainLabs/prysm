package rest

import "net/http"

// MockRestProvider implements RestConnectionProvider for testing.
type MockRestProvider struct {
	MockHandler Handler
	MockClient  *http.Client
	MockHosts   []string
	ConnCounter uint64
}

func (m *MockRestProvider) HttpClient() *http.Client  { return m.MockClient }
func (m *MockRestProvider) Handler() Handler          { return m.MockHandler }
func (m *MockRestProvider) Hosts() []string           { return m.MockHosts }
func (m *MockRestProvider) ConnectionCounter() uint64 { return m.ConnCounter }
