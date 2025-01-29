package lightclient

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"

	lightclient "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/light-client"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
)

func newLightClientFinalityUpdateFromBeaconState(
	ctx context.Context,
	currentSlot primitives.Slot,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState,
	attestedBlock interfaces.ReadOnlySignedBeaconBlock,
	finalizedBlock interfaces.ReadOnlySignedBeaconBlock,
) (interfaces.LightClientFinalityUpdate, error) {
	return lightclient.NewLightClientFinalityUpdateFromBeaconState(ctx, currentSlot, state, block, attestedState, attestedBlock, finalizedBlock)
}

func newLightClientOptimisticUpdateFromBeaconState(
	ctx context.Context,
	currentSlot primitives.Slot,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState,
	attestedBlock interfaces.ReadOnlySignedBeaconBlock,
) (interfaces.LightClientOptimisticUpdate, error) {
	return lightclient.NewLightClientOptimisticUpdateFromBeaconState(ctx, currentSlot, state, block, attestedState, attestedBlock)
}
