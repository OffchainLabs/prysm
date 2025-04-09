package node_client_factory

import (
	"github.com/prysmaticlabs/prysm/v5/config/features"
	beaconApi "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api"
	grpcApi "github.com/prysmaticlabs/prysm/v5/validator/client/grpc-api"
	"github.com/prysmaticlabs/prysm/v5/validator/client/iface"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
)

func NewNodeClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler beaconApi.JsonRestHandler) iface.NodeClient {
	grpcClient := grpcApi.NewNodeClient(validatorConn.GetGrpcClientConn())
	if features.Get().EnableBeaconRESTApi {
		return beaconApi.NewNodeClientWithFallback(jsonRestHandler, grpcClient)
	} else {
		return grpcClient
	}
}
