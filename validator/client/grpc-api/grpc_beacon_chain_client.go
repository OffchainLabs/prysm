package grpc_api

import (
	"context"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
)

type grpcChainClient struct {
	*grpcClientManager[ethpb.BeaconChainClient]
}

func (c *grpcChainClient) ValidatorPerformance(ctx context.Context, in *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error) {
	return c.getClient().GetValidatorPerformance(ctx, in)
}

// NewGrpcChainClient creates a new gRPC chain client that supports
// dynamic connection switching via the NodeConnection's GrpcConnectionProvider.
func NewGrpcChainClient(conn *validatorHelpers.NodeConnection) iface.ChainClient {
	return &grpcChainClient{
		grpcClientManager: newGrpcClientManager(conn, ethpb.NewBeaconChainClient),
	}
}
