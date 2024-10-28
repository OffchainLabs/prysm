package util

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	consensus_types "github.com/prysmaticlabs/prysm/v5/consensus-types"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/ssz"
	v11 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

type TestLightClient struct {
	T              *testing.T
	Ctx            context.Context
	State          state.BeaconState
	Block          interfaces.ReadOnlySignedBeaconBlock
	AttestedState  state.BeaconState
	AttestedBlock  interfaces.ReadOnlySignedBeaconBlock
	FinalizedBlock interfaces.ReadOnlySignedBeaconBlock
}

func NewTestLightClient(t *testing.T) *TestLightClient {
	return &TestLightClient{T: t}
}

func (l *TestLightClient) SetupTestCapella(blinded bool) *TestLightClient {
	ctx := context.Background()

	slot := primitives.Slot(params.BeaconConfig().CapellaForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)

	attestedState, err := NewBeaconStateCapella()
	require.NoError(l.T, err)
	err = attestedState.SetSlot(slot)
	require.NoError(l.T, err)

	finalizedBlock, err := blocks.NewSignedBeaconBlock(NewBeaconBlockCapella())
	require.NoError(l.T, err)
	finalizedBlock.SetSlot(primitives.Slot(params.BeaconConfig().CapellaForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)))
	finalizedHeader, err := finalizedBlock.Header()
	require.NoError(l.T, err)
	finalizedRoot, err := finalizedHeader.Header.HashTreeRoot()
	require.NoError(l.T, err)

	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: params.BeaconConfig().CapellaForkEpoch,
		Root:  finalizedRoot[:],
	}))

	parent := NewBeaconBlockCapella()
	parent.Block.Slot = slot

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	parentHeader, err := signedParent.Header()
	require.NoError(l.T, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(l.T, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	state, err := NewBeaconStateCapella()
	require.NoError(l.T, err)
	err = state.SetSlot(slot)
	require.NoError(l.T, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(l.T, err)

	var signedBlock interfaces.SignedBeaconBlock
	if blinded {
		block := NewBlindedBeaconBlockCapella()
		block.Block.Slot = slot
		block.Block.ParentRoot = parentRoot[:]

		for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
			block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)

		h, err := signedBlock.Header()
		require.NoError(l.T, err)

		err = state.SetLatestBlockHeader(h.Header)
		require.NoError(l.T, err)
		stateRoot, err := state.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		// get a new signed block so the root is updated with the new state root
		block.Block.StateRoot = stateRoot[:]
		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)
	} else {
		block := NewBeaconBlockCapella()
		block.Block.Slot = slot
		block.Block.ParentRoot = parentRoot[:]

		for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
			block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)

		h, err := signedBlock.Header()
		require.NoError(l.T, err)

		err = state.SetLatestBlockHeader(h.Header)
		require.NoError(l.T, err)
		stateRoot, err := state.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		// get a new signed block so the root is updated with the new state root
		block.Block.StateRoot = stateRoot[:]
		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)
	}

	l.State = state
	l.AttestedState = attestedState
	l.AttestedBlock = signedParent
	l.Block = signedBlock
	l.Ctx = ctx
	l.FinalizedBlock = finalizedBlock

	return l
}

