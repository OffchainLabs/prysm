package util

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	fieldparams "github.com/OffchainLabs/prysm/v6/config/fieldparams"
	"github.com/OffchainLabs/prysm/v6/config/params"
	consensus_types "github.com/OffchainLabs/prysm/v6/consensus-types"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	lightclienttypes "github.com/OffchainLabs/prysm/v6/consensus-types/light-client"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/ssz"
	v11 "github.com/OffchainLabs/prysm/v6/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/pkg/errors"
)

type TestLightClient struct {
	finalizedCheckpointInPrevFork bool
	blinded                       bool
	supermajority                 bool
	noFinalizedCheckpoint         bool
	version                       int
	increaseAttestedSlotBy        uint64
	increaseFinalizedSlotBy       uint64
	increaseSignatureSlotBy       uint64
	increaseActiveParticipantsBy  uint64
	attestedParentRoot            [32]byte

	T              *testing.T
	Ctx            context.Context
	State          state.BeaconState
	Block          interfaces.ReadOnlySignedBeaconBlock
	AttestedState  state.BeaconState
	AttestedBlock  interfaces.ReadOnlySignedBeaconBlock
	FinalizedBlock interfaces.ReadOnlySignedBeaconBlock
	FinalizedState state.BeaconState
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
		l.T.Fatalf("Unsupported version %s", version.String(l.version))
		return nil
	}
}

// WithAttestedParentRoot sets the parent root of the attested block.
func WithAttestedParentRoot(parentRoot [32]byte) LightClientOption {
	return func(l *TestLightClient) {
		l.attestedParentRoot = parentRoot
	}
}

// WithBlinded specifies whether the signature block is blinded or not
func WithBlinded() LightClientOption {
	return func(l *TestLightClient) {
		if l.version == version.Altair {
			l.T.Fatalf("Blinded blocks are not supported in Altair")
		}
		l.blinded = true
	}
}

// WithNoFinalizedCheckpoint avoids setting a finalized checkpoint for the attested state
func WithNoFinalizedCheckpoint() LightClientOption {
	return func(l *TestLightClient) {
		l.noFinalizedCheckpoint = true
	}
}

// WithFinalizedCheckpointInPrevFork creates a finalized checkpoint for the attested state, in the previous fork.
func WithFinalizedCheckpointInPrevFork() LightClientOption {
	return func(l *TestLightClient) {
		if l.version == version.Altair {
			l.T.Fatalf("Can't set finalized checkpoint in previous fork for Altair")
		}
		l.finalizedCheckpointInPrevFork = true
	}
}

// WithSupermajority specifies whether the sync committee bits have supermajority or not
func WithSupermajority(increaseActiveParticipantsBy uint64) LightClientOption {
	return func(l *TestLightClient) {
		l.supermajority = true
		l.increaseActiveParticipantsBy = increaseActiveParticipantsBy
	}
}

// WithIncreasedAttestedSlot specifies the number of slots to increase the attested slot by. This does not affect the finalized block's slot if there is any.
func WithIncreasedAttestedSlot(increaseBy uint64) LightClientOption {
	return func(l *TestLightClient) {
		l.increaseAttestedSlotBy = increaseBy
	}
}

// WithIncreasedFinalizedSlot specifies the number of slots to increase the finalized slot by. This DOES NOT affect the attested block's slot. That should be handled separately using WithIncreasedAttestedSlot.
func WithIncreasedFinalizedSlot(increaseBy uint64) LightClientOption {
	return func(l *TestLightClient) {
		l.increaseFinalizedSlotBy = increaseBy
	}
}

// WithIncreasedSignatureSlot specifies the number of slots to increase the signature slot by. This does not affect the attested/finalized block's slot.
func WithIncreasedSignatureSlot(increaseBy uint64) LightClientOption {
	return func(l *TestLightClient) {
		l.increaseSignatureSlotBy = increaseBy
	}
}

