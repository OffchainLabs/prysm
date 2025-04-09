package helpers

import (
	"time"

	"google.golang.org/grpc"
)

// Use an interface with a private dummy function to force all other packages to call NewNodeConnection
type NodeConnection interface {
	// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
	GetGrpcClientConn() *grpc.ClientConn
	GetBeaconApiUrl() string
	GetBeaconApiTimeout() time.Duration
	dummy()
}

type nodeConnection struct {
	// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
	grpcClientConn   *grpc.ClientConn
	beaconApiUrl     string
	beaconApiTimeout time.Duration
}

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
func (c *nodeConnection) GetGrpcClientConn() *grpc.ClientConn {
	return c.grpcClientConn
}

func (c *nodeConnection) GetBeaconApiUrl() string {
	return c.beaconApiUrl
}

func (c *nodeConnection) GetBeaconApiTimeout() time.Duration {
	return c.beaconApiTimeout
}

func (*nodeConnection) dummy() {}

func NewNodeConnection(grpcConn *grpc.ClientConn, beaconApiUrl string, beaconApiTimeout time.Duration) NodeConnection {
	conn := &nodeConnection{}
	conn.grpcClientConn = grpcConn
	conn.beaconApiUrl = beaconApiUrl
	conn.beaconApiTimeout = beaconApiTimeout
	return conn
}
