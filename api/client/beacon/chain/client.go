package chain

import (
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/shared_providers"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
	"google.golang.org/grpc"
)

func NewClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler client.JsonRestHandler) Client {
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

func NewBeaconApiChainClientWithFallback(jsonRestHandler client.JsonRestHandler, fallbackClient Client) Client {
	return &beaconApiChainClient{
		jsonRestHandler:         jsonRestHandler,
		fallbackClient:          fallbackClient,
		stateValidatorsProvider: shared_providers.NewStateValidators(jsonRestHandler),
	}
}