func (l *TestLightClient) setupTestAltair() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().AltairForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(l.increaseAttestedSlotBy)
	}

	signatureSlot := attestedSlot.Add(1)
	if l.increaseSignatureSlotBy > 0 {
		signatureSlot = signatureSlot.Add(l.increaseSignatureSlotBy)
	}

	// Attested State
	attestedState, err := NewBeaconStateAltair()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	var signedFinalizedBlock interfaces.SignedBeaconBlock
	var finalizedState state.BeaconState
	// Finalized checkpoint
	if !l.noFinalizedCheckpoint {
		finalizedSlot := primitives.Slot(uint64(params.BeaconConfig().AltairForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
		if l.increaseFinalizedSlotBy > 0 {
			finalizedSlot = finalizedSlot.Add(l.increaseFinalizedSlotBy)
		}
		// Finalized State & Block
		finalizedState, err = NewBeaconStateAltair()
		require.NoError(l.T, err)
		require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

		finalizedBlock := NewBeaconBlockAltair()
		require.NoError(l.T, err)
		finalizedBlock.Block.Slot = finalizedSlot
		signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
		require.NoError(l.T, err)
		finalizedHeader, err := signedFinalizedBlock.Header()
		require.NoError(l.T, err)
		require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
		finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
		require.NoError(l.T, err)
		finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
		signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
		require.NoError(l.T, err)

		// Set the finalized checkpoint
		finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		finalizedCheckpoint := &ethpb.Checkpoint{
			Epoch: slots.ToEpoch(finalizedSlot),
			Root:  finalizedBlockRoot[:],
		}

		require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))
	}

	// Attested Block
	attestedBlock := NewBeaconBlockAltair()
	attestedBlock.Block.Slot = attestedSlot
	attestedBlock.Block.ParentRoot = l.attestedParentRoot[:]
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
		trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1 + float64(l.increaseActiveParticipantsBy))
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
	l.FinalizedState = finalizedState

	return l
}

