package chain

import (
	"github.com/prysmaticlabs/prysm/v5/config/features"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	beaconApi "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
	"google.golang.org/grpc"
)

func NewChainClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler beaconApi.JsonRestHandler) Client {
	grpcClient := NewGrpcChainClient(validatorConn.GetGrpcClientConn())
	if features.Get().EnableBeaconRESTApi {
		return NewBeaconApiChainClientWithFallback(jsonRestHandler, grpcClient)
	} else {
		return grpcClient
	}
}

func NewGrpcChainClient(cc grpc.ClientConnInterface) Client {
	return &grpcChainClient{ethpb.NewBeaconChainClient(cc)}
}
