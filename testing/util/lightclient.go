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
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

type TestLightClient struct {
	T              *testing.T
	Ctx            context.Context
	State          state.BeaconState
	Block          interfaces.ReadOnlySignedBeaconBlock
	AttestedState  state.BeaconState
	AttestedBlock  interfaces.ReadOnlySignedBeaconBlock
	FinalizedBlock interfaces.ReadOnlySignedBeaconBlock

	version                 int
	supermajority           bool
	blinded                 bool
	increaseAttestedSlotBy  int
	increaseFinalizedSlotBy int
}
type LightClientOption func(l *TestLightClient)

func NewTestLightClient(t *testing.T, forkVersion int, options ...LightClientOption) *TestLightClient {
	l := &TestLightClient{T: t, version: forkVersion}

	for _, option := range options {
		option(l)
	}

	switch l.version {
	case version.Altair:
		return l.setupTestAltair()
	case version.Bellatrix:
		return l.setupTestBellatrix()
	case version.Capella:
		return l.setupTestCapella()
	case version.Deneb:
		return l.setupTestDeneb()
	case version.Electra:
		return l.setupTestElectra()
	default:
		l.T.Fatalf("unknown version %d", l.version)
		return nil
	}
}

// WithBlinded Specifies whether the signature block is blinded or not
func WithBlinded() func(l *TestLightClient) {
	return func(l *TestLightClient) {
		l.blinded = true
	}
}

// WithSupermajority Specifies whether the sync committee bits have supermajority or not
func WithSupermajority(supermajority bool) LightClientOption {
	return func(l *TestLightClient) {
		l.supermajority = supermajority
	}
}

// WithIncreasedAttestedSlot Specifies the number of slots to increase the attested slot by. This does not affect the finalized block's slot if there is any.
func WithIncreasedAttestedSlot(increaseAttestedSlotBy int) LightClientOption {
	return func(l *TestLightClient) {
		l.increaseAttestedSlotBy = increaseAttestedSlotBy
	}
}

// WithIncreasedFinalizedSlot Specifies the number of slots to increase the finalized slot by. This DOES NOT affect the attested block's slot. That should be handled separately using WithIncreasedAttestedSlot.
func WithIncreasedFinalizedSlot(increaseFinalizedSlotBy int) LightClientOption {
	return func(l *TestLightClient) {
		l.increaseFinalizedSlotBy = increaseFinalizedSlotBy
	}
}

func NewTestLightClient2(t *testing.T) *TestLightClient {
	return &TestLightClient{T: t}
}

func (l *TestLightClient) setupTestAltair() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().AltairForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(uint64(l.increaseAttestedSlotBy))
	}

	finalizedSlot := primitives.Slot(uint64(params.BeaconConfig().AltairForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
	if l.increaseFinalizedSlotBy > 0 {
		finalizedSlot = finalizedSlot.Add(uint64(l.increaseFinalizedSlotBy))
	}

	signatureSlot := attestedSlot.Add(1)

	// Finalized State & Block
	finalizedState, err := NewBeaconStateAltair()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

	finalizedBlock := NewBeaconBlockAltair()
	require.NoError(l.T, err)
	finalizedBlock.Block.Slot = finalizedSlot
	signedFinalizedBlock, err := blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)
	finalizedHeader, err := signedFinalizedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
	finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
	signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)

	// Attested State & Block
	attestedState, err := NewBeaconStateAltair()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	// Set the finalized checkpoint
	finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
	require.NoError(l.T, err)
	finalizedCheckpoint := &ethpb.Checkpoint{
		Epoch: slots.ToEpoch(finalizedSlot),
		Root:  finalizedBlockRoot[:],
	}
	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))

	attestedBlock := NewBeaconBlockAltair()
	attestedBlock.Block.Slot = attestedSlot
	signedAttestedBlock, err := blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)
	attestedBlockHeader, err := signedAttestedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetLatestBlockHeader(attestedBlockHeader.Header))
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	attestedBlock.Block.StateRoot = attestedStateRoot[:]
	signedAttestedBlock, err = blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)

	// Signature State & Block
	signatureState, err := NewBeaconStateAltair()
	require.NoError(l.T, err)
	require.NoError(l.T, signatureState.SetSlot(signatureSlot))

	signatureBlock := NewBeaconBlockAltair()
	signatureBlock.Block.Slot = signatureSlot
	attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
	require.NoError(l.T, err)
	signatureBlock.Block.ParentRoot = attestedBlockRoot[:]

	var trueBitNum uint64
	if l.supermajority {
		trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1)
	} else {
		trueBitNum = params.BeaconConfig().MinSyncCommitteeParticipants
	}
	for i := uint64(0); i < trueBitNum; i++ {
		signatureBlock.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedSignatureBlock, err := blocks.NewSignedBeaconBlock(signatureBlock)
	require.NoError(l.T, err)
	signatureBlockHeader, err := signedSignatureBlock.Header()
	require.NoError(l.T, err)
	err = signatureState.SetLatestBlockHeader(signatureBlockHeader.Header)
	require.NoError(l.T, err)
	signatureStateRoot, err := signatureState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	signatureBlock.Block.StateRoot = signatureStateRoot[:]
	signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
	require.NoError(l.T, err)

	l.State = signatureState
	l.AttestedState = attestedState
	l.Block = signedSignatureBlock
	l.Ctx = ctx
	l.FinalizedBlock = signedFinalizedBlock
	l.AttestedBlock = signedAttestedBlock

	return l
}