func (l *TestLightClient) setupTestBellatrix() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().BellatrixForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(l.increaseAttestedSlotBy)
	}

	signatureSlot := attestedSlot.Add(1)
	if l.increaseSignatureSlotBy > 0 {
		signatureSlot = signatureSlot.Add(l.increaseSignatureSlotBy)
	}

	// Attested State & Block
	attestedState, err := NewBeaconStateBellatrix()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	var signedFinalizedBlock interfaces.SignedBeaconBlock
	var finalizedState state.BeaconState
	// Finalized checkpoint
	if !l.noFinalizedCheckpoint {
		var finalizedSlot primitives.Slot
		if l.finalizedCheckpointInPrevFork {
			finalizedSlot = primitives.Slot(uint64(params.BeaconConfig().AltairForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
			if l.increaseFinalizedSlotBy > 0 {
				finalizedSlot = finalizedSlot.Add(l.increaseFinalizedSlotBy)
			}

			finalizedState, err = NewBeaconStateAltair()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

			finalizedBlock := NewBeaconBlockAltair()
			require.NoError(l.T, err)
			finalizedBlock.Block.Slot = finalizedSlot
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
			finalizedHeader, err := signedFinalizedBlock.Header()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
			finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
			require.NoError(l.T, err)
			finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
		} else {
			finalizedSlot = primitives.Slot(uint64(params.BeaconConfig().BellatrixForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
			if l.increaseFinalizedSlotBy > 0 {
				finalizedSlot = finalizedSlot.Add(l.increaseFinalizedSlotBy)
			}

			finalizedState, err = NewBeaconStateBellatrix()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

			finalizedBlock := NewBeaconBlockBellatrix()
			require.NoError(l.T, err)
			finalizedBlock.Block.Slot = finalizedSlot
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
			finalizedHeader, err := signedFinalizedBlock.Header()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
			finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
			require.NoError(l.T, err)
			finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
		}

		// Set the finalized checkpoint
		finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		finalizedCheckpoint := &ethpb.Checkpoint{
			Epoch: slots.ToEpoch(finalizedSlot),
			Root:  finalizedBlockRoot[:],
		}
		require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))
	}

	attestedBlock := NewBeaconBlockBellatrix()
	attestedBlock.Block.Slot = attestedSlot
	attestedBlock.Block.ParentRoot = l.attestedParentRoot[:]
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

	var signedSignatureBlock interfaces.SignedBeaconBlock
	if l.blinded {
		signatureBlock := NewBlindedBeaconBlockBellatrix()
		signatureBlock.Block.Slot = signatureSlot
		attestedBlockRoot, err := signedAttestedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		signatureBlock.Block.ParentRoot = attestedBlockRoot[:]

		var trueBitNum uint64
		if l.supermajority {
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1 + float64(l.increaseActiveParticipantsBy))
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
	l.Block = signedSignatureBlock
	l.Ctx = ctx
	l.FinalizedBlock = signedFinalizedBlock
	l.AttestedBlock = signedAttestedBlock
	l.FinalizedState = finalizedState

	return l
}

func (l *TestLightClient) setupTestCapella() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().CapellaForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(l.increaseAttestedSlotBy)
	}

	signatureSlot := attestedSlot.Add(1)
	if l.increaseSignatureSlotBy > 0 {
		signatureSlot = signatureSlot.Add(l.increaseSignatureSlotBy)
	}

	// Attested State
	attestedState, err := NewBeaconStateCapella()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	var signedFinalizedBlock interfaces.SignedBeaconBlock
	var finalizedState state.BeaconState
	// Finalized checkpoint
	if !l.noFinalizedCheckpoint {
		var finalizedSlot primitives.Slot
		if l.finalizedCheckpointInPrevFork {
			finalizedSlot = primitives.Slot(uint64(params.BeaconConfig().BellatrixForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
			if l.increaseFinalizedSlotBy > 0 {
				finalizedSlot = finalizedSlot.Add(l.increaseFinalizedSlotBy)
			}

			finalizedState, err = NewBeaconStateBellatrix()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

			finalizedBlock := NewBeaconBlockBellatrix()
			require.NoError(l.T, err)
			finalizedBlock.Block.Slot = finalizedSlot
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
			finalizedHeader, err := signedFinalizedBlock.Header()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
			finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
			require.NoError(l.T, err)
			finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
		} else {
			finalizedSlot = primitives.Slot(uint64(params.BeaconConfig().CapellaForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
			if l.increaseFinalizedSlotBy > 0 {
				finalizedSlot = finalizedSlot.Add(l.increaseFinalizedSlotBy)
			}

			finalizedState, err = NewBeaconStateCapella()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

			finalizedBlock := NewBeaconBlockCapella()
			require.NoError(l.T, err)
			finalizedBlock.Block.Slot = finalizedSlot
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
			finalizedHeader, err := signedFinalizedBlock.Header()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
			finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
			require.NoError(l.T, err)
			finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
		}

		// Set the finalized checkpoint
		finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		finalizedCheckpoint := &ethpb.Checkpoint{
			Epoch: slots.ToEpoch(finalizedSlot),
			Root:  finalizedBlockRoot[:],
		}
		require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))
	}

	// Attested Block
	attestedBlock := NewBeaconBlockCapella()
	attestedBlock.Block.Slot = attestedSlot
	attestedBlock.Block.ParentRoot = l.attestedParentRoot[:]
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
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1 + float64(l.increaseActiveParticipantsBy))
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
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1 + float64(l.increaseActiveParticipantsBy))
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
	l.FinalizedState = finalizedState

	return l
}

