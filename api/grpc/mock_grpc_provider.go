package grpc

import "google.golang.org/grpc"

// MockGrpcProvider implements GrpcConnectionProvider for testing.
type MockGrpcProvider struct {
	MockConn     *grpc.ClientConn
	MockHosts    []string
	CurrentIndex int
}

func (m *MockGrpcProvider) CurrentConn() *grpc.ClientConn { return m.MockConn }
func (m *MockGrpcProvider) CurrentHost() string {
	if len(m.MockHosts) > 0 {
		return m.MockHosts[m.CurrentIndex]
	}
	return ""
}
func (m *MockGrpcProvider) Hosts() []string          { return m.MockHosts }
func (m *MockGrpcProvider) SwitchHost(idx int) error { m.CurrentIndex = idx; return nil }
func (m *MockGrpcProvider) Close()                   {}