func (l *TestLightClient) setupTestBellatrix() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().BellatrixForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(uint64(l.increaseAttestedSlotBy))
	}

	finalizedSlot := primitives.Slot(uint64(params.BeaconConfig().BellatrixForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
	if l.increaseFinalizedSlotBy > 0 {
		finalizedSlot = finalizedSlot.Add(uint64(l.increaseFinalizedSlotBy))
	}

	signatureSlot := attestedSlot.Add(1)

	// Finalized State & Block
	finalizedState, err := NewBeaconStateBellatrix()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

	finalizedBlock := NewBeaconBlockBellatrix()
	require.NoError(l.T, err)
	finalizedBlock.Block.Slot = finalizedSlot
	signedFinalizedBlock, err := blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)
	finalizedHeader, err := signedFinalizedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
	finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
	signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)

	// Attested State & Block
	attestedState, err := NewBeaconStateBellatrix()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	// Set the finalized checkpoint
	finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
	require.NoError(l.T, err)
	finalizedCheckpoint := &ethpb.Checkpoint{
		Epoch: slots.ToEpoch(finalizedSlot),
		Root:  finalizedBlockRoot[:],
	}
	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))

	attestedBlock := NewBeaconBlockBellatrix()
	attestedBlock.Block.Slot = attestedSlot
	signedAttestedBlock, err := blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)
	attestedBlockHeader, err := signedAttestedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetLatestBlockHeader(attestedBlockHeader.Header))
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	attestedBlock.Block.StateRoot = attestedStateRoot[:]
	signedAttestedBlock, err = blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)

	// Signature State & Block
	signatureState, err := NewBeaconStateBellatrix()
	require.NoError(l.T, err)
	require.NoError(l.T, signatureState.SetSlot(signatureSlot))

	signatureBlock := NewBeaconBlockBellatrix()
	signatureBlock.Block.Slot = signatureSlot
	attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
	require.NoError(l.T, err)
	signatureBlock.Block.ParentRoot = attestedBlockRoot[:]

	var trueBitNum uint64
	if l.supermajority {
		trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1)
	} else {
		trueBitNum = params.BeaconConfig().MinSyncCommitteeParticipants
	}
	for i := uint64(0); i < trueBitNum; i++ {
		signatureBlock.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
	}

	signedSignatureBlock, err := blocks.NewSignedBeaconBlock(signatureBlock)
	require.NoError(l.T, err)
	signatureBlockHeader, err := signedSignatureBlock.Header()
	require.NoError(l.T, err)
	err = signatureState.SetLatestBlockHeader(signatureBlockHeader.Header)
	require.NoError(l.T, err)
	signatureStateRoot, err := signatureState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	signatureBlock.Block.StateRoot = signatureStateRoot[:]
	signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
	require.NoError(l.T, err)

	l.State = signatureState
	l.AttestedState = attestedState
	l.Block = signedSignatureBlock
	l.Ctx = ctx
	l.FinalizedBlock = signedFinalizedBlock
	l.AttestedBlock = signedAttestedBlock

	return l
}

