package lightclient

import (
	"context"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"

	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
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
) (*structs.LightClientFinalityUpdate, error) {
	result, err := lightclient.NewLightClientFinalityUpdateFromBeaconState(ctx, currentSlot, state, block, attestedState, attestedBlock, finalizedBlock)
	if err != nil {
		return nil, err
	}

	return structs.LightClientFinalityUpdateFromConsensus(result)
}

func newLightClientOptimisticUpdateFromBeaconState(
	ctx context.Context,
	currentSlot primitives.Slot,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState,
	attestedBlock interfaces.ReadOnlySignedBeaconBlock,
) (*structs.LightClientOptimisticUpdate, error) {
	result, err := lightclient.NewLightClientOptimisticUpdateFromBeaconState(ctx, currentSlot, state, block, attestedState, attestedBlock)
	if err != nil {
		return nil, err
	}

	return structs.LightClientOptimisticUpdateFromConsensus(result)
}
