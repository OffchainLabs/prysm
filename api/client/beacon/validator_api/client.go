package validator_api

import (
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/node"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/prysm_api"
	"github.com/prysmaticlabs/prysm/v5/api/client/beacon/shared_providers"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
)

func NewClient(
	validatorConn validatorHelpers.NodeConnection,
	jsonRestHandler client.JsonRestHandler,
	opt ...ValidatorClientOpt,
) Client {
	if features.Get().EnableBeaconRESTApi {
		return NewBeaconApiValidatorClient(jsonRestHandler, opt...)
	} else {
		return NewGrpcValidatorClient(validatorConn.GetGrpcClientConn())
	}
}

func NewBeaconApiValidatorClient(jsonRestHandler client.JsonRestHandler, opts ...ValidatorClientOpt) Client {
	c := &beaconApiValidatorClient{
		genesisProvider:         shared_providers.NewGenesis(jsonRestHandler),
		dutiesProvider:          shared_providers.NewDuties(jsonRestHandler),
		stateValidatorsProvider: shared_providers.NewStateValidators(jsonRestHandler),
		jsonRestHandler:         jsonRestHandler,
		beaconBlockConverter:    beaconApiBeaconBlockConverter{},
		prysmChainClient:        prysm_api.NewPrysmChainRestClient(jsonRestHandler, node.NewNodeClientWithFallback(jsonRestHandler, nil)), //TODO: this is really bad design...
		isEventStreamRunning:    false,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}
