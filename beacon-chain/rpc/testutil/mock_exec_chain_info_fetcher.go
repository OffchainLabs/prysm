package testutil

// MockExecutionChainInfoFetcher is a fake implementation of the powchain.ChainInfoFetcher
type MockExecutionChainInfoFetcher struct {
	CurrEndpoint string
	CurrError    error
}

func (*MockExecutionChainInfoFetcher) ExecutionClientConnected() bool {
	return true
}

func (m *MockExecutionChainInfoFetcher) ExecutionClientEndpoint() string {
	return m.CurrEndpoint
}

func (m *MockExecutionChainInfoFetcher) ExecutionClientConnectionErr() error {
	return m.CurrError
}
