package shared_providers

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type StateValidatorsProvider interface {
	StateValidators(context.Context, []string, []primitives.ValidatorIndex, []string) (*structs.GetValidatorsResponse, error)
	StateValidatorsForSlot(context.Context, primitives.Slot, []string, []primitives.ValidatorIndex, []string) (*structs.GetValidatorsResponse, error)
	StateValidatorsForHead(context.Context, []string, []primitives.ValidatorIndex, []string) (*structs.GetValidatorsResponse, error)
}
