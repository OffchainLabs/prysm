package kv

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/genesis"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/OffchainLabs/prysm/v7/time/slots"
)

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
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	cb := util.NewBeaconBlock()
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
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)

	cb := util.NewBeaconBlock()
	cb.Block.Slot = boundarySlot - 1
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
	require.Equal(t, checkpointEpoch, jcp.Epoch)
	require.DeepEqual(t, broot[:], jcp.Root)
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
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	cb := util.NewBeaconBlock()
	cb.Block.Slot = 31
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)
	require.ErrorContains(t, "non epoch boundary offset", db.SaveOrigin(ctx, csb, cbb))
}