func (l *TestLightClient) SetupTestCapellaFinalizedBlockAltair(blinded bool) *TestLightClient {
	ctx := context.Background()

	slot := primitives.Slot(params.BeaconConfig().CapellaForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)

	attestedState, err := NewBeaconStateCapella()
	require.NoError(l.T, err)
	err = attestedState.SetSlot(slot)
	require.NoError(l.T, err)

	finalizedBlock, err := blocks.NewSignedBeaconBlock(NewBeaconBlockAltair())
	require.NoError(l.T, err)
	finalizedBlock.SetSlot(1)
	finalizedHeader, err := finalizedBlock.Header()
	require.NoError(l.T, err)
	finalizedRoot, err := finalizedHeader.Header.HashTreeRoot()
	require.NoError(l.T, err)

	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: params.BeaconConfig().AltairForkEpoch - 10,
		Root:  finalizedRoot[:],
	}))

	parent := NewBeaconBlockCapella()
	parent.Block.Slot = slot

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	parentHeader, err := signedParent.Header()
	require.NoError(l.T, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(l.T, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	state, err := NewBeaconStateCapella()
	require.NoError(l.T, err)
	err = state.SetSlot(slot)
	require.NoError(l.T, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(l.T, err)

	var signedBlock interfaces.SignedBeaconBlock
	if blinded {
		block := NewBlindedBeaconBlockCapella()
		block.Block.Slot = slot
		block.Block.ParentRoot = parentRoot[:]

		for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
			block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)

		h, err := signedBlock.Header()
		require.NoError(l.T, err)

		err = state.SetLatestBlockHeader(h.Header)
		require.NoError(l.T, err)
		stateRoot, err := state.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		// get a new signed block so the root is updated with the new state root
		block.Block.StateRoot = stateRoot[:]
		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)
	} else {
		block := NewBeaconBlockCapella()
		block.Block.Slot = slot
		block.Block.ParentRoot = parentRoot[:]

		for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
			block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)

		h, err := signedBlock.Header()
		require.NoError(l.T, err)

		err = state.SetLatestBlockHeader(h.Header)
		require.NoError(l.T, err)
		stateRoot, err := state.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		// get a new signed block so the root is updated with the new state root
		block.Block.StateRoot = stateRoot[:]
		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)
	}

	l.State = state
	l.AttestedState = attestedState
	l.AttestedBlock = signedParent
	l.Block = signedBlock
	l.Ctx = ctx
	l.FinalizedBlock = finalizedBlock

	return l
}

