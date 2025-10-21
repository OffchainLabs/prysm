package kv

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/testing/assert"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"google.golang.org/protobuf/proto"
)

func TestStore_JustifiedCheckpoint_CanSaveRetrieve(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	root := bytesutil.ToBytes32([]byte{'A'})
	cp := &ethpb.Checkpoint{
		Epoch: 10,
		Root:  root[:],
	}
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(1))
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveJustifiedCheckpoint(ctx, cp))

	retrieved, err := db.JustifiedCheckpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(cp, retrieved), "Wanted %v, received %v", cp, retrieved)
}

func TestStore_JustifiedCheckpoint_Recover(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	blk := util.HydrateSignedBeaconBlock(&ethpb.SignedBeaconBlock{})
	r, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	cp := &ethpb.Checkpoint{
		Epoch: 2,
		Root:  r[:],
	}
	wb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wb))
	require.NoError(t, db.SaveJustifiedCheckpoint(ctx, cp))
	retrieved, err := db.JustifiedCheckpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(cp, retrieved), "Wanted %v, received %v", cp, retrieved)
}

func TestStore_FinalizedCheckpoint_CanSaveRetrieve(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()

	genesis := bytesutil.ToBytes32([]byte{'G', 'E', 'N', 'E', 'S', 'I', 'S'})
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesis))

	blk := util.NewBeaconBlock()
	blk.Block.ParentRoot = genesis[:]
	blk.Block.Slot = 40

	root, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	cp := &ethpb.Checkpoint{
		Epoch: 5,
		Root:  root[:],
	}

	// a valid chain is required to save finalized checkpoint.
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))
	st, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(1))
	// a state is required to save checkpoint
	require.NoError(t, db.SaveState(ctx, st, root))

	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, cp))

	retrieved, err := db.FinalizedCheckpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(cp, retrieved), "Wanted %v, received %v", cp, retrieved)
}

func TestStore_FinalizedCheckpoint_Recover(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	blk := util.HydrateSignedBeaconBlock(&ethpb.SignedBeaconBlock{})
	r, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	cp := &ethpb.Checkpoint{
		Epoch: 2,
		Root:  r[:],
	}
	wb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, r))
	require.NoError(t, db.SaveBlock(ctx, wb))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, cp))
	retrieved, err := db.FinalizedCheckpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(cp, retrieved), "Wanted %v, received %v", cp, retrieved)
}

func TestStore_JustifiedCheckpoint_DefaultIsZeroHash(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()

	cp := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	retrieved, err := db.JustifiedCheckpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(cp, retrieved), "Wanted %v, received %v", cp, retrieved)
}

func TestStore_FinalizedCheckpoint_DefaultIsZeroHash(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()

	cp := &ethpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	retrieved, err := db.FinalizedCheckpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(cp, retrieved), "Wanted %v, received %v", cp, retrieved)
}

func TestStore_FinalizedCheckpoint_StateMustExist(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()
	cp := &ethpb.Checkpoint{
		Epoch: 5,
		Root:  []byte{'B'},
	}

	require.ErrorContains(t, errMissingStateForCheckpoint.Error(), db.SaveFinalizedCheckpoint(ctx, cp))
}

// Regression test: verify that saving a checkpoint triggers recovery which writes
// the state summary into the correct stateSummaryBucket so that HasStateSummary/StateSummary see it.
func TestRecoverStateSummary_WritesToStateSummaryBucket(t *testing.T) {
	db := setupDB(t)
	ctx := t.Context()

	// Create a block without saving a state or summary, so recovery is needed.
	blk := util.HydrateSignedBeaconBlock(&ethpb.SignedBeaconBlock{})
	root, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))

	// Precondition: summary not present yet.
	require.Equal(t, false, db.HasStateSummary(ctx, root))

	// Saving justified checkpoint should trigger recovery path calling recoverStateSummary.
	cp := &ethpb.Checkpoint{Epoch: 2, Root: root[:]}
	require.NoError(t, db.SaveJustifiedCheckpoint(ctx, cp))

	// Postcondition: summary is visible via the public summary APIs (which read stateSummaryBucket).
	require.Equal(t, true, db.HasStateSummary(ctx, root))
	summary, err := db.StateSummary(ctx, root)
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.DeepEqual(t, &ethpb.StateSummary{Slot: blk.Block.Slot, Root: root[:]}, summary)
}
