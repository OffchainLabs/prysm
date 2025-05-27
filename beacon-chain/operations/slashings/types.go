package slashings

import (
	"context"
	"sync"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

// PoolInserter is capable of inserting new slashing objects into the operations pool.
type PoolInserter interface {
	InsertAttesterSlashing(
		ctx context.Context,
		state state.ReadOnlyBeaconState,
		slashing ethpb.AttSlashing,
	) error
	InsertProposerSlashing(
		ctx context.Context,
		state state.ReadOnlyBeaconState,
		slashing *ethpb.ProposerSlashing,
	) error
}

// PoolManager maintains a pool of pending and recently included attester and proposer slashings.
// This pool is used by proposers to insert data into new blocks.
type PoolManager interface {
	PoolInserter
	PendingAttesterSlashings(ctx context.Context, state state.ReadOnlyBeaconState, noLimit bool) []ethpb.AttSlashing
	PendingProposerSlashings(ctx context.Context, state state.ReadOnlyBeaconState, noLimit bool) []*ethpb.ProposerSlashing
	MarkIncludedAttesterSlashing(as ethpb.AttSlashing)
	MarkIncludedProposerSlashing(ps *ethpb.ProposerSlashing)
	ConvertToElectra()
}

// Option for pool service configuration.
type Option func(p *PoolService) error

// PoolService manages the Pool.
type PoolService struct {
	ctx             context.Context
	cancel          context.CancelFunc
	poolManager     PoolManager
	currentSlotFn   func() primitives.Slot
	cw              startup.ClockWaiter
	clock           *startup.Clock
	runElectraTimer bool
}

// Pool is a concrete implementation of PoolManager.
type Pool struct {
	lock                    sync.RWMutex
	pendingProposerSlashing []*ethpb.ProposerSlashing
	pendingAttesterSlashing []*PendingAttesterSlashing
	included                map[primitives.ValidatorIndex]bool
}

// PendingAttesterSlashing represents an attester slashing in the operation pool.
// Allows for easy binary searching of included validator indexes.
type PendingAttesterSlashing struct {
	attesterSlashing ethpb.AttSlashing
	validatorToSlash primitives.ValidatorIndex
}
