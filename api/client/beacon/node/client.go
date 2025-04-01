package node

import (
	"github.com/prysmaticlabs/prysm/v5/config/features"
	beaconApi "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
)

func NewNodeClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler beaconApi.JsonRestHandler) NodeClient {
	grpcClient := NewNodeClient(validatorConn.GetGrpcClientConn())
	if features.Get().EnableBeaconRESTApi {
		return NewNodeClientWithFallback(jsonRestHandler, grpcClient)
	} else {
		return grpcClient
	}
}