func (l *TestLightClient) setupTestDeneb() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().DenebForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(l.increaseAttestedSlotBy)
	}

	signatureSlot := attestedSlot.Add(1)
	if l.increaseSignatureSlotBy > 0 {
		signatureSlot = signatureSlot.Add(l.increaseSignatureSlotBy)
	}

	// Attested State
	attestedState, err := NewBeaconStateDeneb()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	var signedFinalizedBlock interfaces.SignedBeaconBlock
	var finalizedState state.BeaconState
	// Finalized checkpoint
	if !l.noFinalizedCheckpoint {
		var finalizedSlot primitives.Slot

		if l.finalizedCheckpointInPrevFork {
			finalizedSlot = primitives.Slot(uint64(params.BeaconConfig().CapellaForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
			if l.increaseFinalizedSlotBy > 0 {
				finalizedSlot = finalizedSlot.Add(l.increaseFinalizedSlotBy)
			}

			finalizedState, err = NewBeaconStateCapella()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

			finalizedBlock := NewBeaconBlockCapella()
			require.NoError(l.T, err)
			finalizedBlock.Block.Slot = finalizedSlot
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
			finalizedHeader, err := signedFinalizedBlock.Header()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
			finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
			require.NoError(l.T, err)
			finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
		} else {
			finalizedSlot = primitives.Slot(uint64(params.BeaconConfig().DenebForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
			if l.increaseFinalizedSlotBy > 0 {
				finalizedSlot = finalizedSlot.Add(l.increaseFinalizedSlotBy)
			}

			finalizedState, err = NewBeaconStateDeneb()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

			finalizedBlock := NewBeaconBlockDeneb()
			require.NoError(l.T, err)
			finalizedBlock.Block.Slot = finalizedSlot
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
			finalizedHeader, err := signedFinalizedBlock.Header()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
			finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
			require.NoError(l.T, err)
			finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
		}

		// Set the finalized checkpoint
		finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		finalizedCheckpoint := &ethpb.Checkpoint{
			Epoch: slots.ToEpoch(finalizedSlot),
			Root:  finalizedBlockRoot[:],
		}
		require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))
	}

	// Attested Block
	attestedBlock := NewBeaconBlockDeneb()
	attestedBlock.Block.Slot = attestedSlot
	attestedBlock.Block.ParentRoot = l.attestedParentRoot[:]
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
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1 + float64(l.increaseActiveParticipantsBy))
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
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1 + float64(l.increaseActiveParticipantsBy))
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
	l.FinalizedState = finalizedState

	return l
}

