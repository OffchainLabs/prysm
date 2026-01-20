package helpers

import (
	"net/http"

	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"github.com/OffchainLabs/prysm/v7/api/rest"
	"google.golang.org/grpc"
)

// NodeConnection provides access to both gRPC and REST API connections to a beacon node.
// Use an interface with a private dummy function to force all other packages to call NewNodeConnection.
type NodeConnection interface {
	// GetGrpcClientConn returns the current gRPC client connection.
	// Returns nil if no gRPC provider is configured.
	GetGrpcClientConn() *grpc.ClientConn
	// GetGrpcConnectionProvider returns the gRPC connection provider.
	GetGrpcConnectionProvider() grpcutil.GrpcConnectionProvider
	// GetRestConnectionProvider returns the REST connection provider.
	GetRestConnectionProvider() rest.RestConnectionProvider
	// GetHttpClient returns the configured HTTP client for REST API requests.
	// Returns nil if no REST provider is configured.
	GetHttpClient() *http.Client
	dummy()
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

func (c *nodeConnection) GetHttpClient() *http.Client {
	if c.restConnectionProvider == nil {
		return nil
	}
	return c.restConnectionProvider.HttpClient()
}

func (*nodeConnection) dummy() {}

// NewNodeConnection creates a new NodeConnection with the given gRPC and REST providers.
// Either provider can be nil if that connection type is not needed.
func NewNodeConnection(grpcProvider grpcutil.GrpcConnectionProvider, restProvider rest.RestConnectionProvider) NodeConnection {
	return &nodeConnection{
		grpcConnectionProvider: grpcProvider,
		restConnectionProvider: restProvider,
	}
}
