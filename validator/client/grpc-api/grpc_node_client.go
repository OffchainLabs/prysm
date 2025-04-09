package grpc_api

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/health"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var (
	_ = iface.NodeClient(&grpcNodeClient{})
)

// Deprecated: gRPC API is being deprecated in favour of REST API.
type grpcNodeClient struct {
	nodeClient    ethpb.NodeClient
	healthTracker health.Tracker
}

// Deprecated: gRPC API is being deprecated in favour of REST API.
func (c *grpcNodeClient) SyncStatus(ctx context.Context, in *empty.Empty) (*ethpb.SyncStatus, error) {
	return c.nodeClient.GetSyncStatus(ctx, in)
}

// Deprecated: gRPC API is being deprecated in favour of REST API.
func (c *grpcNodeClient) Genesis(ctx context.Context, in *empty.Empty) (*ethpb.Genesis, error) {
	return c.nodeClient.GetGenesis(ctx, in)
}

// Deprecated: gRPC API is being deprecated in favour of REST API.
func (c *grpcNodeClient) Version(ctx context.Context, in *empty.Empty) (*ethpb.Version, error) {
	return c.nodeClient.GetVersion(ctx, in)
}

// Deprecated: gRPC API is being deprecated in favour of REST API.
func (c *grpcNodeClient) Peers(ctx context.Context, in *empty.Empty) (*ethpb.Peers, error) {
	return c.nodeClient.ListPeers(ctx, in)
}

// Deprecated: gRPC API is being deprecated in favour of REST API.
func (c *grpcNodeClient) IsHealthy(ctx context.Context) bool {
	_, err := c.nodeClient.GetHealth(ctx, &ethpb.HealthRequest{})
	if err != nil {
		log.WithError(err).Debug("failed to get health of node")
		return false
	}
	return true
}

// Deprecated: gRPC API is being deprecated in favour of REST API.
func (c *grpcNodeClient) HealthTracker() health.Tracker {
	return c.healthTracker
}

// Deprecated: gRPC API is being deprecated in favour of REST API.
func NewNodeClient(cc grpc.ClientConnInterface) iface.NodeClient {
	g := &grpcNodeClient{nodeClient: ethpb.NewNodeClient(cc)}
	g.healthTracker = health.NewTracker(g)
	return g
}
