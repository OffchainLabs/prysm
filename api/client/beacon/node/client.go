package node

import (
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/health"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/shared_providers"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
)

func NewClient(validatorConn validatorHelpers.NodeConnection, jsonRestHandler client.JsonRestHandler) Client {
	grpcClient := NewNodeClient(validatorConn.GetGrpcClientConn())
	if features.Get().EnableBeaconRESTApi {
		return NewNodeClientWithFallback(jsonRestHandler, grpcClient)
	} else {
		return grpcClient
	}
}

func NewNodeClientWithFallback(jsonRestHandler client.JsonRestHandler, fallbackClient Client) Client {
	b := &beaconapiNodeClient{
		jsonRestHandler: jsonRestHandler,
		fallbackClient:  fallbackClient,
		genesisProvider: shared_providers.NewGenesis(jsonRestHandler),
	}
	b.healthTracker = health.NewTracker(b)
	return b
}
