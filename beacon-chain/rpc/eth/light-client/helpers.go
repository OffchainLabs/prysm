package lightclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"

	lightclient "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/light-client"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

func createLightClientBootstrap(
	ctx context.Context,
	currentSlot primitives.Slot,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
) (*structs.LightClientBootstrap, error) {
	// assert compute_epoch_at_slot(state.slot) >= ALTAIR_FORK_EPOCH
	if slots.ToEpoch(state.Slot()) < params.BeaconConfig().AltairForkEpoch {
		return nil, fmt.Errorf("light client bootstrap is not supported before Altair, invalid slot %d", state.Slot())
	}

	// assert state.slot == state.latest_block_header.slot
	latestBlockHeader := state.LatestBlockHeader()
	if state.Slot() != latestBlockHeader.Slot {
		return nil, fmt.Errorf("state slot %d not equal to latest block header slot %d", state.Slot(), latestBlockHeader.Slot)
	}

	// header.state_root = hash_tree_root(state)
	stateRoot, err := state.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get state root")
	}
	latestBlockHeader.StateRoot = stateRoot[:]

	// assert hash_tree_root(header) == hash_tree_root(block.message)
	latestBlockHeaderRoot, err := latestBlockHeader.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get latest block header root")
	}
	beaconBlockRoot, err := block.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get block root")
	}
	if latestBlockHeaderRoot != beaconBlockRoot {
		return nil, fmt.Errorf("latest block header root %#x not equal to block root %#x", latestBlockHeaderRoot, beaconBlockRoot)
	}

	lightClientHeader, err := lightclient.BlockToLightClientHeader(ctx, currentSlot, block)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert block to light client header")
	}

	apiLightClientHeader := &structs.LightClientHeader{
		Beacon: structs.BeaconBlockHeaderFromConsensus(lightClientHeader.Beacon()),
	}

	headerJSON, err := json.Marshal(apiLightClientHeader)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert header to raw message")
	}
	currentSyncCommittee, err := state.CurrentSyncCommittee()
	if err != nil {
		return nil, errors.Wrap(err, "could not get current sync committee")
	}
	currentSyncCommitteeProof, err := state.CurrentSyncCommitteeProof(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get current sync committee proof")
	}

	var branch []string
	if state.Version() >= version.Electra {
		branch = make([]string, fieldparams.SyncCommitteeBranchDepthElectra)
	} else {
		branch = make([]string, fieldparams.SyncCommitteeBranchDepth)
	}
	for i, proof := range currentSyncCommitteeProof {
		branch[i] = hexutil.Encode(proof)
	}

	result := &structs.LightClientBootstrap{
		Header:                     headerJSON,
		CurrentSyncCommittee:       structs.SyncCommitteeFromConsensus(currentSyncCommittee),
		CurrentSyncCommitteeBranch: branch,
	}

	return result, nil
}

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
