package grpc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	pkgErrors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// GrpcConnectionProvider manages multiple gRPC connections for failover support.
// It allows switching between different beacon node endpoints when the current one becomes unavailable.
type GrpcConnectionProvider interface {
	// CurrentConn returns the currently active gRPC connection.
	// Returns nil if the provider has been closed.
	CurrentConn() *grpc.ClientConn
	// CurrentHost returns the address of the currently active endpoint.
	CurrentHost() string
	// Hosts returns all configured endpoint addresses.
	Hosts() []string
	// Conn returns the connection at the given index.
	Conn(index int) *grpc.ClientConn
	// SetHost switches to the endpoint at the given index.
	SetHost(index int) error
	// NextHost switches to the next available endpoint in round-robin fashion.
	NextHost()
	// Close closes all managed connections.
	Close() error
}

type grpcConnectionProvider struct {
	// Immutable after construction - no lock needed for reads
	endpoints   []string
	connections []*grpc.ClientConn

	// Atomic index for lock-free current endpoint access
	currentIndex atomic.Uint64

	// Mutex only for Close() and write operations that need log consistency
	mu     sync.Mutex
	closed atomic.Bool
}

// NewGrpcConnectionProvider creates a new connection provider that manages multiple gRPC connections.
// The endpoint parameter can be a comma-separated list of addresses (e.g., "host1:4000,host2:4000").
// It creates a separate connection for each endpoint using the provided dial options.
func NewGrpcConnectionProvider(
	ctx context.Context,
	endpoint string,
	dialOpts []grpc.DialOption,
) (GrpcConnectionProvider, error) {
	endpoints := parseEndpoints(endpoint)
	if len(endpoints) == 0 {
		return nil, pkgErrors.New("no gRPC endpoints provided")
	}

	connections := make([]*grpc.ClientConn, 0, len(endpoints))
	for _, ep := range endpoints {
		conn, err := grpc.DialContext(ctx, ep, dialOpts...)
		if err != nil {
			// Clean up already created connections
			for _, c := range connections {
				if err := c.Close(); err != nil {
					log.WithError(err).Warn("Failed to close connection during cleanup")
				}
			}
			return nil, pkgErrors.Wrapf(err, "failed to connect to gRPC endpoint %s", ep)
		}
		connections = append(connections, conn)
	}

	log.WithFields(logrus.Fields{
		"endpoints": endpoints,
		"count":     len(endpoints),
	}).Info("Initialized gRPC connection provider with multiple endpoints")

	return &grpcConnectionProvider{
		endpoints:   endpoints,
		connections: connections,
	}, nil
}

// parseEndpoints splits a comma-separated endpoint string into individual endpoints.
func parseEndpoints(endpoint string) []string {
	if endpoint == "" {
		return nil
	}
	var endpoints []string
	for p := range strings.SplitSeq(endpoint, ",") {
		if p = strings.TrimSpace(p); p != "" {
			endpoints = append(endpoints, p)
		}
	}
	return endpoints
}

func (p *grpcConnectionProvider) CurrentConn() *grpc.ClientConn {
	if p.closed.Load() {
		return nil
	}
	idx := p.currentIndex.Load() % uint64(len(p.connections))
	return p.connections[idx]
}

func (p *grpcConnectionProvider) CurrentHost() string {
	idx := p.currentIndex.Load() % uint64(len(p.endpoints))
	return p.endpoints[idx]
}

func (p *grpcConnectionProvider) Hosts() []string {
	// Return a copy to maintain immutability
	hosts := make([]string, len(p.endpoints))
	copy(hosts, p.endpoints)
	return hosts
}

func (p *grpcConnectionProvider) Conn(index int) *grpc.ClientConn {
	if p.closed.Load() {
		return nil
	}
	if index < 0 || index >= len(p.connections) {
		return nil
	}
	return p.connections[index]
}

func (p *grpcConnectionProvider) SetHost(index int) error {
	if index < 0 || index >= len(p.endpoints) {
		return pkgErrors.Errorf("invalid host index %d, must be between 0 and %d", index, len(p.endpoints)-1)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	oldIdx := p.currentIndex.Load()
	p.currentIndex.Store(uint64(index))

	log.WithFields(logrus.Fields{
		"previousHost": p.endpoints[oldIdx%uint64(len(p.endpoints))],
		"newHost":      p.endpoints[index],
	}).Debug("Trying gRPC endpoint")
	return nil
}

func (p *grpcConnectionProvider) NextHost() {
	p.mu.Lock()
	defer p.mu.Unlock()

	oldIdx := p.currentIndex.Load()
	newIdx := (oldIdx + 1) % uint64(len(p.endpoints))
	p.currentIndex.Store(newIdx)

	log.WithFields(logrus.Fields{
		"previousHost": p.endpoints[oldIdx],
		"newHost":      p.endpoints[newIdx],
	}).Debug("Switched to next gRPC endpoint")
}

func (p *grpcConnectionProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed.Load() {
		return nil
	}
	p.closed.Store(true)

	var errs []error
	for i, conn := range p.connections {
		if err := conn.Close(); err != nil {
			errs = append(errs, pkgErrors.Wrapf(err, "failed to close connection to %s", p.endpoints[i]))
		}
	}
	return errors.Join(errs...)
}
