package archive

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// A round walks to the finalized epoch start and then offers the handoff. The handoff is offered with the
// first boundary the walk has *not* written, which is what stategen compares against the finalized slot.
func TestRound_WalksThenOffersHandoff(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	ctx := t.Context()
	store := testDB.SetupDB(t).(*kv.Store)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	fEpoch := primitives.Epoch(2)
	fSlot := primitives.Slot(fEpoch) * slotsPerEpoch

	genesisState, keys := util.DeterministicGenesisState(t, 64)
	require.NoError(t, store.InitializeArchiveOrigin(ctx, genesisState))
	buildChain(t, ctx, store, genesisState, keys, fSlot, nil)
	require.NoError(t, store.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{
		Epoch: fEpoch,
		Root:  finalizedRootAtSlot(t, store, fSlot),
	}))

	sg := &mockStateManager{pending: true}
	svc := New(ctx, store, sg, nil, nil)
	as, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	svc.setStatus(as)

	// Handoff refused: the walk has caught up but the service keeps going.
	done, err := svc.round(ctx)
	require.NoError(t, err)
	require.Equal(t, false, done)
	require.Equal(t, fSlot, svc.status().RegeneratedThroughSlot)
	require.Equal(t, fSlot+slotsPerEpoch, sg.nextArg)

	// Handoff accepted: the status is marked complete and persisted.
	sg.handoff = true
	done, err = svc.round(ctx)
	require.NoError(t, err)
	require.Equal(t, true, done)
	persisted, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, true, persisted.Complete)

	// A completed archive no longer blocks live writes into the tree.
	_, pendingInDB := archivePendingForTesting(t, store)
	require.Equal(t, false, pendingInDB)
}

// A service whose regeneration is already complete must not walk again.
func TestStart_NoopWhenNotPending(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	store := testDB.SetupDB(t).(*kv.Store)
	svc := New(t.Context(), store, &mockStateManager{pending: false}, nil, func() error {
		t.Fatal("backfill waiter must not be called when regeneration is complete")
		return nil
	})
	svc.Start()
}

func finalizedRootAtSlot(t *testing.T, store *kv.Store, slot primitives.Slot) []byte {
	_, roots, err := store.HighestRootsBelowSlot(t.Context(), slot+1)
	require.NoError(t, err)
	require.Equal(t, 1, len(roots))
	return roots[0][:]
}

func archivePendingForTesting(t *testing.T, store *kv.Store) (primitives.Slot, bool) {
	as, err := store.ArchiveStatus(t.Context())
	require.NoError(t, err)
	return as.RegeneratedThroughSlot, !as.Complete
}