func (l *TestLightClient) SetupTestAltair() *TestLightClient {
	ctx := context.Background()

	slot := primitives.Slot(params.BeaconConfig().AltairForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)

	attestedState, err := NewBeaconStateAltair()
	require.NoError(l.T, err)
	err = attestedState.SetSlot(slot)
	require.NoError(l.T, err)

	finalizedBlock, err := blocks.NewSignedBeaconBlock(NewBeaconBlockAltair())
	require.NoError(l.T, err)
	finalizedBlock.SetSlot(1)
	finalizedHeader, err := finalizedBlock.Header()
	require.NoError(l.T, err)
	finalizedRoot, err := finalizedHeader.Header.HashTreeRoot()
	require.NoError(l.T, err)

	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: params.BeaconConfig().AltairForkEpoch - 10,
		Root:  finalizedRoot[:],
	}))

	parent := NewBeaconBlockAltair()
	parent.Block.Slot = slot

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	parentHeader, err := signedParent.Header()
	require.NoError(l.T, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(l.T, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	state, err := NewBeaconStateAltair()
	require.NoError(l.T, err)
	err = state.SetSlot(slot)
	require.NoError(l.T, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(l.T, err)

	block := NewBeaconBlockAltair()
	block.Block.Slot = slot
	block.Block.ParentRoot = parentRoot[:]

	for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
		block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(l.T, err)

	h, err := signedBlock.Header()
	require.NoError(l.T, err)

	err = state.SetLatestBlockHeader(h.Header)
	require.NoError(l.T, err)
	stateRoot, err := state.HashTreeRoot(ctx)
	require.NoError(l.T, err)

	// get a new signed block so the root is updated with the new state root
	block.Block.StateRoot = stateRoot[:]
	signedBlock, err = blocks.NewSignedBeaconBlock(block)
	require.NoError(l.T, err)

	l.State = state
	l.AttestedState = attestedState
	l.Block = signedBlock
	l.Ctx = ctx
	l.FinalizedBlock = finalizedBlock
	l.AttestedBlock = signedParent

	return l
}

func (l *TestLightClient) SetupTestBellatrix() *TestLightClient {
	ctx := context.Background()

	slot := primitives.Slot(params.BeaconConfig().BellatrixForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)

	attestedState, err := NewBeaconStateBellatrix()
	require.NoError(l.T, err)
	err = attestedState.SetSlot(slot)
	require.NoError(l.T, err)

	finalizedBlock, err := blocks.NewSignedBeaconBlock(NewBeaconBlockBellatrix())
	require.NoError(l.T, err)
	finalizedBlock.SetSlot(1)
	finalizedHeader, err := finalizedBlock.Header()
	require.NoError(l.T, err)
	finalizedRoot, err := finalizedHeader.Header.HashTreeRoot()
	require.NoError(l.T, err)

	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: params.BeaconConfig().BellatrixForkEpoch - 10,
		Root:  finalizedRoot[:],
	}))

	parent := NewBeaconBlockBellatrix()
	parent.Block.Slot = slot

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	parentHeader, err := signedParent.Header()
	require.NoError(l.T, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(l.T, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	state, err := NewBeaconStateBellatrix()
	require.NoError(l.T, err)
	err = state.SetSlot(slot)
	require.NoError(l.T, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(l.T, err)

	block := NewBeaconBlockBellatrix()
	block.Block.Slot = slot
	block.Block.ParentRoot = parentRoot[:]

	for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
		block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedBlock, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(l.T, err)

	h, err := signedBlock.Header()
	require.NoError(l.T, err)

	err = state.SetLatestBlockHeader(h.Header)
	require.NoError(l.T, err)
	stateRoot, err := state.HashTreeRoot(ctx)
	require.NoError(l.T, err)

	// get a new signed block so the root is updated with the new state root
	block.Block.StateRoot = stateRoot[:]
	signedBlock, err = blocks.NewSignedBeaconBlock(block)
	require.NoError(l.T, err)

	l.State = state
	l.AttestedState = attestedState
	l.Block = signedBlock
	l.Ctx = ctx
	l.FinalizedBlock = finalizedBlock
	l.AttestedBlock = signedParent

	return l
}

func (l *TestLightClient) SetupTestDeneb(blinded bool) *TestLightClient {
	ctx := context.Background()

	slot := primitives.Slot(params.BeaconConfig().DenebForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)

	attestedState, err := NewBeaconStateDeneb()
	require.NoError(l.T, err)
	err = attestedState.SetSlot(slot)
	require.NoError(l.T, err)

	finalizedBlock, err := blocks.NewSignedBeaconBlock(NewBeaconBlockDeneb())
	require.NoError(l.T, err)
	finalizedBlock.SetSlot(primitives.Slot(params.BeaconConfig().DenebForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)))
	finalizedHeader, err := finalizedBlock.Header()
	require.NoError(l.T, err)
	finalizedRoot, err := finalizedHeader.Header.HashTreeRoot()
	require.NoError(l.T, err)

	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: params.BeaconConfig().DenebForkEpoch,
		Root:  finalizedRoot[:],
	}))

	parent := NewBeaconBlockDeneb()
	parent.Block.Slot = slot

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	parentHeader, err := signedParent.Header()
	require.NoError(l.T, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(l.T, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	state, err := NewBeaconStateDeneb()
	require.NoError(l.T, err)
	err = state.SetSlot(slot)
	require.NoError(l.T, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(l.T, err)

	var signedBlock interfaces.SignedBeaconBlock
	if blinded {
		block := NewBlindedBeaconBlockDeneb()
		block.Message.Slot = slot
		block.Message.ParentRoot = parentRoot[:]

		for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
			block.Message.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)

		h, err := signedBlock.Header()
		require.NoError(l.T, err)

		err = state.SetLatestBlockHeader(h.Header)
		require.NoError(l.T, err)
		stateRoot, err := state.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		// get a new signed block so the root is updated with the new state root
		block.Message.StateRoot = stateRoot[:]
		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)
	} else {
		block := NewBeaconBlockDeneb()
		block.Block.Slot = slot
		block.Block.ParentRoot = parentRoot[:]

		for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
			block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)

		h, err := signedBlock.Header()
		require.NoError(l.T, err)

		err = state.SetLatestBlockHeader(h.Header)
		require.NoError(l.T, err)
		stateRoot, err := state.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		// get a new signed block so the root is updated with the new state root
		block.Block.StateRoot = stateRoot[:]
		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)
	}

	l.State = state
	l.AttestedState = attestedState
	l.AttestedBlock = signedParent
	l.Block = signedBlock
	l.Ctx = ctx
	l.FinalizedBlock = finalizedBlock

	return l
}

