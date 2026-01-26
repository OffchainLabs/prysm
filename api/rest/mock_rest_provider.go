package rest

import "net/http"

// MockRestProvider implements RestConnectionProvider for testing.
type MockRestProvider struct {
	MockClient  *http.Client
	MockHandler RestHandler
	MockHosts   []string
	HostIndex   int
}

func (m *MockRestProvider) HttpClient() *http.Client { return m.MockClient }
func (m *MockRestProvider) RestHandler() RestHandler { return m.MockHandler }
func (m *MockRestProvider) CurrentHost() string {
	if len(m.MockHosts) > 0 {
		return m.MockHosts[m.HostIndex%len(m.MockHosts)]
	}
	return ""
}
func (m *MockRestProvider) Hosts() []string            { return m.MockHosts }
func (m *MockRestProvider) SwitchHost(index int) error { m.HostIndex = index; return nil }
