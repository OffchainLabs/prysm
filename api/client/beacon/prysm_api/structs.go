package prysm_api

import (
	"github.com/prysmaticlabs/prysm/v5/config/features"
	beaconApi "github.com/prysmaticlabs/prysm/v5/validator/client/beacon-api"
	nodeClientFactory "github.com/prysmaticlabs/prysm/v5/validator/client/node-client-factory"
	validatorHelpers "github.com/prysmaticlabs/prysm/v5/validator/helpers"
)

type ValidatorCount struct {
	Status string
	Count  uint64
}
