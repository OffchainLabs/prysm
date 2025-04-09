package grpc_api

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	"google.golang.org/grpc"
)

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
type grpcChainClient struct {
	beaconChainClient ethpb.BeaconChainClient
}

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
func (c *grpcChainClient) ChainHead(ctx context.Context, in *empty.Empty) (*ethpb.ChainHead, error) {
	return c.beaconChainClient.GetChainHead(ctx, in)
}

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
func (c *grpcChainClient) ValidatorBalances(ctx context.Context, in *ethpb.ListValidatorBalancesRequest) (*ethpb.ValidatorBalances, error) {
	return c.beaconChainClient.ListValidatorBalances(ctx, in)
}

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
func (c *grpcChainClient) Validators(ctx context.Context, in *ethpb.ListValidatorsRequest) (*ethpb.Validators, error) {
	return c.beaconChainClient.ListValidators(ctx, in)
}

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
func (c *grpcChainClient) ValidatorQueue(ctx context.Context, in *empty.Empty) (*ethpb.ValidatorQueue, error) {
	return c.beaconChainClient.GetValidatorQueue(ctx, in)
}

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
func (c *grpcChainClient) ValidatorPerformance(ctx context.Context, in *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error) {
	return c.beaconChainClient.GetValidatorPerformance(ctx, in)
}

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
func (c *grpcChainClient) ValidatorParticipation(ctx context.Context, in *ethpb.GetValidatorParticipationRequest) (*ethpb.ValidatorParticipationResponse, error) {
	return c.beaconChainClient.GetValidatorParticipation(ctx, in)
}

// Deprecated: gRPC API will still be supported for some time, most likely until v8 in 2026, but will be eventually removed in favor of REST API.
func NewGrpcChainClient(cc grpc.ClientConnInterface) iface.ChainClient {
	return &grpcChainClient{ethpb.NewBeaconChainClient(cc)}
}
