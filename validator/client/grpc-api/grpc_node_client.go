package grpc_api

import (
	"context"
	"sync"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
)

var (
	_ = iface.NodeClient(&grpcNodeClient{})
)

type grpcNodeClient struct {
	nodeClient   ethpb.NodeClient
	conn         validatorHelpers.NodeConnection
	lastHost     string
	clientMu     sync.RWMutex
}

// getClient returns the current node client, recreating it if the connection has changed.
func (c *grpcNodeClient) getClient() ethpb.NodeClient {
	if c.conn == nil || c.conn.GetGrpcConnectionProvider() == nil {
		// No connection provider, use static client
		return c.nodeClient
	}

	currentHost := c.conn.GetGrpcConnectionProvider().CurrentHost()
	c.clientMu.RLock()
	if c.lastHost == currentHost {
		client := c.nodeClient
		c.clientMu.RUnlock()
		return client
	}
	c.clientMu.RUnlock()

	// Connection changed, need to recreate client
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	// Double-check after acquiring write lock
	if c.lastHost == currentHost {
		return c.nodeClient
	}
	c.nodeClient = ethpb.NewNodeClient(c.conn.GetGrpcClientConn())
	c.lastHost = currentHost
	return c.nodeClient
}

func (c *grpcNodeClient) SyncStatus(ctx context.Context, in *empty.Empty) (*ethpb.SyncStatus, error) {
	return c.getClient().GetSyncStatus(ctx, in)
}

func (c *grpcNodeClient) Genesis(ctx context.Context, in *empty.Empty) (*ethpb.Genesis, error) {
	return c.getClient().GetGenesis(ctx, in)
}

func (c *grpcNodeClient) Version(ctx context.Context, in *empty.Empty) (*ethpb.Version, error) {
	return c.getClient().GetVersion(ctx, in)
}

func (c *grpcNodeClient) Peers(ctx context.Context, in *empty.Empty) (*ethpb.Peers, error) {
	return c.getClient().ListPeers(ctx, in)
}

func (c *grpcNodeClient) IsHealthy(ctx context.Context) bool {
	_, err := c.getClient().GetHealth(ctx, &ethpb.HealthRequest{})
	if err != nil {
		log.WithError(err).Error("Failed to get health of node")
		return false
	}
	return true
}

// NewNodeClient creates a new gRPC node client from a single connection.
// This is the legacy constructor for backward compatibility.
func NewNodeClient(cc grpc.ClientConnInterface) iface.NodeClient {
	return &grpcNodeClient{nodeClient: ethpb.NewNodeClient(cc)}
}

// NewNodeClientWithConnection creates a new gRPC node client that supports
// dynamic connection switching via the NodeConnection's GrpcConnectionProvider.
func NewNodeClientWithConnection(conn validatorHelpers.NodeConnection) iface.NodeClient {
	client := &grpcNodeClient{
		conn:       conn,
		nodeClient: ethpb.NewNodeClient(conn.GetGrpcClientConn()),
	}
	if provider := conn.GetGrpcConnectionProvider(); provider != nil {
		client.lastHost = provider.CurrentHost()
	}
	return client
}
