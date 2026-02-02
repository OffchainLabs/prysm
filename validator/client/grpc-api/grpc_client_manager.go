package grpc_api

import (
	"sync"

	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"google.golang.org/grpc"
)

// grpcClientManager handles dynamic gRPC client recreation when the connection changes.
// It uses generics to work with any gRPC client type.
type grpcClientManager[T any] struct {
	mu        sync.Mutex
	conn      validatorHelpers.NodeConnection
	client    T
	lastHost  string
	newClient func(grpc.ClientConnInterface) T
}

// newGrpcClientManager creates a new client manager with the given connection and client constructor.
func newGrpcClientManager[T any](
	conn validatorHelpers.NodeConnection,
	newClient func(grpc.ClientConnInterface) T,
) *grpcClientManager[T] {
	return &grpcClientManager[T]{
		conn:      conn,
		newClient: newClient,
		client:    newClient(conn.GetGrpcClientConn()),
		lastHost:  conn.GetGrpcConnectionProvider().CurrentHost(),
	}
}

// getClient returns the current client, recreating it if the connection has changed.
func (m *grpcClientManager[T]) getClient() T {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentHost := m.conn.GetGrpcConnectionProvider().CurrentHost()
	if m.lastHost != currentHost {
		m.client = m.newClient(m.conn.GetGrpcClientConn())
		m.lastHost = currentHost
	}
	return m.client
}
