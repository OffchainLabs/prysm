package node_client_factory

import (
	"github.com/OffchainLabs/prysm/v7/config/features"
	beaconApi "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api"
	grpcApi "github.com/OffchainLabs/prysm/v7/validator/client/grpc-api"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v7/validator/helpers"
)

func NewNodeClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler beaconApi.RestHandler) iface.NodeClient {
	// Use connection-aware client if a connection provider is configured for gRPC failover support
	var grpcClient iface.NodeClient
	if validatorConn.GetGrpcConnectionProvider() != nil {
		grpcClient = grpcApi.NewNodeClientWithConnection(validatorConn)
	} else {
		grpcClient = grpcApi.NewNodeClient(validatorConn.GetGrpcClientConn())
	}
	if features.Get().EnableBeaconRESTApi {
		return beaconApi.NewNodeClientWithFallback(jsonRestHandler, grpcClient)
	} else {
		return grpcClient
	}
}
