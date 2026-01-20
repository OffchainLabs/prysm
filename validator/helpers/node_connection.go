package helpers

import (
	"strings"
	"time"

	grpcutil "github.com/OffchainLabs/prysm/v7/api/grpc"
	"google.golang.org/grpc"
)

// Use an interface with a private dummy function to force all other packages to call NewNodeConnection
type NodeConnection interface {
	GetGrpcClientConn() *grpc.ClientConn
	GetBeaconApiUrl() string
	// GetBeaconApiHosts returns the list of beacon API hosts parsed from the URL.
	GetBeaconApiHosts() []string
	GetBeaconApiHeaders() map[string][]string
	setBeaconApiHeaders(map[string][]string)
	GetBeaconApiTimeout() time.Duration
	setBeaconApiTimeout(time.Duration)
	// GetGrpcConnectionProvider returns the gRPC connection provider.
	GetGrpcConnectionProvider() grpcutil.GrpcConnectionProvider
	dummy()
}

type nodeConnection struct {
	grpcConnectionProvider grpcutil.GrpcConnectionProvider
	beaconApiUrl           string
	beaconApiHeaders       map[string][]string
	beaconApiTimeout       time.Duration
}

// NodeConnectionOption is a functional option for configuring the node connection.
type NodeConnectionOption func(nc NodeConnection)

// WithBeaconApiHeaders sets the HTTP headers that should be sent to the server along with each request.
func WithBeaconApiHeaders(headers map[string][]string) NodeConnectionOption {
	return func(nc NodeConnection) {
		nc.setBeaconApiHeaders(headers)
	}
}

// WithBeaconApiTimeout sets the HTTP request timeout.
func WithBeaconApiTimeout(timeout time.Duration) NodeConnectionOption {
	return func(nc NodeConnection) {
		nc.setBeaconApiTimeout(timeout)
	}
}

func (c *nodeConnection) GetGrpcClientConn() *grpc.ClientConn {
	if c.grpcConnectionProvider == nil {
		return nil
	}
	return c.grpcConnectionProvider.CurrentConn()
}

func (c *nodeConnection) GetBeaconApiUrl() string {
	return c.beaconApiUrl
}

func (c *nodeConnection) GetBeaconApiHosts() []string {
	url := strings.ReplaceAll(c.beaconApiUrl, " ", "")
	if url == "" {
		return nil
	}
	return strings.Split(url, ",")
}

func (c *nodeConnection) GetBeaconApiHeaders() map[string][]string {
	return c.beaconApiHeaders
}

func (c *nodeConnection) setBeaconApiHeaders(headers map[string][]string) {
	c.beaconApiHeaders = headers
}

func (c *nodeConnection) GetBeaconApiTimeout() time.Duration {
	return c.beaconApiTimeout
}

func (c *nodeConnection) setBeaconApiTimeout(timeout time.Duration) {
	c.beaconApiTimeout = timeout
}

func (c *nodeConnection) GetGrpcConnectionProvider() grpcutil.GrpcConnectionProvider {
	return c.grpcConnectionProvider
}

func (*nodeConnection) dummy() {}

func NewNodeConnection(provider grpcutil.GrpcConnectionProvider, beaconApiUrl string, opts ...NodeConnectionOption) NodeConnection {
	conn := &nodeConnection{
		grpcConnectionProvider: provider,
		beaconApiUrl:           beaconApiUrl,
	}
	for _, opt := range opts {
		opt(conn)
	}
	return conn
}