func (l *TestLightClient) setupTestElectra() *TestLightClient {
	ctx := context.Background()

	attestedSlot := primitives.Slot(uint64(params.BeaconConfig().ElectraForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)).Add(1)
	if l.increaseAttestedSlotBy > 0 {
		attestedSlot = attestedSlot.Add(l.increaseAttestedSlotBy)
	}

	signatureSlot := attestedSlot.Add(1)
	if l.increaseSignatureSlotBy > 0 {
		signatureSlot = signatureSlot.Add(l.increaseSignatureSlotBy)
	}

	// Attested State & Block
	attestedState, err := NewBeaconStateElectra()
	require.NoError(l.T, err)
	require.NoError(l.T, attestedState.SetSlot(attestedSlot))

	var signedFinalizedBlock interfaces.SignedBeaconBlock
	var finalizedState state.BeaconState
	// Finalized checkpoint
	if !l.noFinalizedCheckpoint {
		var finalizedSlot primitives.Slot

		if l.finalizedCheckpointInPrevFork {
			finalizedSlot = primitives.Slot(uint64(params.BeaconConfig().DenebForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
			if l.increaseFinalizedSlotBy > 0 {
				finalizedSlot = finalizedSlot.Add(l.increaseFinalizedSlotBy)
			}

			finalizedState, err = NewBeaconStateDeneb()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

			finalizedBlock := NewBeaconBlockDeneb()
			require.NoError(l.T, err)
			finalizedBlock.Block.Slot = finalizedSlot
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
			finalizedHeader, err := signedFinalizedBlock.Header()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
			finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
			require.NoError(l.T, err)
			finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
		} else {
			finalizedSlot = primitives.Slot(uint64(params.BeaconConfig().ElectraForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch))
			if l.increaseFinalizedSlotBy > 0 {
				finalizedSlot = finalizedSlot.Add(l.increaseFinalizedSlotBy)
			}

			finalizedState, err = NewBeaconStateElectra()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetSlot(finalizedSlot))

			finalizedBlock := NewBeaconBlockElectra()
			require.NoError(l.T, err)
			finalizedBlock.Block.Slot = finalizedSlot
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
			finalizedHeader, err := signedFinalizedBlock.Header()
			require.NoError(l.T, err)
			require.NoError(l.T, finalizedState.SetLatestBlockHeader(finalizedHeader.Header))
			finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
			require.NoError(l.T, err)
			finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
			signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
			require.NoError(l.T, err)
		}

		// Set the finalized checkpoint
		finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
		require.NoError(l.T, err)
		finalizedCheckpoint := &ethpb.Checkpoint{
			Epoch: slots.ToEpoch(finalizedSlot),
			Root:  finalizedBlockRoot[:],
		}
		require.NoError(l.T, attestedState.SetFinalizedCheckpoint(finalizedCheckpoint))
	}

	// Attested Block
	attestedBlock := NewBeaconBlockElectra()
	attestedBlock.Block.Slot = attestedSlot
	attestedBlock.Block.ParentRoot = l.attestedParentRoot[:]
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
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1 + float64(l.increaseActiveParticipantsBy))
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
			trueBitNum = uint64((float64(params.BeaconConfig().SyncCommitteeSize) * 2.0 / 3.0) + 1 + float64(l.increaseActiveParticipantsBy))
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
	l.FinalizedState = finalizedState

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

func MockOptimisticUpdate() (interfaces.LightClientOptimisticUpdate, error) {
	pbUpdate := &ethpb.LightClientOptimisticUpdateAltair{
		AttestedHeader: &ethpb.LightClientHeaderAltair{
			Beacon: &ethpb.BeaconBlockHeader{
				Slot:       primitives.Slot(32),
				ParentRoot: make([]byte, 32),
				StateRoot:  make([]byte, 32),
				BodyRoot:   make([]byte, 32),
			},
		},
		SyncAggregate: &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, 64),
			SyncCommitteeSignature: make([]byte, 96),
		},
		SignatureSlot: primitives.Slot(33),
	}
	return lightclienttypes.NewWrappedOptimisticUpdateAltair(pbUpdate)
}

func MockFinalityUpdate() (interfaces.LightClientFinalityUpdate, error) {
	finalityBranch := make([][]byte, fieldparams.FinalityBranchDepth)
	for i := 0; i < len(finalityBranch); i++ {
		finalityBranch[i] = make([]byte, 32)
	}

	pbUpdate := &ethpb.LightClientFinalityUpdateAltair{
		FinalizedHeader: &ethpb.LightClientHeaderAltair{
			Beacon: &ethpb.BeaconBlockHeader{
				Slot:       primitives.Slot(31),
				ParentRoot: make([]byte, 32),
				StateRoot:  make([]byte, 32),
				BodyRoot:   make([]byte, 32),
			},
		},
		FinalityBranch: finalityBranch,
		AttestedHeader: &ethpb.LightClientHeaderAltair{
			Beacon: &ethpb.BeaconBlockHeader{
				Slot:       primitives.Slot(32),
				ParentRoot: make([]byte, 32),
				StateRoot:  make([]byte, 32),
				BodyRoot:   make([]byte, 32),
			},
		},
		SyncAggregate: &ethpb.SyncAggregate{
			SyncCommitteeBits:      make([]byte, 64),
			SyncCommitteeSignature: make([]byte, 96),
		},
		SignatureSlot: primitives.Slot(33),
	}
	return lightclienttypes.NewWrappedFinalityUpdateAltair(pbUpdate)
}
