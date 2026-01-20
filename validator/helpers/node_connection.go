package helpers

import (
	"context"
	"net/http"

	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// NodeConnection provides access to both gRPC and REST API connections to a beacon node.
type NodeConnection interface {
	// GetGrpcClientConn returns the current gRPC client connection.
	// Returns nil if no gRPC provider is configured.
	GetGrpcClientConn() *grpc.ClientConn
	// GetGrpcConnectionProvider returns the gRPC connection provider.
	GetGrpcConnectionProvider() grpcutil.GrpcConnectionProvider
	// GetRestConnectionProvider returns the REST connection provider.
	GetRestConnectionProvider() rest.RestConnectionProvider
	// GetRestHandler returns the REST handler for making API requests.
	// Returns nil if no REST provider is configured.
	GetRestHandler() rest.RestHandler
	// GetHttpClient returns the configured HTTP client for REST API requests.
	// Returns nil if no REST provider is configured.
	GetHttpClient() *http.Client
}

type nodeConnection struct {
	grpcConnectionProvider grpcutil.GrpcConnectionProvider
	restConnectionProvider rest.RestConnectionProvider
}

func (c *nodeConnection) GetGrpcClientConn() *grpc.ClientConn {
	if c.grpcConnectionProvider == nil {
		return nil
	}
	return c.grpcConnectionProvider.CurrentConn()
}

func (c *nodeConnection) GetGrpcConnectionProvider() grpcutil.GrpcConnectionProvider {
	return c.grpcConnectionProvider
}

func (c *nodeConnection) GetRestConnectionProvider() rest.RestConnectionProvider {
	return c.restConnectionProvider
}

func (c *nodeConnection) GetRestHandler() rest.RestHandler {
	if c.restConnectionProvider == nil {
		return nil
	}
	return c.restConnectionProvider.RestHandler()
}

func (c *nodeConnection) GetHttpClient() *http.Client {
	if c.restConnectionProvider == nil {
		return nil
	}
	return c.restConnectionProvider.HttpClient()
}

// nodeConnectionBuilder is used internally to build a NodeConnection.
type nodeConnectionBuilder struct {
	grpcProvider grpcutil.GrpcConnectionProvider
	restProvider rest.RestConnectionProvider
}

// NodeConnectionOption is a functional option for configuring a NodeConnection.
type NodeConnectionOption func(*nodeConnectionBuilder) error

// WithGrpc configures a gRPC connection provider for the NodeConnection.
// If endpoint is empty, this option is a no-op.
func WithGrpc(ctx context.Context, endpoint string, dialOpts []grpc.DialOption) NodeConnectionOption {
	return func(b *nodeConnectionBuilder) error {
		if endpoint == "" {
			return nil
		}
		provider, err := grpcutil.NewGrpcConnectionProvider(ctx, endpoint, dialOpts)
		if err != nil {
			return errors.Wrap(err, "failed to create gRPC connection provider")
		}
		b.grpcProvider = provider
		return nil
	}
}

// WithREST configures a REST connection provider for the NodeConnection.
// If endpoint is empty, this option is a no-op.
func WithREST(endpoint string, opts ...rest.RestConnectionProviderOption) NodeConnectionOption {
	return func(b *nodeConnectionBuilder) error {
		if endpoint == "" {
			return nil
		}
		provider, err := rest.NewRestConnectionProvider(endpoint, opts...)
		if err != nil {
			return errors.Wrap(err, "failed to create REST connection provider")
		}
		b.restProvider = provider
		return nil
	}
}

// WithGrpcProvider sets a pre-built gRPC connection provider.
// This is primarily useful for testing with mock providers.
func WithGrpcProvider(provider grpcutil.GrpcConnectionProvider) NodeConnectionOption {
	return func(b *nodeConnectionBuilder) error {
		b.grpcProvider = provider
		return nil
	}
}

// WithRestProvider sets a pre-built REST connection provider.
// This is primarily useful for testing with mock providers.
func WithRestProvider(provider rest.RestConnectionProvider) NodeConnectionOption {
	return func(b *nodeConnectionBuilder) error {
		b.restProvider = provider
		return nil
	}
}

// NewNodeConnection creates a new NodeConnection with the given options.
// At least one provider (gRPC or REST) must be configured via options.
// Returns an error if no providers are configured.
func NewNodeConnection(opts ...NodeConnectionOption) (NodeConnection, error) {
	b := &nodeConnectionBuilder{}
	for _, opt := range opts {
		if err := opt(b); err != nil {
			return nil, err
		}
	}

	if b.grpcProvider == nil && b.restProvider == nil {
		return nil, errors.New("at least one beacon node endpoint must be provided (--beacon-rpc-provider or --beacon-rest-api-provider)")
	}

	return &nodeConnection{
		grpcConnectionProvider: b.grpcProvider,
		restConnectionProvider: b.restProvider,
	}, nil
}
