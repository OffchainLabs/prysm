package helpers

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var log = logrus.WithField("prefix", "helpers")

// GrpcConnectionProvider manages multiple gRPC connections for failover support.
// It allows switching between different beacon node endpoints when the current one becomes unavailable.
type GrpcConnectionProvider interface {
	// CurrentConn returns the currently active gRPC connection.
	CurrentConn() *grpc.ClientConn
	// CurrentHost returns the address of the currently active endpoint.
	CurrentHost() string
	// Hosts returns all configured endpoint addresses.
	Hosts() []string
	// SetHost switches to the endpoint at the given index.
	SetHost(index int) error
	// NextHost switches to the next available endpoint in round-robin fashion.
	NextHost()
	// Close closes all managed connections.
	Close() error
}

type grpcConnectionProvider struct {
	endpoints    []string
	connections  []*grpc.ClientConn
	currentIndex atomic.Uint64
	mu           sync.RWMutex
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
		return nil, errors.New("no gRPC endpoints provided")
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
			return nil, errors.Wrapf(err, "failed to connect to gRPC endpoint %s", ep)
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
	endpoint = strings.ReplaceAll(endpoint, " ", "")
	if endpoint == "" {
		return nil
	}
	parts := strings.Split(endpoint, ",")
	endpoints := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			endpoints = append(endpoints, p)
		}
	}
	return endpoints
}

func (p *grpcConnectionProvider) CurrentConn() *grpc.ClientConn {
	p.mu.RLock()
	defer p.mu.RUnlock()
	idx := p.currentIndex.Load() % uint64(len(p.connections))
	return p.connections[idx]
}

func (p *grpcConnectionProvider) CurrentHost() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	idx := p.currentIndex.Load() % uint64(len(p.endpoints))
	return p.endpoints[idx]
}

func (p *grpcConnectionProvider) Hosts() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	hosts := make([]string, len(p.endpoints))
	copy(hosts, p.endpoints)
	return hosts
}

func (p *grpcConnectionProvider) SetHost(index int) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if index < 0 || index >= len(p.endpoints) {
		return errors.Errorf("invalid host index %d, must be between 0 and %d", index, len(p.endpoints)-1)
	}
	oldIdx := p.currentIndex.Load()
	p.currentIndex.Store(uint64(index))
	log.WithFields(logrus.Fields{
		"previousHost": p.endpoints[oldIdx%uint64(len(p.endpoints))],
		"newHost":      p.endpoints[index],
	}).Info("Switched gRPC endpoint")
	return nil
}

func (p *grpcConnectionProvider) NextHost() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	oldIdx := p.currentIndex.Load()
	newIdx := (oldIdx + 1) % uint64(len(p.endpoints))
	p.currentIndex.Store(newIdx)
	log.WithFields(logrus.Fields{
		"previousHost": p.endpoints[oldIdx%uint64(len(p.endpoints))],
		"newHost":      p.endpoints[newIdx],
	}).Warn("Beacon node is not responding, switching gRPC endpoint")
}

func (p *grpcConnectionProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var errs []error
	for i, conn := range p.connections {
		if err := conn.Close(); err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to close connection to %s", p.endpoints[i]))
		}
	}
	if len(errs) > 0 {
		return errors.Errorf("errors closing connections: %v", errs)
	}
	return nil
}

// GrpcHealthChecker provides health checking functionality for gRPC endpoints.
type GrpcHealthChecker interface {
	// IsHealthy checks if the gRPC connection at the given index is healthy.
	IsHealthy(ctx context.Context, index int) bool
}

// WithHealthCheck wraps a GrpcConnectionProvider with health checking capability.
type grpcConnectionProviderWithHealth struct {
	GrpcConnectionProvider
	healthCheck func(ctx context.Context, conn *grpc.ClientConn) bool
}

// NewGrpcConnectionProviderWithHealthCheck creates a connection provider with custom health checking.
func NewGrpcConnectionProviderWithHealthCheck(
	provider GrpcConnectionProvider,
	healthCheck func(ctx context.Context, conn *grpc.ClientConn) bool,
) GrpcHealthChecker {
	return &grpcConnectionProviderWithHealth{
		GrpcConnectionProvider: provider,
		healthCheck:            healthCheck,
	}
}

func (p *grpcConnectionProviderWithHealth) IsHealthy(ctx context.Context, index int) bool {
	hosts := p.Hosts()
	if index < 0 || index >= len(hosts) {
		return false
	}
	// We need to get the connection at the specific index
	// This requires accessing the underlying provider's connections
	if provider, ok := p.GrpcConnectionProvider.(*grpcConnectionProvider); ok {
		provider.mu.RLock()
		conn := provider.connections[index]
		provider.mu.RUnlock()
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return p.healthCheck(ctx, conn)
	}
	return false
}
