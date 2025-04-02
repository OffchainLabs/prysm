package shared_providers

import (
	"github.com/prysmaticlabs/prysm/v5/api/client"
)

func NewStateValidators(jsonRestHandler client.JsonRestHandler) StateValidators {
	return &stateValidatorsProvider{jsonRestHandler: jsonRestHandler}
}

func NewDuties(jsonRestHandler client.JsonRestHandler) Duties {
	return &dutiesProvider{jsonRestHandler: jsonRestHandler}
}

func NewGenesis(jsonRestHandler client.JsonRestHandler) Genesis {
	return &genesisProvider{jsonRestHandler: jsonRestHandler}
}
