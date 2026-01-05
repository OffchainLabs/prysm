package grpc_api

import (
	"context"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
)

type grpcChainClient struct {
	*grpcClientManager[ethpb.BeaconChainClient]
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
	return &grpcChainClient{
		grpcClientManager: &grpcClientManager[ethpb.BeaconChainClient]{
			client:    ethpb.NewBeaconChainClient(cc),
			newClient: ethpb.NewBeaconChainClient,
		},
	}
}

// NewGrpcChainClientWithConnection creates a new gRPC chain client that supports
// dynamic connection switching via the NodeConnection's GrpcConnectionProvider.
func NewGrpcChainClientWithConnection(conn validatorHelpers.NodeConnection) iface.ChainClient {
	return &grpcChainClient{
		grpcClientManager: newGrpcClientManager(conn, ethpb.NewBeaconChainClient),
	}
}
