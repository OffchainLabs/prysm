package node

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/genesis"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestValidateArchiveOrigin(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	require.NoError(t, genesis.Initialize(t.Context(), t.TempDir()))
	gvr := genesis.ValidatorsRoot()

	t.Run("epoch boundary is accepted", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetGenesisValidatorsRoot(gvr[:]))
		require.NoError(t, st.SetSlot(8192))
		require.NoError(t, validateArchiveOrigin(st))
	})

	t.Run("mid-epoch slot is rejected", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetGenesisValidatorsRoot(gvr[:]))
		require.NoError(t, st.SetSlot(8193))
		require.ErrorContains(t, "must be an epoch boundary", validateArchiveOrigin(st))
	})

	t.Run("wrong network is rejected", func(t *testing.T) {
		st, err := util.NewBeaconState()
		require.NoError(t, err)
		require.NoError(t, st.SetGenesisValidatorsRoot(make([]byte, 32)))
		require.NoError(t, st.SetSlot(8192))
		require.ErrorContains(t, "different network", validateArchiveOrigin(st))
	})

	t.Run("nil is rejected", func(t *testing.T) {
		require.ErrorContains(t, "nil", validateArchiveOrigin(nil))
	})
}

// Anchoring the tree is irreversible, so an archive origin the node cannot backfill down to must be
// rejected before anything is written. Otherwise the operator's only recovery is deleting the database:
// InitializeArchiveOrigin refuses to move an offset that already exists.
func TestFinalizeArchiveOrigin_RejectedOriginLeavesDBReAnchorable(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	flags.Init(&flags.GlobalFlags{StateDiffExponents: []int{21, 18, 16, 13, 11, 9, 5}})

	ctx := t.Context()
	beaconDB := testDB.SetupDB(t)
	store := beaconDB.(*kv.Store)

	// A checkpoint-synced node whose sync origin is one epoch in.
	syncOrigin := uint64(params.BeaconConfig().SlotsPerEpoch)
	require.NoError(t, store.SaveBackfillStatus(ctx, &dbval.BackfillStatus{
		OriginSlot: syncOrigin,
		LowSlot:    syncOrigin,
	}))

	tooHigh, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, tooHigh.SetSlot(primitives.Slot(syncOrigin)*4))
	tooHighSlot := tooHigh.Slot()

	b := &BeaconNode{ctx: ctx, db: beaconDB, archiveOriginState: tooHigh, ArchiveOriginSlot: &tooHighSlot}
	require.ErrorContains(t, "is above the sync origin slot", b.finalizeArchiveOrigin(ctx))

	// Nothing was written, so the corrected origin anchors cleanly rather than hitting
	// "archive origin state changed" / "already anchored".
	_, err = store.ArchiveStatus(ctx)
	require.ErrorIs(t, err, kv.ErrNotFound)

	good, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, good.SetSlot(primitives.Slot(syncOrigin)))
	goodSlot := good.Slot()

	b = &BeaconNode{ctx: ctx, db: beaconDB, archiveOriginState: good, ArchiveOriginSlot: &goodSlot}
	require.NoError(t, b.finalizeArchiveOrigin(ctx))

	as, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, goodSlot, as.OriginSlot)
	require.Equal(t, true, b.archiveRegenPending)
	require.IsNil(t, b.archiveOriginState)
}
