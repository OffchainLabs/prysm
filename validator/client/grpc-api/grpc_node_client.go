package grpc_api

import (
	"context"
	"time"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
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

func (c *grpcNodeClient) IsReady(ctx context.Context) bool {
	// GetHealth returns 200 OK only if node is synced and not optimistic.
	// otherwise it will throw an error
	start := time.Now()
	_, err := c.getClient().GetHealth(ctx, &ethpb.HealthRequest{})
	if err != nil {
		fields := logrus.Fields{
			"url":      c.conn.GetGrpcConnectionProvider().CurrentHost(),
			"duration": time.Since(start),
		}
		if s, ok := status.FromError(err); ok {
			fields["grpcCode"] = s.Code().String()
		}
		log.WithError(err).WithFields(fields).Debug("Node is not ready")
		return false
	}
	log.WithFields(logrus.Fields{
		"url":      c.conn.GetGrpcConnectionProvider().CurrentHost(),
		"duration": time.Since(start),
	}).Debug("Beacon node health request completed")
	return true
}

// NewNodeClient creates a new gRPC node client that supports
// dynamic connection switching via the NodeConnection's GrpcConnectionProvider.
func NewNodeClient(conn validatorHelpers.NodeConnection) iface.NodeClient {
	return &grpcNodeClient{
		grpcClientManager: newGrpcClientManager(conn, ethpb.NewNodeClient),
	}
}