func (l *TestLightClient) setupTestCapella() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().CapellaForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(uint64(l.increaseAttestedSlotBy))
	}

	finalizedSlot := primitives.Slot(uint64(params.BeaconConfig().CapellaForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
	if l.increaseFinalizedSlotBy > 0 {
		finalizedSlot = finalizedSlot.Add(uint64(l.increaseFinalizedSlotBy))
	}

	signatureSlot := attestedSlot.Add(1)

	// Finalized State & Block
	finalizedState, err := NewBeaconStateCapella()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

	finalizedBlock := NewBeaconBlockCapella()
	require.NoError(l.T, err)
	finalizedBlock.Block.Slot = finalizedSlot
	signedFinalizedBlock, err := blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)
	finalizedHeader, err := signedFinalizedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
	finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
	signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)

	// Attested State & Block
	attestedState, err := NewBeaconStateCapella()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	// Set the finalized checkpoint
	finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
	require.NoError(l.T, err)
	finalizedCheckpoint := &ethpb.Checkpoint{
		Epoch: slots.ToEpoch(finalizedSlot),
		Root:  finalizedBlockRoot[:],
	}
	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))

	attestedBlock := NewBeaconBlockCapella()
	attestedBlock.Block.Slot = attestedSlot
	signedAttestedBlock, err := blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)
	attestedBlockHeader, err := signedAttestedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetLatestBlockHeader(attestedBlockHeader.Header))
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	attestedBlock.Block.StateRoot = attestedStateRoot[:]
	signedAttestedBlock, err = blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)

	// Signature State & Block
	signatureState, err := NewBeaconStateCapella()
	require.NoError(l.T, err)
	require.NoError(l.T, signatureState.SetSlot(signatureSlot))

	var signedSignatureBlock interfaces.SignedBeaconBlock
	if l.blinded {
		signatureBlock := NewBlindedBeaconBlockCapella()
		signatureBlock.Block.Slot = signatureSlot
		attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		signatureBlock.Block.ParentRoot = attestedBlockRoot[:]

		var trueBitNum uint64
		if l.supermajority {
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1)
		} else {
			trueBitNum = params.BeaconConfig().MinSyncCommitteeParticipants
		}
		for i := uint64(0); i < trueBitNum; i++ {
			signatureBlock.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)

		signatureBlockHeader, err := signedSignatureBlock.Header()
		require.NoError(l.T, err)

		err = signatureState.SetLatestBlockHeader(signatureBlockHeader.Header)
		require.NoError(l.T, err)
		stateRoot, err := signatureState.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		signatureBlock.Block.StateRoot = stateRoot[:]
		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)
	} else {
		signatureBlock := NewBeaconBlockCapella()
		signatureBlock.Block.Slot = signatureSlot
		attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		signatureBlock.Block.ParentRoot = attestedBlockRoot[:]

		var trueBitNum uint64
		if l.supermajority {
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1)
		} else {
			trueBitNum = params.BeaconConfig().MinSyncCommitteeParticipants
		}
		for i := uint64(0); i < trueBitNum; i++ {
			signatureBlock.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)

		signatureBlockHeader, err := signedSignatureBlock.Header()
		require.NoError(l.T, err)

		err = signatureState.SetLatestBlockHeader(signatureBlockHeader.Header)
		require.NoError(l.T, err)
		signatureStateRoot, err := signatureState.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		signatureBlock.Block.StateRoot = signatureStateRoot[:]
		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)
	}

	l.State = signatureState
	l.AttestedState = attestedState
	l.AttestedBlock = signedAttestedBlock
	l.Block = signedSignatureBlock
	l.Ctx = ctx
	l.FinalizedBlock = signedFinalizedBlock

	return l
}

