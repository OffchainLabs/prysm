package kv

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/genesis"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

// linkOriginBlock makes st.latest_block_header describe b, as it would on a real chain.
func linkOriginBlock(t *testing.T, st state.BeaconState, b *ethpb.SignedBeaconBlock) {
	b.Block.StateRoot = bytesutil.PadTo([]byte("origin-state-root"), 32)
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	bodyRoot, err := wsb.Block().Body().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, st.SetLatestBlockHeader(&ethpb.BeaconBlockHeader{
		Slot:          b.Block.Slot,
		ProposerIndex: b.Block.ProposerIndex,
		ParentRoot:    b.Block.ParentRoot,
		StateRoot:     b.Block.StateRoot,
		BodyRoot:      bodyRoot[:],
	}))
}

func TestSaveOrigin(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	// Embedded Genesis works with Mainnet config
	params.OverrideBeaconConfig(params.MainnetConfig())

	ctx := t.Context()
	db := setupDB(t)

	// Initialize genesis with mainnet config - this will load the embedded mainnet state
	require.NoError(t, genesis.Initialize(ctx, t.TempDir()))

	// Get the initialized genesis state
	st, err := genesis.State()
	require.NoError(t, err)

	sb, err := st.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, db.LoadGenesis(ctx, sb))

	// this is necessary for mainnet, because LoadGenesis is short-circuited by the embedded state,
	// so the genesis root key is never written to the db.
	require.NoError(t, db.EnsureEmbeddedGenesis(ctx))

	cst, err := util.NewBeaconState()
	require.NoError(t, err)
	cb := util.NewBeaconBlock()
	linkOriginBlock(t, cst, cb)
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, db.SaveOrigin(ctx, csb, cbb))

	broot, err := scb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, true, db.IsFinalizedBlock(ctx, broot))
}

func TestSaveOrigin_BoundaryBlockCheckpointEpoch(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())

	ctx := t.Context()
	db := setupDB(t)

	// The origin state sits at the first slot of the checkpoint epoch, while the origin
	// block sits in the previous epoch because the checkpoint epoch's first slot is empty.
	checkpointEpoch := primitives.Epoch(2)
	boundarySlot, err := slots.EpochStart(checkpointEpoch)
	require.NoError(t, err)

	cst, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, cst.SetSlot(boundarySlot))

	cb := util.NewBeaconBlock()
	cb.Block.Slot = boundarySlot - 1
	linkOriginBlock(t, cst, cb)
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)

	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)

	require.NoError(t, db.SaveOrigin(ctx, csb, cbb))

	broot, err := scb.Block().HashTreeRoot()
	require.NoError(t, err)

	fcp, err := db.FinalizedCheckpoint(ctx)
	require.NoError(t, err)
	require.Equal(t, checkpointEpoch, fcp.Epoch)
	require.DeepEqual(t, broot[:], fcp.Root)

	jcp, err := db.JustifiedCheckpoint(ctx)
	require.NoError(t, err)
	require.Equal(t, cst.CurrentJustifiedCheckpoint().Epoch, jcp.Epoch)
	require.DeepEqual(t, broot[:], jcp.Root)
}

func TestSaveOrigin_UnfinalizedAnchor(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())

	ctx := t.Context()
	db := setupDB(t)

	originEpoch := primitives.Epoch(100)
	justifiedEpoch := primitives.Epoch(10)
	boundarySlot, err := slots.EpochStart(originEpoch)
	require.NoError(t, err)

	cst, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, cst.SetSlot(boundarySlot))
	require.NoError(t, cst.SetCurrentJustifiedCheckpoint(&ethpb.Checkpoint{
		Epoch: justifiedEpoch,
		Root:  bytesutil.PadTo([]byte("justified"), 32),
	}))
	require.NoError(t, cst.SetFinalizedCheckpoint(&ethpb.Checkpoint{
		Epoch: justifiedEpoch - 2,
		Root:  bytesutil.PadTo([]byte("finalized"), 32),
	}))

	cb := util.NewBeaconBlock()
	cb.Block.Slot = boundarySlot
	linkOriginBlock(t, cst, cb)
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)

	require.NoError(t, db.SaveOrigin(ctx, csb, cbb))

	broot, err := scb.Block().HashTreeRoot()
	require.NoError(t, err)

	fcp, err := db.FinalizedCheckpoint(ctx)
	require.NoError(t, err)
	require.Equal(t, originEpoch, fcp.Epoch)
	require.DeepEqual(t, broot[:], fcp.Root)

	jcp, err := db.JustifiedCheckpoint(ctx)
	require.NoError(t, err)
	require.Equal(t, justifiedEpoch, jcp.Epoch)
	require.DeepEqual(t, broot[:], jcp.Root)

	require.Equal(t, true, db.IsFinalizedBlock(ctx, broot))
}

func TestSaveOrigin_BlockDoesNotMatchState(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())

	ctx := t.Context()
	db := setupDB(t)

	cst, err := util.NewBeaconState()
	require.NoError(t, err)
	cb := util.NewBeaconBlock()
	linkOriginBlock(t, cst, cb)
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)

	cb.Block.ProposerIndex = 42
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)

	require.ErrorIs(t, db.SaveOrigin(ctx, csb, cbb), errOriginBlockMismatch)
}

func TestSaveOrigin_StateDiffNonEpochBoundarySlot(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetCfg()
	setDefaultStateDiffExponents()

	ctx := t.Context()
	db := setupDB(t)

	require.NoError(t, genesis.Initialize(ctx, t.TempDir()))

	st, err := genesis.State()
	require.NoError(t, err)

	sb, err := st.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, db.LoadGenesis(ctx, sb))
	require.NoError(t, db.EnsureEmbeddedGenesis(ctx))

	cst, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, cst.SetSlot(31))
	cb := util.NewBeaconBlock()
	cb.Block.Slot = 31
	linkOriginBlock(t, cst, cb)
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)
	require.ErrorContains(t, "non epoch boundary offset", db.SaveOrigin(ctx, csb, cbb))
}
