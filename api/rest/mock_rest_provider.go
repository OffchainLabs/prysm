package rest

import "net/http"

// MockRestProvider implements RestConnectionProvider for testing.
type MockRestProvider struct {
	MockClient *http.Client
	MockHosts  []string
	HostIndex  int
}

func (m *MockRestProvider) HttpClient() *http.Client { return m.MockClient }
func (m *MockRestProvider) CurrentHost() string {
	if len(m.MockHosts) > 0 {
		return m.MockHosts[m.HostIndex%len(m.MockHosts)]
	}
	return ""
}
func (m *MockRestProvider) Hosts() []string       { return m.MockHosts }
func (m *MockRestProvider) SetHost(index int) error { m.HostIndex = index; return nil }
func (m *MockRestProvider) NextHost()             { m.HostIndex = (m.HostIndex + 1) % len(m.MockHosts) }