func (l *TestLightClient) SetupTestElectra(blinded bool) *TestLightClient {
	ctx := context.Background()

	slot := primitives.Slot(params.BeaconConfig().ElectraForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)

	attestedState, err := NewBeaconStateElectra()
	require.NoError(l.T, err)
	err = attestedState.SetSlot(slot)
	require.NoError(l.T, err)

	finalizedBlock, err := blocks.NewSignedBeaconBlock(NewBeaconBlockElectra())
	require.NoError(l.T, err)
	finalizedBlock.SetSlot(1)
	finalizedHeader, err := finalizedBlock.Header()
	require.NoError(l.T, err)
	finalizedRoot, err := finalizedHeader.Header.HashTreeRoot()
	require.NoError(l.T, err)

	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: params.BeaconConfig().ElectraForkEpoch - 10,
		Root:  finalizedRoot[:],
	}))

	parent := NewBeaconBlockElectra()
	parent.Block.Slot = slot

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	parentHeader, err := signedParent.Header()
	require.NoError(l.T, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(l.T, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	state, err := NewBeaconStateElectra()
	require.NoError(l.T, err)
	err = state.SetSlot(slot)
	require.NoError(l.T, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(l.T, err)

	var signedBlock interfaces.SignedBeaconBlock
	if blinded {
		block := NewBlindedBeaconBlockElectra()
		block.Message.Slot = slot
		block.Message.ParentRoot = parentRoot[:]

		for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
			block.Message.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)

		h, err := signedBlock.Header()
		require.NoError(l.T, err)

		err = state.SetLatestBlockHeader(h.Header)
		require.NoError(l.T, err)
		stateRoot, err := state.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		// get a new signed block so the root is updated with the new state root
		block.Message.StateRoot = stateRoot[:]
		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)
	} else {
		block := NewBeaconBlockElectra()
		block.Block.Slot = slot
		block.Block.ParentRoot = parentRoot[:]

		for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
			block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)

		h, err := signedBlock.Header()
		require.NoError(l.T, err)

		err = state.SetLatestBlockHeader(h.Header)
		require.NoError(l.T, err)
		stateRoot, err := state.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		// get a new signed block so the root is updated with the new state root
		block.Block.StateRoot = stateRoot[:]
		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)
	}

	l.State = state
	l.AttestedState = attestedState
	l.AttestedBlock = signedParent
	l.Block = signedBlock
	l.Ctx = ctx
	l.FinalizedBlock = finalizedBlock

	return l
}

func (l *TestLightClient) SetupTestDenebFinalizedBlockCapella(blinded bool) *TestLightClient {
	ctx := context.Background()

	slot := primitives.Slot(params.BeaconConfig().DenebForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)

	attestedState, err := NewBeaconStateDeneb()
	require.NoError(l.T, err)
	err = attestedState.SetSlot(slot)
	require.NoError(l.T, err)

	finalizedBlock, err := blocks.NewSignedBeaconBlock(NewBeaconBlockCapella())
	require.NoError(l.T, err)
	finalizedBlock.SetSlot(primitives.Slot(params.BeaconConfig().DenebForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Sub(15))
	finalizedHeader, err := finalizedBlock.Header()
	require.NoError(l.T, err)
	finalizedRoot, err := finalizedHeader.Header.HashTreeRoot()
	require.NoError(l.T, err)

	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: params.BeaconConfig().DenebForkEpoch - 1,
		Root:  finalizedRoot[:],
	}))

	parent := NewBeaconBlockDeneb()
	parent.Block.Slot = slot

	signedParent, err := blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	parentHeader, err := signedParent.Header()
	require.NoError(l.T, err)
	attestedHeader := parentHeader.Header

	err = attestedState.SetLatestBlockHeader(attestedHeader)
	require.NoError(l.T, err)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)

	// get a new signed block so the root is updated with the new state root
	parent.Block.StateRoot = attestedStateRoot[:]
	signedParent, err = blocks.NewSignedBeaconBlock(parent)
	require.NoError(l.T, err)

	state, err := NewBeaconStateDeneb()
	require.NoError(l.T, err)
	err = state.SetSlot(slot)
	require.NoError(l.T, err)

	parentRoot, err := signedParent.Block().HashTreeRoot()
	require.NoError(l.T, err)

	var signedBlock interfaces.SignedBeaconBlock
	if blinded {
		block := NewBlindedBeaconBlockDeneb()
		block.Message.Slot = slot
		block.Message.ParentRoot = parentRoot[:]

		for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
			block.Message.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)

		h, err := signedBlock.Header()
		require.NoError(l.T, err)

		err = state.SetLatestBlockHeader(h.Header)
		require.NoError(l.T, err)
		stateRoot, err := state.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		// get a new signed block so the root is updated with the new state root
		block.Message.StateRoot = stateRoot[:]
		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)
	} else {
		block := NewBeaconBlockDeneb()
		block.Block.Slot = slot
		block.Block.ParentRoot = parentRoot[:]

		for i := uint64(0); i < params.BeaconConfig().MinSyncCommitteeParticipants; i++ {
			block.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)

		h, err := signedBlock.Header()
		require.NoError(l.T, err)

		err = state.SetLatestBlockHeader(h.Header)
		require.NoError(l.T, err)
		stateRoot, err := state.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		// get a new signed block so the root is updated with the new state root
		block.Block.StateRoot = stateRoot[:]
		signedBlock, err = blocks.NewSignedBeaconBlock(block)
		require.NoError(l.T, err)
	}

	l.State = state
	l.AttestedState = attestedState
	l.AttestedBlock = signedParent
	l.Block = signedBlock
	l.Ctx = ctx
	l.FinalizedBlock = finalizedBlock

	return l
}