func (l *TestLightClient) setupTestDeneb() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().DenebForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(uint64(l.increaseAttestedSlotBy))
	}

	finalizedSlot := primitives.Slot(uint64(params.BeaconConfig().DenebForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
	if l.increaseFinalizedSlotBy > 0 {
		finalizedSlot = finalizedSlot.Add(uint64(l.increaseFinalizedSlotBy))
	}

	signatureSlot := attestedSlot.Add(1)

	// Finalized State & Block
	finalizedState, err := NewBeaconStateDeneb()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

	finalizedBlock := NewBeaconBlockDeneb()
	require.NoError(l.T, err)
	finalizedBlock.Block.Slot = finalizedSlot
	signedFinalizedBlock, err := blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)
	finalizedHeader, err := signedFinalizedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
	finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
	signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)

	// Attested State & Block
	attestedState, err := NewBeaconStateDeneb()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	// Set the finalized checkpoint
	finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
	require.NoError(l.T, err)
	finalizedCheckpoint := &ethpb.Checkpoint{
		Epoch: slots.ToEpoch(finalizedSlot),
		Root:  finalizedBlockRoot[:],
	}
	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))

	attestedBlock := NewBeaconBlockDeneb()
	attestedBlock.Block.Slot = attestedSlot
	signedAttestedBlock, err := blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)
	attestedBlockHeader, err := signedAttestedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetLatestBlockHeader(attestedBlockHeader.Header))
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	attestedBlock.Block.StateRoot = attestedStateRoot[:]
	signedAttestedBlock, err = blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)

	// Signature State & Block
	signatureState, err := NewBeaconStateDeneb()
	require.NoError(l.T, err)
	require.NoError(l.T, signatureState.SetSlot(signatureSlot))

	var signedSignatureBlock interfaces.SignedBeaconBlock
	if l.blinded {
		signatureBlock := NewBlindedBeaconBlockDeneb()
		signatureBlock.Message.Slot = signatureSlot
		attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		signatureBlock.Message.ParentRoot = attestedBlockRoot[:]

		var trueBitNum uint64
		if l.supermajority {
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1)
		} else {
			trueBitNum = params.BeaconConfig().MinSyncCommitteeParticipants
		}
		for i := uint64(0); i < trueBitNum; i++ {
			signatureBlock.Message.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)

		signatureBlockHeader, err := signedSignatureBlock.Header()
		require.NoError(l.T, err)

		err = signatureState.SetLatestBlockHeader(signatureBlockHeader.Header)
		require.NoError(l.T, err)
		stateRoot, err := signatureState.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		signatureBlock.Message.StateRoot = stateRoot[:]
		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)
	} else {
		signatureBlock := NewBeaconBlockDeneb()
		signatureBlock.Block.Slot = signatureSlot
		attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		signatureBlock.Block.ParentRoot = attestedBlockRoot[:]

		var trueBitNum uint64
		if l.supermajority {
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1)
		} else {
			trueBitNum = params.BeaconConfig().MinSyncCommitteeParticipants
		}
		for i := uint64(0); i < trueBitNum; i++ {
			signatureBlock.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)

		signatureBlockHeader, err := signedSignatureBlock.Header()
		require.NoError(l.T, err)

		err = signatureState.SetLatestBlockHeader(signatureBlockHeader.Header)
		require.NoError(l.T, err)
		signatureStateRoot, err := signatureState.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		signatureBlock.Block.StateRoot = signatureStateRoot[:]
		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)
	}

	l.State = signatureState
	l.AttestedState = attestedState
	l.AttestedBlock = signedAttestedBlock
	l.Block = signedSignatureBlock
	l.Ctx = ctx
	l.FinalizedBlock = signedFinalizedBlock

	return l
}

