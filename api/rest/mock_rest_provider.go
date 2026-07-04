package rest

// MockRestProvider implements RestConnectionProvider for testing.
type MockRestProvider struct {
	MockHandler Handler
	MockHosts   []string
	HostIndex   int
}

func (m *MockRestProvider) Handler() Handler { return m.MockHandler }
func (m *MockRestProvider) CurrentHost() string {
	if len(m.MockHosts) > 0 {
		return m.MockHosts[m.HostIndex%len(m.MockHosts)]
	}
	return ""
}
func (m *MockRestProvider) Hosts() []string            { return m.MockHosts }
func (m *MockRestProvider) SwitchHost(index int) error { m.HostIndex = index; return nil }
