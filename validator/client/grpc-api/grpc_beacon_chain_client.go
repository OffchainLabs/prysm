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

type grpcChainClient struct {
	beaconChainClient ethpb.BeaconChainClient
	conn              validatorHelpers.NodeConnection
	lastHost          string
	clientMu          sync.RWMutex
}

// getClient returns the current chain client, recreating it if the connection has changed.
func (c *grpcChainClient) getClient() ethpb.BeaconChainClient {
	if c.conn == nil || c.conn.GetGrpcConnectionProvider() == nil {
		// No connection provider, use static client
		return c.beaconChainClient
	}

	currentHost := c.conn.GetGrpcConnectionProvider().CurrentHost()
	c.clientMu.RLock()
	if c.lastHost == currentHost {
		client := c.beaconChainClient
		c.clientMu.RUnlock()
		return client
	}
	c.clientMu.RUnlock()

	// Connection changed, need to recreate client
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	// Double-check after acquiring write lock
	if c.lastHost == currentHost {
		return c.beaconChainClient
	}
	c.beaconChainClient = ethpb.NewBeaconChainClient(c.conn.GetGrpcClientConn())
	c.lastHost = currentHost
	return c.beaconChainClient
}

func (c *grpcChainClient) ChainHead(ctx context.Context, in *empty.Empty) (*ethpb.ChainHead, error) {
	return c.getClient().GetChainHead(ctx, in)
}

func (c *grpcChainClient) ValidatorBalances(ctx context.Context, in *ethpb.ListValidatorBalancesRequest) (*ethpb.ValidatorBalances, error) {
	return c.getClient().ListValidatorBalances(ctx, in)
}

func (c *grpcChainClient) Validators(ctx context.Context, in *ethpb.ListValidatorsRequest) (*ethpb.Validators, error) {
	return c.getClient().ListValidators(ctx, in)
}

func (c *grpcChainClient) ValidatorQueue(ctx context.Context, in *empty.Empty) (*ethpb.ValidatorQueue, error) {
	return c.getClient().GetValidatorQueue(ctx, in)
}

func (c *grpcChainClient) ValidatorPerformance(ctx context.Context, in *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error) {
	return c.getClient().GetValidatorPerformance(ctx, in)
}

func (c *grpcChainClient) ValidatorParticipation(ctx context.Context, in *ethpb.GetValidatorParticipationRequest) (*ethpb.ValidatorParticipationResponse, error) {
	return c.getClient().GetValidatorParticipation(ctx, in)
}

// NewGrpcChainClient creates a new gRPC chain client from a single connection.
// This is the legacy constructor for backward compatibility.
func NewGrpcChainClient(cc grpc.ClientConnInterface) iface.ChainClient {
	return &grpcChainClient{beaconChainClient: ethpb.NewBeaconChainClient(cc)}
}

// NewGrpcChainClientWithConnection creates a new gRPC chain client that supports
// dynamic connection switching via the NodeConnection's GrpcConnectionProvider.
func NewGrpcChainClientWithConnection(conn validatorHelpers.NodeConnection) iface.ChainClient {
	client := &grpcChainClient{
		conn:              conn,
		beaconChainClient: ethpb.NewBeaconChainClient(conn.GetGrpcClientConn()),
	}
	if provider := conn.GetGrpcConnectionProvider(); provider != nil {
		client.lastHost = provider.CurrentHost()
	}
	return client
}
