package grpc_api

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/api/client/beacon/health"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	"github.com/golang/protobuf/ptypes/empty"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var (
	_ = iface.NodeClient(&grpcNodeClient{})
)

type grpcNodeClient struct {
	nodeClient    ethpb.NodeClient
	healthTracker health.Tracker
}

func (c *grpcNodeClient) SyncStatus(ctx context.Context, in *empty.Empty) (*ethpb.SyncStatus, error) {
	return c.nodeClient.GetSyncStatus(ctx, in)
}

func (c *grpcNodeClient) Genesis(ctx context.Context, in *empty.Empty) (*ethpb.Genesis, error) {
	return c.nodeClient.GetGenesis(ctx, in)
}

func (c *grpcNodeClient) Version(ctx context.Context, in *empty.Empty) (*ethpb.Version, error) {
	return c.nodeClient.GetVersion(ctx, in)
}

func (c *grpcNodeClient) Peers(ctx context.Context, in *empty.Empty) (*ethpb.Peers, error) {
	return c.nodeClient.ListPeers(ctx, in)
}

func (c *grpcNodeClient) IsHealthy(ctx context.Context) bool {
	_, err := c.nodeClient.GetHealth(ctx, &ethpb.HealthRequest{})
	if err != nil {
		log.WithError(err).Debug("failed to get health of node")
		return false
	}
	return true
}

func (c *grpcNodeClient) HealthTracker() health.Tracker {
	return c.healthTracker
}

func NewNodeClient(cc grpc.ClientConnInterface) iface.NodeClient {
	g := &grpcNodeClient{nodeClient: ethpb.NewNodeClient(cc)}
	g.healthTracker = health.NewTracker(g)
	return g
}
