package grpc_api

import (
	"context"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
)

type grpcPrysmChainClient struct {
	chainClient iface.ChainClient
}

func (c *grpcPrysmChainClient) ValidatorPerformance(ctx context.Context, in *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error) {
	return c.chainClient.ValidatorPerformance(ctx, in)
}

// NewGrpcPrysmChainClient creates a new gRPC Prysm chain client that supports
// dynamic connection switching via the NodeConnection's GrpcConnectionProvider.
func NewGrpcPrysmChainClient(conn validatorHelpers.NodeConnection) iface.PrysmChainClient {
	return &grpcPrysmChainClient{chainClient: NewGrpcChainClient(conn)}
}
