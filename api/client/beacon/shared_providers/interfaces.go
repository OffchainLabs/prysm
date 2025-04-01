package shared_providers

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
)

type Genesis interface {
	Genesis(ctx context.Context) (*structs.Genesis, error)
}

type StateValidators interface {
	StateValidators(context.Context, []string, []primitives.ValidatorIndex, []string) (*structs.GetValidatorsResponse, error)
	StateValidatorsForSlot(context.Context, primitives.Slot, []string, []primitives.ValidatorIndex, []string) (*structs.GetValidatorsResponse, error)
	StateValidatorsForHead(context.Context, []string, []primitives.ValidatorIndex, []string) (*structs.GetValidatorsResponse, error)
}

type Duties interface {
	AttesterDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*structs.AttesterDuty, error)
	ProposerDuties(ctx context.Context, epoch primitives.Epoch) ([]*structs.ProposerDuty, error)
	SyncDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*structs.SyncCommitteeDuty, error)
	Committees(ctx context.Context, epoch primitives.Epoch) ([]*structs.Committee, error)
}
