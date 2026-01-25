package grpc

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	pkgErrors "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// GrpcConnectionProvider manages gRPC connections for failover support.
// It allows switching between different beacon node endpoints when the current one becomes unavailable.
// Only one connection is maintained at a time - when switching hosts, the old connection is closed.
type GrpcConnectionProvider interface {
	// CurrentConn returns the currently active gRPC connection.
	// The connection is created lazily on first call.
	// Returns nil if the provider has been closed.
	CurrentConn() *grpc.ClientConn
	// CurrentHost returns the address of the currently active endpoint.
	CurrentHost() string
	// Hosts returns all configured endpoint addresses.
	Hosts() []string
	// SetHost switches to the endpoint at the given index.
	// The new connection is created lazily on next CurrentConn() call.
	SetHost(index int) error
	// Close closes the current connection.
	Close() error
}

type grpcConnectionProvider struct {
	// Immutable after construction - no lock needed for reads
	endpoints []string
	ctx       context.Context
	dialOpts  []grpc.DialOption

	// Current connection state (protected by mutex)
``
in case you remove the name from the mutex
	currentIndex uint64
	conn         *grpc.ClientConn

	mu     sync.Mutex
	closed atomic.Bool
}

// NewGrpcConnectionProvider creates a new connection provider that manages gRPC connections.
// The endpoint parameter can be a comma-separated list of addresses (e.g., "host1:4000,host2:4000").
// Only one connection is maintained at a time, created lazily on first use.
func NewGrpcConnectionProvider(
	ctx context.Context,
	endpoint string,
	dialOpts []grpc.DialOption,
) (GrpcConnectionProvider, error) {
	endpoints := parseEndpoints(endpoint)
	if len(endpoints) == 0 {
		return nil, pkgErrors.New("no gRPC endpoints provided")
	}

	log.WithFields(logrus.Fields{
		"endpoints": endpoints,
		"count":     len(endpoints),
	}).Info("Initialized gRPC connection provider")

	return &grpcConnectionProvider{
		endpoints: endpoints,
		ctx:       ctx,
		dialOpts:  dialOpts,
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

	p.mu.Lock()
	defer p.mu.Unlock()

	// Return existing connection if available
	if p.conn != nil {
		return p.conn
	}

	// Create connection lazily
	ep := p.endpoints[p.currentIndex]
	conn, err := grpc.DialContext(p.ctx, ep, p.dialOpts...)
	if err != nil {
		log.WithError(err).WithField("endpoint", ep).Error("Failed to create gRPC connection")
		return nil
	}

	p.conn = conn
	log.WithField("endpoint", ep).Debug("Created gRPC connection")
	return conn
}

func (p *grpcConnectionProvider) CurrentHost() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.endpoints[p.currentIndex]
}

func (p *grpcConnectionProvider) Hosts() []string {
	// Return a copy to maintain immutability
	hosts := make([]string, len(p.endpoints))
	copy(hosts, p.endpoints)
	return hosts
}

func (p *grpcConnectionProvider) SetHost(index int) error {
	if index < 0 || index >= len(p.endpoints) {
		return pkgErrors.Errorf("invalid host index %d, must be between 0 and %d", index, len(p.endpoints)-1)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if uint64(index) == p.currentIndex {
		return nil // Already on this host
	}

	oldHost := p.endpoints[p.currentIndex]

	// Close existing connection if any
	if p.conn != nil {
		if err := p.conn.Close(); err != nil {
			log.WithError(err).WithField("endpoint", oldHost).Debug("Failed to close previous connection")
		}
		p.conn = nil
	}

	p.currentIndex = uint64(index)

	log.WithFields(logrus.Fields{
		"previousHost": oldHost,
		"newHost":      p.endpoints[index],
	}).Debug("Switched gRPC endpoint")
	return nil
}

func (p *grpcConnectionProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed.Load() {
		return nil
	}
	p.closed.Store(true)

	if p.conn != nil {
		if err := p.conn.Close(); err != nil {
			return pkgErrors.Wrapf(err, "failed to close connection to %s", p.endpoints[p.currentIndex])
		}
		p.conn = nil
	}
	return nil
}
