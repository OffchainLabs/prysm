package grpc

import "google.golang.org/grpc"

// MockGrpcProvider implements GrpcConnectionProvider for testing.
type MockGrpcProvider struct {
	MockConn     *grpc.ClientConn
	MockHosts    []string
	CurrentIndex int
	ConnCounter  uint64
}

func (m *MockGrpcProvider) CurrentConn() *grpc.ClientConn { return m.MockConn }
func (m *MockGrpcProvider) CurrentHost() string {
	if len(m.MockHosts) > 0 {
		return m.MockHosts[m.CurrentIndex]
	}
	return ""
}
func (m *MockGrpcProvider) Hosts() []string { return m.MockHosts }
func (m *MockGrpcProvider) SwitchHost(idx int) error {
	m.CurrentIndex = idx
	m.ConnCounter++
	return nil
}
func (m *MockGrpcProvider) ConnectionCounter() uint64 { return m.ConnCounter }
func (m *MockGrpcProvider) Close()                    {}
