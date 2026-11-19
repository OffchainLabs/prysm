package grpc_api

import (
	"context"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/fallback"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// ReadyStatus reports whether the current beacon node can serve validator duties.
// GetHealth succeeds only if the node is synced and not optimistic; otherwise its
// error message tells why.
func (c *grpcNodeClient) ReadyStatus(ctx context.Context) fallback.NodeStatus {
	_, err := c.getClient().GetHealth(ctx, &ethpb.HealthRequest{})
	if err == nil {
		return fallback.NodeReady
	}
	log.WithError(err).WithField("url", api.RedactEndpoint(c.conn.GetGrpcConnectionProvider().CurrentHost())).Debug("Node is not ready")

	if s, ok := status.FromError(err); ok && s.Code() == codes.Unavailable {
		switch s.Message() {
		case api.HealthSyncingReason:
			return fallback.NodeSyncing
		case api.HealthOptimisticReason:
			return fallback.NodeOptimistic
		}
	}
	return fallback.NodeUnavailable
}

// NewNodeClient creates a new gRPC node client that supports
// dynamic connection switching via the NodeConnection's GrpcConnectionProvider.
func NewNodeClient(conn *validatorHelpers.NodeConnection) iface.NodeClient {
	return &grpcNodeClient{
		grpcClientManager: newGrpcClientManager(conn, ethpb.NewNodeClient),
	}
}
