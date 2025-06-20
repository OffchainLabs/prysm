package node_client_factory

import (
	"github.com/OffchainLabs/prysm/v6/config/features"
	beaconApi "github.com/OffchainLabs/prysm/v6/validator/client/beacon-api"
	grpcApi "github.com/OffchainLabs/prysm/v6/validator/client/grpc-api"
	"github.com/OffchainLabs/prysm/v6/validator/client/iface"
	validatorHelpers "github.com/OffchainLabs/prysm/v6/validator/helpers"
)

func NewNodeClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler beaconApi.RestHandler) iface.NodeClient {
	grpcClient := grpcApi.NewNodeClient(validatorConn.GetGrpcClientConn())
	if features.Get().EnableBeaconRESTApi {
		return beaconApi.NewNodeClientWithFallback(jsonRestHandler, grpcClient)
	} else {
		return grpcClient
	}
}