func (l *TestLightClient) CheckAttestedHeader(header interfaces.LightClientHeader) {
	updateAttestedHeaderBeacon := header.Beacon()
	testAttestedHeader, err := l.AttestedBlock.Header()
	require.NoError(l.T, err)
	require.Equal(l.T, l.AttestedBlock.Block().Slot(), updateAttestedHeaderBeacon.Slot, "Attested block slot is not equal")
	require.Equal(l.T, testAttestedHeader.Header.ProposerIndex, updateAttestedHeaderBeacon.ProposerIndex, "Attested block proposer index is not equal")
	require.DeepSSZEqual(l.T, testAttestedHeader.Header.ParentRoot, updateAttestedHeaderBeacon.ParentRoot, "Attested block parent root is not equal")
	require.DeepSSZEqual(l.T, testAttestedHeader.Header.BodyRoot, updateAttestedHeaderBeacon.BodyRoot, "Attested block body root is not equal")

	attestedStateRoot, err := l.AttestedState.HashTreeRoot(l.Ctx)
	require.NoError(l.T, err)
	require.DeepSSZEqual(l.T, attestedStateRoot[:], updateAttestedHeaderBeacon.StateRoot, "Attested block state root is not equal")

	if l.AttestedBlock.Version() == version.Capella {
		payloadInterface, err := l.AttestedBlock.Block().Body().Execution()
		require.NoError(l.T, err)
		transactionsRoot, err := payloadInterface.TransactionsRoot()
		if errors.Is(err, consensus_types.ErrUnsupportedField) {
			transactions, err := payloadInterface.Transactions()
			require.NoError(l.T, err)
			transactionsRootArray, err := ssz.TransactionsRoot(transactions)
			require.NoError(l.T, err)
			transactionsRoot = transactionsRootArray[:]
		} else {
			require.NoError(l.T, err)
		}
		withdrawalsRoot, err := payloadInterface.WithdrawalsRoot()
		if errors.Is(err, consensus_types.ErrUnsupportedField) {
			withdrawals, err := payloadInterface.Withdrawals()
			require.NoError(l.T, err)
			withdrawalsRootArray, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
			require.NoError(l.T, err)
			withdrawalsRoot = withdrawalsRootArray[:]
		} else {
			require.NoError(l.T, err)
		}
		execution := &v11.ExecutionPayloadHeaderCapella{
			ParentHash:       payloadInterface.ParentHash(),
			FeeRecipient:     payloadInterface.FeeRecipient(),
			StateRoot:        payloadInterface.StateRoot(),
			ReceiptsRoot:     payloadInterface.ReceiptsRoot(),
			LogsBloom:        payloadInterface.LogsBloom(),
			PrevRandao:       payloadInterface.PrevRandao(),
			BlockNumber:      payloadInterface.BlockNumber(),
			GasLimit:         payloadInterface.GasLimit(),
			GasUsed:          payloadInterface.GasUsed(),
			Timestamp:        payloadInterface.Timestamp(),
			ExtraData:        payloadInterface.ExtraData(),
			BaseFeePerGas:    payloadInterface.BaseFeePerGas(),
			BlockHash:        payloadInterface.BlockHash(),
			TransactionsRoot: transactionsRoot,
			WithdrawalsRoot:  withdrawalsRoot,
		}

		updateAttestedHeaderExecution, err := header.Execution()
		require.NoError(l.T, err)
		require.DeepSSZEqual(l.T, execution, updateAttestedHeaderExecution.Proto(), "Attested Block Execution is not equal")

		executionPayloadProof, err := blocks.PayloadProof(l.Ctx, l.AttestedBlock.Block())
		require.NoError(l.T, err)
		updateAttestedHeaderExecutionBranch, err := header.ExecutionBranch()
		require.NoError(l.T, err)
		for i, leaf := range updateAttestedHeaderExecutionBranch {
			require.DeepSSZEqual(l.T, executionPayloadProof[i], leaf[:], "Leaf is not equal")
		}
	}

	if l.AttestedBlock.Version() == version.Deneb {
		payloadInterface, err := l.AttestedBlock.Block().Body().Execution()
		require.NoError(l.T, err)
		transactionsRoot, err := payloadInterface.TransactionsRoot()
		if errors.Is(err, consensus_types.ErrUnsupportedField) {
			transactions, err := payloadInterface.Transactions()
			require.NoError(l.T, err)
			transactionsRootArray, err := ssz.TransactionsRoot(transactions)
			require.NoError(l.T, err)
			transactionsRoot = transactionsRootArray[:]
		} else {
			require.NoError(l.T, err)
		}
		withdrawalsRoot, err := payloadInterface.WithdrawalsRoot()
		if errors.Is(err, consensus_types.ErrUnsupportedField) {
			withdrawals, err := payloadInterface.Withdrawals()
			require.NoError(l.T, err)
			withdrawalsRootArray, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
			require.NoError(l.T, err)
			withdrawalsRoot = withdrawalsRootArray[:]
		} else {
			require.NoError(l.T, err)
		}
		execution := &v11.ExecutionPayloadHeaderDeneb{
			ParentHash:       payloadInterface.ParentHash(),
			FeeRecipient:     payloadInterface.FeeRecipient(),
			StateRoot:        payloadInterface.StateRoot(),
			ReceiptsRoot:     payloadInterface.ReceiptsRoot(),
			LogsBloom:        payloadInterface.LogsBloom(),
			PrevRandao:       payloadInterface.PrevRandao(),
			BlockNumber:      payloadInterface.BlockNumber(),
			GasLimit:         payloadInterface.GasLimit(),
			GasUsed:          payloadInterface.GasUsed(),
			Timestamp:        payloadInterface.Timestamp(),
			ExtraData:        payloadInterface.ExtraData(),
			BaseFeePerGas:    payloadInterface.BaseFeePerGas(),
			BlockHash:        payloadInterface.BlockHash(),
			TransactionsRoot: transactionsRoot,
			WithdrawalsRoot:  withdrawalsRoot,
		}

		updateAttestedHeaderExecution, err := header.Execution()
		require.NoError(l.T, err)
		require.DeepSSZEqual(l.T, execution, updateAttestedHeaderExecution.Proto(), "Attested Block Execution is not equal")

		executionPayloadProof, err := blocks.PayloadProof(l.Ctx, l.AttestedBlock.Block())
		require.NoError(l.T, err)
		updateAttestedHeaderExecutionBranch, err := header.ExecutionBranch()
		require.NoError(l.T, err)
		for i, leaf := range updateAttestedHeaderExecutionBranch {
			require.DeepSSZEqual(l.T, executionPayloadProof[i], leaf[:], "Leaf is not equal")
		}
	}
}

func (l *TestLightClient) CheckSyncAggregate(sa *pb.SyncAggregate) {
	syncAggregate, err := l.Block.Block().Body().SyncAggregate()
	require.NoError(l.T, err)
	require.DeepSSZEqual(l.T, syncAggregate.SyncCommitteeBits, sa.SyncCommitteeBits, "SyncAggregate bits is not equal")
	require.DeepSSZEqual(l.T, syncAggregate.SyncCommitteeSignature, sa.SyncCommitteeSignature, "SyncAggregate signature is not equal")
}