func (l *TestLightClient) setupTestElectra() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().ElectraForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(uint64(l.increaseAttestedSlotBy))
	}

	finalizedSlot := primitives.Slot(uint64(params.BeaconConfig().ElectraForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
	if l.increaseFinalizedSlotBy > 0 {
		finalizedSlot = finalizedSlot.Add(uint64(l.increaseFinalizedSlotBy))
	}

	signatureSlot := attestedSlot.Add(1)

	// Finalized State & Block
	finalizedState, err := NewBeaconStateElectra()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

	finalizedBlock := NewBeaconBlockElectra()
	require.NoError(l.T, err)
	finalizedBlock.Block.Slot = finalizedSlot
	signedFinalizedBlock, err := blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)
	finalizedHeader, err := signedFinalizedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
	finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
	signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)

	// Attested State & Block
	attestedState, err := NewBeaconStateElectra()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	// Set the finalized checkpoint
	finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
	require.NoError(l.T, err)
	finalizedCheckpoint := &ethpb.Checkpoint{
		Epoch: slots.ToEpoch(finalizedSlot),
		Root:  finalizedBlockRoot[:],
	}
	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))

	attestedBlock := NewBeaconBlockElectra()
	attestedBlock.Block.Slot = attestedSlot
	signedAttestedBlock, err := blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)
	attestedBlockHeader, err := signedAttestedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetLatestBlockHeader(attestedBlockHeader.Header))
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	attestedBlock.Block.StateRoot = attestedStateRoot[:]
	signedAttestedBlock, err = blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)

	// Signature State & Block
	signatureState, err := NewBeaconStateElectra()
	require.NoError(l.T, err)
	require.NoError(l.T, signatureState.SetSlot(signatureSlot))

	var signedSignatureBlock interfaces.SignedBeaconBlock
	if l.blinded {
		signatureBlock := NewBlindedBeaconBlockElectra()
		signatureBlock.Message.Slot = signatureSlot
		attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		signatureBlock.Message.ParentRoot = attestedBlockRoot[:]

		var trueBitNum uint64
		if l.supermajority {
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1)
		} else {
			trueBitNum = params.BeaconConfig().MinSyncCommitteeParticipants
		}
		for i := uint64(0); i < trueBitNum; i++ {
			signatureBlock.Message.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)

		signatureBlockHeader, err := signedSignatureBlock.Header()
		require.NoError(l.T, err)

		err = signatureState.SetLatestBlockHeader(signatureBlockHeader.Header)
		require.NoError(l.T, err)
		stateRoot, err := signatureState.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		signatureBlock.Message.StateRoot = stateRoot[:]
		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)
	} else {
		signatureBlock := NewBeaconBlockElectra()
		signatureBlock.Block.Slot = signatureSlot
		attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		signatureBlock.Block.ParentRoot = attestedBlockRoot[:]

		var trueBitNum uint64
		if l.supermajority {
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1)
		} else {
			trueBitNum = params.BeaconConfig().MinSyncCommitteeParticipants
		}
		for i := uint64(0); i < trueBitNum; i++ {
			signatureBlock.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)

		signatureBlockHeader, err := signedSignatureBlock.Header()
		require.NoError(l.T, err)

		err = signatureState.SetLatestBlockHeader(signatureBlockHeader.Header)
		require.NoError(l.T, err)
		signatureStateRoot, err := signatureState.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		signatureBlock.Block.StateRoot = signatureStateRoot[:]
		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)
	}

	l.State = signatureState
	l.AttestedState = attestedState
	l.AttestedBlock = signedAttestedBlock
	l.Block = signedSignatureBlock
	l.Ctx = ctx
	l.FinalizedBlock = signedFinalizedBlock

	return l
}

