package grpc_api

import (
	"sync"

	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"google.golang.org/grpc"
)

// grpcClientManager handles dynamic gRPC client recreation when the connection changes.
// It uses generics to work with any gRPC client type.
type grpcClientManager[T any] struct {
	conn      validatorHelpers.NodeConnection
	client    T
	lastHost  string
	clientMu  sync.RWMutex
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
	// Safety check for tests that create manager directly without connection
	if m.conn == nil || m.conn.GetGrpcConnectionProvider() == nil {
		return m.client
	}
	currentHost := m.conn.GetGrpcConnectionProvider().CurrentHost()
	m.clientMu.RLock()
	if m.lastHost == currentHost {
		client := m.client
		m.clientMu.RUnlock()
		return client
	}
	m.clientMu.RUnlock()

	// Connection changed, need to recreate client
	m.clientMu.Lock()
	defer m.clientMu.Unlock()
	// Double-check after acquiring write lock
	if m.lastHost == currentHost {
		return m.client
	}
	m.client = m.newClient(m.conn.GetGrpcClientConn())
	m.lastHost = currentHost
	return m.client
}
