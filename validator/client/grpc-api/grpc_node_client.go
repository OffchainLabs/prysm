package grpc_api

import (
	"context"

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
	*grpcClientManager[ethpb.NodeClient]
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
	return &grpcNodeClient{
		grpcClientManager: &grpcClientManager[ethpb.NodeClient]{
			client:    ethpb.NewNodeClient(cc),
			newClient: ethpb.NewNodeClient,
		},
	}
}

// NewNodeClientWithConnection creates a new gRPC node client that supports
// dynamic connection switching via the NodeConnection's GrpcConnectionProvider.
func NewNodeClientWithConnection(conn validatorHelpers.NodeConnection) iface.NodeClient {
	return &grpcNodeClient{
		grpcClientManager: newGrpcClientManager(conn, ethpb.NewNodeClient),
	}
}
