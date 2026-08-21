package kv

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/genesis"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
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

	// A pre-Gloas origin leaves envelope coverage UNINITIALIZED.
	_, err = db.EnvelopeCoverage(ctx)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestSaveOriginGloasSeedsEnvelopeCoverage(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig().Copy()
	cfg.AltairForkEpoch = 0
	cfg.BellatrixForkEpoch = 0
	cfg.CapellaForkEpoch = 0
	cfg.DenebForkEpoch = 0
	cfg.ElectraForkEpoch = 0
	cfg.FuluForkEpoch = 0
	cfg.GloasForkEpoch = 0
	params.OverrideBeaconConfig(cfg)
	params.BeaconConfig().InitializeForkSchedule()

	ctx := t.Context()
	db := setupDB(t)

	cst, err := util.NewBeaconStateGloas(func(st *ethpb.BeaconStateGloas) error {
		st.Fork = &ethpb.Fork{
			PreviousVersion: params.BeaconConfig().FuluForkVersion,
			CurrentVersion:  params.BeaconConfig().GloasForkVersion,
		}
		return nil
	})
	require.NoError(t, err)
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	cb := util.NewBeaconBlockGloas()
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, db.SaveOrigin(ctx, csb, cbb))

	broot, err := scb.Block().HashTreeRoot()
	require.NoError(t, err)

	// A Gloas+ origin seeds an empty coverage interval anchored at the
	// origin block, written only after every other origin write succeeded.
	cov, err := db.EnvelopeCoverage(ctx)
	require.NoError(t, err)
	require.Equal(t, uint32(1), cov.FormatVersion)
	require.Equal(t, uint64(scb.Block().Slot()), cov.LowSlot)
	require.Equal(t, uint64(scb.Block().Slot()), cov.HighSlot)
	require.DeepEqual(t, broot[:], cov.HighAnchorRoot)
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
