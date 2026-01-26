package grpc

import "google.golang.org/grpc"

// MockGrpcProvider implements GrpcConnectionProvider for testing.
type MockGrpcProvider struct {
	MockConn  *grpc.ClientConn
	MockHosts []string
}

func (m *MockGrpcProvider) CurrentConn() *grpc.ClientConn { return m.MockConn }
func (m *MockGrpcProvider) CurrentHost() string {
	if len(m.MockHosts) > 0 {
		return m.MockHosts[0]
	}
	return ""
}
func (m *MockGrpcProvider) Hosts() []string      { return m.MockHosts }
func (m *MockGrpcProvider) SwitchHost(int) error { return nil }
func (m *MockGrpcProvider) Close()               {}