func (l *TestLightClient) setupTestFulu() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().FuluForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(uint64(l.increaseAttestedSlotBy))
	}

	finalizedSlot := primitives.Slot(uint64(params.BeaconConfig().FuluForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
	if l.increaseFinalizedSlotBy > 0 {
		finalizedSlot = finalizedSlot.Add(uint64(l.increaseFinalizedSlotBy))
	}

	signatureSlot := attestedSlot.Add(1)

	// Finalized State & Block
	finalizedState, err := NewBeaconStateFulu()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

	finalizedBlock := NewBeaconBlockFulu()
	require.NoError(l.T, err)
	finalizedBlock.Block.Slot = finalizedSlot
	signedFinalizedBlock, err := blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)
	finalizedHeader, err := signedFinalizedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
	finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
	signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(l.T, err)

	// Attested State & Block
	attestedState, err := NewBeaconStateFulu()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	// Set the finalized checkpoint
	finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
	require.NoError(l.T, err)
	finalizedCheckpoint := &ethpb.Checkpoint{
		Epoch: slots.ToEpoch(finalizedSlot),
		Root:  finalizedBlockRoot[:],
	}
	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))

	attestedBlock := NewBeaconBlockFulu()
	attestedBlock.Block.Slot = attestedSlot
	signedAttestedBlock, err := blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)
	attestedBlockHeader, err := signedAttestedBlock.Header()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetLatestBlockHeader(attestedBlockHeader.Header))
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	require.NoError(l.T, err)
	attestedBlock.Block.StateRoot = attestedStateRoot[:]
	signedAttestedBlock, err = blocks.NewSignedBeaconBlock(attestedBlock)
	require.NoError(l.T, err)

	// Signature State & Block
	signatureState, err := NewBeaconStateFulu()
	require.NoError(l.T, err)
	require.NoError(l.T, signatureState.SetSlot(signatureSlot))

	var signedSignatureBlock interfaces.SignedBeaconBlock
	if l.blinded {
		signatureBlock := NewBlindedBeaconBlockFulu()
		signatureBlock.Message.Slot = signatureSlot
		attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		signatureBlock.Message.ParentRoot = attestedBlockRoot[:]

		var trueBitNum uint64
		if l.supermajority {
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1)
		} else {
			trueBitNum = params.BeaconConfig().MinSyncCommitteeParticipants
		}
		for i := uint64(0); i < trueBitNum; i++ {
			signatureBlock.Message.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)

		signatureBlockHeader, err := signedSignatureBlock.Header()
		require.NoError(l.T, err)

		err = signatureState.SetLatestBlockHeader(signatureBlockHeader.Header)
		require.NoError(l.T, err)
		stateRoot, err := signatureState.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		signatureBlock.Message.StateRoot = stateRoot[:]
		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)
	} else {
		signatureBlock := NewBeaconBlockFulu()
		signatureBlock.Block.Slot = signatureSlot
		attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		signatureBlock.Block.ParentRoot = attestedBlockRoot[:]

		var trueBitNum uint64
		if l.supermajority {
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1)
		} else {
			trueBitNum = params.BeaconConfig().MinSyncCommitteeParticipants
		}
		for i := uint64(0); i < trueBitNum; i++ {
			signatureBlock.Block.Body.SyncAggregate.SyncCommitteeBits.SetBitAt(i, true)
		}

		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)

		signatureBlockHeader, err := signedSignatureBlock.Header()
		require.NoError(l.T, err)

		err = signatureState.SetLatestBlockHeader(signatureBlockHeader.Header)
		require.NoError(l.T, err)
		signatureStateRoot, err := signatureState.HashTreeRoot(ctx)
		require.NoError(l.T, err)

		signatureBlock.Block.StateRoot = signatureStateRoot[:]
		signedSignatureBlock, err = blocks.NewSignedBeaconBlock(signatureBlock)
		require.NoError(l.T, err)
	}

	l.State = signatureState
	l.AttestedState = attestedState
	l.AttestedBlock = signedAttestedBlock
	l.Block = signedSignatureBlock
	l.Ctx = ctx
	l.FinalizedBlock = signedFinalizedBlock

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

func (l *TestLightClient) SetupTestElectraFinalizedBlockDeneb(blinded bool) *TestLightClient {
	ctx := context.Background()

	slot := primitives.Slot(params.BeaconConfig().ElectraForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	finalizedBlockSlot := primitives.Slot(params.BeaconConfig().DenebForkEpoch * primitives.Epoch(params.BeaconConfig().SlotsPerEpoch))

	attestedState, err := NewBeaconStateElectra()
	require.NoError(l.T, err)
	err = attestedState.SetSlot(slot)
	require.NoError(l.T, err)

	finalizedBlock, err := blocks.NewSignedBeaconBlock(NewBeaconBlockDeneb())
	require.NoError(l.T, err)
	finalizedBlock.SetSlot(finalizedBlockSlot)
	finalizedHeader, err := finalizedBlock.Header()
	require.NoError(l.T, err)
	finalizedRoot, err := finalizedHeader.Header.HashTreeRoot()
	require.NoError(l.T, err)

	require.NoError(l.T, attestedState.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: params.BeaconConfig().DenebForkEpoch,
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

func (l *TestLightClient) CheckSyncAggregate(sa *ethpb.SyncAggregate) {
	syncAggregate, err := l.Block.Block().Body().SyncAggregate()
	require.NoError(l.T, err)
	require.DeepSSZEqual(l.T, syncAggregate.SyncCommitteeBits, sa.SyncCommitteeBits, "SyncAggregate bits is not equal")
	require.DeepSSZEqual(l.T, syncAggregate.SyncCommitteeSignature, sa.SyncCommitteeSignature, "SyncAggregate signature is not equal")
}
