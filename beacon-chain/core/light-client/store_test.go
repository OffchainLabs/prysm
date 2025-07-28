package light_client

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v6/async/event"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/db"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	p2pTesting "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

func TestLightClientStore(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.AltairForkEpoch = 1
	cfg.BellatrixForkEpoch = 2
	cfg.CapellaForkEpoch = 3
	cfg.DenebForkEpoch = 4
	cfg.ElectraForkEpoch = 5
	params.OverrideBeaconConfig(cfg)

	// Initialize the light client store
	lcStore := NewLightClientStore(testDB.SetupDB(t), &p2pTesting.FakeP2P{}, new(event.Feed))

	// Create test light client updates for Capella and Deneb
	lCapella := util.NewTestLightClient(t, version.Capella)
	opUpdateCapella, err := NewLightClientOptimisticUpdateFromBeaconState(lCapella.Ctx, lCapella.State, lCapella.Block, lCapella.AttestedState, lCapella.AttestedBlock)
	require.NoError(t, err)
	require.NotNil(t, opUpdateCapella, "OptimisticUpdateCapella is nil")
	finUpdateCapella, err := NewLightClientFinalityUpdateFromBeaconState(lCapella.Ctx, lCapella.State, lCapella.Block, lCapella.AttestedState, lCapella.AttestedBlock, lCapella.FinalizedBlock)
	require.NoError(t, err)
	require.NotNil(t, finUpdateCapella, "FinalityUpdateCapella is nil")

	lDeneb := util.NewTestLightClient(t, version.Deneb)
	opUpdateDeneb, err := NewLightClientOptimisticUpdateFromBeaconState(lDeneb.Ctx, lDeneb.State, lDeneb.Block, lDeneb.AttestedState, lDeneb.AttestedBlock)
	require.NoError(t, err)
	require.NotNil(t, opUpdateDeneb, "OptimisticUpdateDeneb is nil")
	finUpdateDeneb, err := NewLightClientFinalityUpdateFromBeaconState(lDeneb.Ctx, lDeneb.State, lDeneb.Block, lDeneb.AttestedState, lDeneb.AttestedBlock, lDeneb.FinalizedBlock)
	require.NoError(t, err)
	require.NotNil(t, finUpdateDeneb, "FinalityUpdateDeneb is nil")

	// Initially the store should have nil values for both updates
	require.IsNil(t, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should be nil")
	require.IsNil(t, lcStore.LastOptimisticUpdate(), "lastOptimisticUpdate should be nil")

	// Set and get finality with Capella update. Optimistic update should be nil
	lcStore.SetLastFinalityUpdate(finUpdateCapella, false)
	require.Equal(t, finUpdateCapella, lcStore.LastFinalityUpdate(), "lastFinalityUpdate is wrong")
	require.IsNil(t, lcStore.LastOptimisticUpdate(), "lastOptimisticUpdate should be nil")

	// Set and get optimistic with Capella update. Finality update should be Capella
	lcStore.SetLastOptimisticUpdate(opUpdateCapella, false)
	require.Equal(t, opUpdateCapella, lcStore.LastOptimisticUpdate(), "lastOptimisticUpdate is wrong")
	require.Equal(t, finUpdateCapella, lcStore.LastFinalityUpdate(), "lastFinalityUpdate is wrong")

	// Set and get finality and optimistic with Deneb update
	lcStore.SetLastFinalityUpdate(finUpdateDeneb, false)
	lcStore.SetLastOptimisticUpdate(opUpdateDeneb, false)
	require.Equal(t, finUpdateDeneb, lcStore.LastFinalityUpdate(), "lastFinalityUpdate is wrong")
	require.Equal(t, opUpdateDeneb, lcStore.LastOptimisticUpdate(), "lastOptimisticUpdate is wrong")
}

func TestLightClientStore_SetLastFinalityUpdate(t *testing.T) {
	p2p := p2pTesting.NewTestP2P(t)
	lcStore := NewLightClientStore(testDB.SetupDB(t), p2p, new(event.Feed))

	// update 0 with basic data and no supermajority following an empty lastFinalityUpdate - should save and broadcast
	l0 := util.NewTestLightClient(t, version.Altair)
	update0, err := NewLightClientFinalityUpdateFromBeaconState(l0.Ctx, l0.State, l0.Block, l0.AttestedState, l0.AttestedBlock, l0.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, IsBetterFinalityUpdate(update0, lcStore.LastFinalityUpdate()), "update0 should be better than nil")
	// update0 should be valid for broadcast - meaning it should be broadcasted
	require.Equal(t, true, IsFinalityUpdateValidForBroadcast(update0, lcStore.LastFinalityUpdate()), "update0 should be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update0, true)
	require.Equal(t, update0, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, true, p2p.BroadcastCalled.Load(), "Broadcast should have been called after setting a new last finality update when previous is nil")
	p2p.BroadcastCalled.Store(false) // Reset for next test

	// update 1 with same finality slot, increased attested slot, and no supermajority - should save but not broadcast
	l1 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(1))
	update1, err := NewLightClientFinalityUpdateFromBeaconState(l1.Ctx, l1.State, l1.Block, l1.AttestedState, l1.AttestedBlock, l1.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, IsBetterFinalityUpdate(update1, update0), "update1 should be better than update0")
	// update1 should not be valid for broadcast - meaning it should not be broadcasted
	require.Equal(t, false, IsFinalityUpdateValidForBroadcast(update1, lcStore.LastFinalityUpdate()), "update1 should not be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update1, true)
	require.Equal(t, update1, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, false, p2p.BroadcastCalled.Load(), "Broadcast should not have been called after setting a new last finality update without supermajority")
	p2p.BroadcastCalled.Store(false) // Reset for next test

	// update 2 with same finality slot, increased attested slot, and supermajority - should save and broadcast
	l2 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(2), util.WithSupermajority(0))
	update2, err := NewLightClientFinalityUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, IsBetterFinalityUpdate(update2, update1), "update2 should be better than update1")
	// update2 should be valid for broadcast - meaning it should be broadcasted
	require.Equal(t, true, IsFinalityUpdateValidForBroadcast(update2, lcStore.LastFinalityUpdate()), "update2 should be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update2, true)
	require.Equal(t, update2, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, true, p2p.BroadcastCalled.Load(), "Broadcast should have been called after setting a new last finality update with supermajority")
	p2p.BroadcastCalled.Store(false) // Reset for next test

	// update 3 with same finality slot, increased attested slot, and supermajority - should save but not broadcast
	l3 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(3), util.WithSupermajority(0))
	update3, err := NewLightClientFinalityUpdateFromBeaconState(l3.Ctx, l3.State, l3.Block, l3.AttestedState, l3.AttestedBlock, l3.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, IsBetterFinalityUpdate(update3, update2), "update3 should be better than update2")
	// update3 should not be valid for broadcast - meaning it should not be broadcasted
	require.Equal(t, false, IsFinalityUpdateValidForBroadcast(update3, lcStore.LastFinalityUpdate()), "update3 should not be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update3, true)
	require.Equal(t, update3, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, false, p2p.BroadcastCalled.Load(), "Broadcast should not have been when previous was already broadcast")

	// update 4 with increased finality slot, increased attested slot, and supermajority - should save and broadcast
	l4 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedFinalizedSlot(1), util.WithIncreasedAttestedSlot(1), util.WithSupermajority(0))
	update4, err := NewLightClientFinalityUpdateFromBeaconState(l4.Ctx, l4.State, l4.Block, l4.AttestedState, l4.AttestedBlock, l4.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, IsBetterFinalityUpdate(update4, update3), "update4 should be better than update3")
	// update4 should be valid for broadcast - meaning it should be broadcasted
	require.Equal(t, true, IsFinalityUpdateValidForBroadcast(update4, lcStore.LastFinalityUpdate()), "update4 should be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update4, true)
	require.Equal(t, update4, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, true, p2p.BroadcastCalled.Load(), "Broadcast should have been called after a new finality update with increased finality slot")
	p2p.BroadcastCalled.Store(false) // Reset for next test

	// update 5 with the same new finality slot, increased attested slot, and supermajority - should save but not broadcast
	l5 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedFinalizedSlot(1), util.WithIncreasedAttestedSlot(2), util.WithSupermajority(0))
	update5, err := NewLightClientFinalityUpdateFromBeaconState(l5.Ctx, l5.State, l5.Block, l5.AttestedState, l5.AttestedBlock, l5.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, IsBetterFinalityUpdate(update5, update4), "update5 should be better than update4")
	// update5 should not be valid for broadcast - meaning it should not be broadcasted
	require.Equal(t, false, IsFinalityUpdateValidForBroadcast(update5, lcStore.LastFinalityUpdate()), "update5 should not be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update5, true)
	require.Equal(t, update5, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, false, p2p.BroadcastCalled.Load(), "Broadcast should not have been called when previous was already broadcast with supermajority")

	// update 6 with the same new finality slot, increased attested slot, and no supermajority - should save but not broadcast
	l6 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedFinalizedSlot(1), util.WithIncreasedAttestedSlot(3))
	update6, err := NewLightClientFinalityUpdateFromBeaconState(l6.Ctx, l6.State, l6.Block, l6.AttestedState, l6.AttestedBlock, l6.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, IsBetterFinalityUpdate(update6, update5), "update6 should be better than update5")
	// update6 should not be valid for broadcast - meaning it should not be broadcasted
	require.Equal(t, false, IsFinalityUpdateValidForBroadcast(update6, lcStore.LastFinalityUpdate()), "update6 should not be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update6, true)
	require.Equal(t, update6, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, false, p2p.BroadcastCalled.Load(), "Broadcast should not have been called when previous was already broadcast with supermajority")
}

func TestLightClientStore_SaveLCData(t *testing.T) {
	t.Run("no parent in cache or db - new is head", func(t *testing.T) {
		db := testDB.SetupDB(t)
		s := NewLightClientStore(db, &p2pTesting.FakeP2P{}, new(event.Feed))
		require.NotNil(t, s)

		l := util.NewTestLightClient(t, version.Altair)

		// make the new block the head
		blkRoot, err := l.Block.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, db.SaveState(l.Ctx, l.State, blkRoot))
		require.NoError(t, db.SaveHeadBlockRoot(l.Ctx, blkRoot))

		require.NoError(t, s.SaveLCData(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock), "Failed to save light client data")

		update, err := NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		finalityUpdate, err := NewLightClientFinalityUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		optimisticUpdate, err := NewLightClientOptimisticUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock)
		require.NoError(t, err)
		attstedBlkRoot, err := l.AttestedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		require.DeepEqual(t, finalityUpdate, s.lastFinalityUpdate, "Expected to find the last finality update in the store")
		require.DeepEqual(t, optimisticUpdate, s.lastOptimisticUpdate, "Expected to find the last optimistic update in the store")
		require.DeepEqual(t, update, *s.nonFinalityCache.items[attstedBlkRoot].bestUpdate, "Expected to find the update in the non-finality cache")
		require.DeepEqual(t, finalityUpdate, *s.nonFinalityCache.items[attstedBlkRoot].bestFinalityUpdate, "Expected to find the finality update in the non-finality cache")
	})

	t.Run("no parent in cache or db - new not head", func(t *testing.T) {
		db := testDB.SetupDB(t)
		s := NewLightClientStore(db, &p2pTesting.FakeP2P{}, new(event.Feed))
		require.NotNil(t, s)

		l := util.NewTestLightClient(t, version.Altair)

		// make the new block the head
		blkRoot, err := l.FinalizedBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, db.SaveState(l.Ctx, l.FinalizedState, blkRoot))
		require.NoError(t, db.SaveHeadBlockRoot(l.Ctx, blkRoot))

		require.NoError(t, s.SaveLCData(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock), "Failed to save light client data")

		update, err := NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		finalityUpdate, err := NewLightClientFinalityUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		optimisticUpdate, err := NewLightClientOptimisticUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock)
		require.NoError(t, err)
		attstedBlkRoot, err := l.AttestedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		require.IsNil(t, s.lastFinalityUpdate, "Expected to not find the last finality update in the store since the block is not head")
		require.DeepEqual(t, optimisticUpdate, s.lastOptimisticUpdate, "Expected to find the last optimistic update in the store")
		require.DeepEqual(t, update, *s.nonFinalityCache.items[attstedBlkRoot].bestUpdate, "Expected to find the update in the non-finality cache")
		require.DeepEqual(t, finalityUpdate, *s.nonFinalityCache.items[attstedBlkRoot].bestFinalityUpdate, "Expected to find the finality update in the non-finality cache")
	})

	t.Run("parent in db", func(t *testing.T) {
		db := testDB.SetupDB(t)
		s := NewLightClientStore(db, &p2pTesting.FakeP2P{}, new(event.Feed))
		require.NotNil(t, s)

		l := util.NewTestLightClient(t, version.Altair)

		// save an update for this period in db
		period := slots.SyncCommitteePeriod(slots.ToEpoch(l.AttestedBlock.Block().Slot()))
		update, err := NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		require.NoError(t, db.SaveLightClientUpdate(l.Ctx, period, update), "Failed to save light client update in db")

		l2 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(1), util.WithSupermajority(0)) // updates from this setup should be all better
		// make the new block the head
		blkRoot, err := l2.Block.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, db.SaveState(l2.Ctx, l2.State, blkRoot))
		require.NoError(t, db.SaveHeadBlockRoot(l2.Ctx, blkRoot))

		require.NoError(t, s.SaveLCData(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock), "Failed to save light client data")

		update, err = NewLightClientUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock)
		require.NoError(t, err)
		finalityUpdate, err := NewLightClientFinalityUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock)
		require.NoError(t, err)
		optimisticUpdate, err := NewLightClientOptimisticUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock)
		require.NoError(t, err)
		attstedBlkRoot, err := l2.AttestedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		require.DeepEqual(t, finalityUpdate, s.lastFinalityUpdate, "Expected to find the last finality update in the store")
		require.DeepEqual(t, optimisticUpdate, s.lastOptimisticUpdate, "Expected to find the last optimistic update in the store")
		require.DeepEqual(t, update, *s.nonFinalityCache.items[attstedBlkRoot].bestUpdate, "Expected to find the update in the non-finality cache")
		require.DeepEqual(t, finalityUpdate, *s.nonFinalityCache.items[attstedBlkRoot].bestFinalityUpdate, "Expected to find the finality update in the non-finality cache")
	})

	t.Run("parent in cache", func(t *testing.T) {
		db := testDB.SetupDB(t)
		s := NewLightClientStore(db, &p2pTesting.FakeP2P{}, new(event.Feed))
		require.NotNil(t, s)

		l := util.NewTestLightClient(t, version.Altair)
		l2 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(1), util.WithSupermajority(0)) // updates from this setup should be all better

		// save the cache item for this period in cache
		period := slots.SyncCommitteePeriod(slots.ToEpoch(l.AttestedBlock.Block().Slot()))
		update, err := NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		finalityUpdate, err := NewLightClientFinalityUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		item := &lightClientCacheItem{
			period:             period,
			bestUpdate:         &update,
			bestFinalityUpdate: &finalityUpdate,
		}
		attestedBlockRoot := l2.AttestedBlock.Block().ParentRoot() // we want this item to be the parent of the new block
		s.nonFinalityCache.items[attestedBlockRoot] = item

		// make the new block the head
		blkRoot, err := l2.Block.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, db.SaveState(l2.Ctx, l2.State, blkRoot))
		require.NoError(t, db.SaveHeadBlockRoot(l2.Ctx, blkRoot))

		require.NoError(t, s.SaveLCData(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock), "Failed to save light client data")

		update, err = NewLightClientUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock)
		require.NoError(t, err)
		finalityUpdate, err = NewLightClientFinalityUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock)
		require.NoError(t, err)
		optimisticUpdate, err := NewLightClientOptimisticUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock)
		require.NoError(t, err)
		attstedBlkRoot, err := l2.AttestedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		require.DeepEqual(t, finalityUpdate, s.lastFinalityUpdate, "Expected to find the last finality update in the store")
		require.DeepEqual(t, optimisticUpdate, s.lastOptimisticUpdate, "Expected to find the last optimistic update in the store")
		require.DeepEqual(t, update, *s.nonFinalityCache.items[attstedBlkRoot].bestUpdate, "Expected to find the update in the non-finality cache")
		require.DeepEqual(t, finalityUpdate, *s.nonFinalityCache.items[attstedBlkRoot].bestFinalityUpdate, "Expected to find the finality update in the non-finality cache")
	})

	t.Run("parent in the previous period", func(t *testing.T) {
		db := testDB.SetupDB(t)
		s := NewLightClientStore(db, &p2pTesting.FakeP2P{}, new(event.Feed))
		require.NotNil(t, s)

		l := util.NewTestLightClient(t, version.Altair)
		l2 := util.NewTestLightClient(t, version.Bellatrix, util.WithIncreasedAttestedSlot(1), util.WithSupermajority(0)) // updates from this setup should be all better

		// save the cache item for this period1 in cache
		period1 := slots.SyncCommitteePeriod(slots.ToEpoch(l.AttestedBlock.Block().Slot()))
		update, err := NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		finalityUpdate, err := NewLightClientFinalityUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		item := &lightClientCacheItem{
			period:             period1,
			bestUpdate:         &update,
			bestFinalityUpdate: &finalityUpdate,
		}
		attestedBlockRoot := l2.AttestedBlock.Block().ParentRoot() // we want this item to be the parent of the new block
		s.nonFinalityCache.items[attestedBlockRoot] = item

		// make the new block the head
		blkRoot, err := l2.Block.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, db.SaveState(l2.Ctx, l2.State, blkRoot))
		require.NoError(t, db.SaveHeadBlockRoot(l2.Ctx, blkRoot))

		require.NoError(t, s.SaveLCData(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock), "Failed to save light client data")

		update, err = NewLightClientUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock)
		require.NoError(t, err)
		finalityUpdate, err = NewLightClientFinalityUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock)
		require.NoError(t, err)
		optimisticUpdate, err := NewLightClientOptimisticUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock)
		require.NoError(t, err)
		attstedBlkRoot, err := l2.AttestedBlock.Block().HashTreeRoot()
		require.NoError(t, err)

		require.DeepEqual(t, finalityUpdate, s.lastFinalityUpdate, "Expected to find the last finality update in the store")
		require.DeepEqual(t, optimisticUpdate, s.lastOptimisticUpdate, "Expected to find the last optimistic update in the store")
		require.DeepEqual(t, update, *s.nonFinalityCache.items[attstedBlkRoot].bestUpdate, "Expected to find the update in the non-finality cache")
		require.DeepEqual(t, finalityUpdate, *s.nonFinalityCache.items[attstedBlkRoot].bestFinalityUpdate, "Expected to find the finality update in the non-finality cache")
	})
}

func TestLightClientStore_MigrateToCold(t *testing.T) {
	// This tests the scenario where chain advances but the cache is empty.
	// It should see that there is nothing in the cache to migrate and just update the tail to the new finalized root.
	t.Run("empty cache", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)
		ctx := context.Background()

		finalizedBlockRoot := saveInitialFinalizedCheckpointData(t, ctx, beaconDB)

		s := NewLightClientStore(beaconDB, &p2pTesting.FakeP2P{}, new(event.Feed))
		require.NotNil(t, s)

		require.DeepSSZEqual(t, finalizedBlockRoot, s.nonFinalityCache.tail)

		for i := 0; i < 3; i++ {
			newBlock := util.NewBeaconBlock()
			newBlock.Block.Slot = primitives.Slot(32 + uint64(i))
			newBlock.Block.ParentRoot = finalizedBlockRoot[:]
			signedNewBlock, err := blocks.NewSignedBeaconBlock(newBlock)
			require.NoError(t, err)
			blockRoot, err := signedNewBlock.Block().HashTreeRoot()
			require.NoError(t, err)
			require.NoError(t, beaconDB.SaveBlock(ctx, signedNewBlock))
			finalizedBlockRoot = blockRoot
		}

		err := s.MigrateToCold(ctx, finalizedBlockRoot)
		require.NoError(t, err)
		require.Equal(t, 0, len(s.nonFinalityCache.items))
		require.DeepSSZEqual(t, finalizedBlockRoot, s.nonFinalityCache.tail)
	})

	// This tests the scenario where chain advances but the CANONICAL cache is empty.
	// It should see that there is nothing in the canonical cache to migrate and just update the tail to the new finalized root AND delete anything non-canonical.
	t.Run("non canonical cache", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)
		ctx := context.Background()

		finalizedBlockRoot := saveInitialFinalizedCheckpointData(t, ctx, beaconDB)

		s := NewLightClientStore(beaconDB, &p2pTesting.FakeP2P{}, new(event.Feed))
		require.NotNil(t, s)

		require.DeepSSZEqual(t, finalizedBlockRoot, s.nonFinalityCache.tail)

		for i := 0; i < 3; i++ {
			newBlock := util.NewBeaconBlock()
			newBlock.Block.Slot = primitives.Slot(32 + uint64(i))
			newBlock.Block.ParentRoot = finalizedBlockRoot[:]
			signedNewBlock, err := blocks.NewSignedBeaconBlock(newBlock)
			require.NoError(t, err)
			blockRoot, err := signedNewBlock.Block().HashTreeRoot()
			require.NoError(t, err)
			require.NoError(t, beaconDB.SaveBlock(ctx, signedNewBlock))
			finalizedBlockRoot = blockRoot
		}

		// Add a non-canonical item to the cache
		cacheItem := &lightClientCacheItem{
			period: 0,
			slot:   33,
		}
		nonCanonicalBlockRoot := [32]byte{1, 2, 3, 4}
		s.nonFinalityCache.items[nonCanonicalBlockRoot] = cacheItem

		require.Equal(t, 1, len(s.nonFinalityCache.items))

		err := s.MigrateToCold(ctx, finalizedBlockRoot)
		require.NoError(t, err)
		require.Equal(t, 0, len(s.nonFinalityCache.items), "Expected the non-canonical item in the cache to be deleted")
		require.DeepSSZEqual(t, finalizedBlockRoot, s.nonFinalityCache.tail)
	})

	// db has update - cache has both canonical and non-canonical items. finalized height is higher than all.
	// should update the update in db and delete cache.
	t.Run("mixed cache - finality higher than cache", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)
		ctx := context.Background()

		finalizedBlockRoot := saveInitialFinalizedCheckpointData(t, ctx, beaconDB)
		require.NoError(t, beaconDB.SaveHeadBlockRoot(ctx, finalizedBlockRoot))

		s := NewLightClientStore(beaconDB, &p2pTesting.FakeP2P{}, new(event.Feed))
		require.NotNil(t, s)

		require.DeepSSZEqual(t, finalizedBlockRoot, s.nonFinalityCache.tail)

		// Save an update for this period in db
		l := util.NewTestLightClient(t, version.Altair)
		update, err := NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		period := slots.SyncCommitteePeriod(slots.ToEpoch(l.AttestedBlock.Block().Slot()))
		require.NoError(t, beaconDB.SaveLightClientUpdate(ctx, period, update))

		lastBlockRoot := finalizedBlockRoot
		lastSlot := primitives.Slot(32)
		lastUpdate := update
		for i := 1; i < 4; i++ {
			l = util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(uint64(i)), util.WithSupermajority(0))
			require.NoError(t, beaconDB.SaveBlock(ctx, l.Block))
			require.NoError(t, s.SaveLCData(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock))
			root, err := l.Block.Block().HashTreeRoot()
			require.NoError(t, err)
			lastBlockRoot = root
			lastSlot = l.Block.Block().Slot()
			update, err = NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)
			lastUpdate = update
		}

		require.Equal(t, 3, len(s.nonFinalityCache.items))

		newBlock := util.NewBeaconBlock()
		newBlock.Block.Slot = lastSlot.Add(1)
		newBlock.Block.ParentRoot = lastBlockRoot[:]
		signedNewBlock, err := blocks.NewSignedBeaconBlock(newBlock)
		require.NoError(t, err)
		blockRoot, err := signedNewBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveBlock(ctx, signedNewBlock))
		finalizedBlockRoot = blockRoot

		// Add a non-canonical item to the cache
		cacheItem := &lightClientCacheItem{
			period: 0,
			slot:   33,
		}
		nonCanonicalBlockRoot := [32]byte{1, 2, 3, 4}
		s.nonFinalityCache.items[nonCanonicalBlockRoot] = cacheItem

		require.Equal(t, 4, len(s.nonFinalityCache.items))

		err = s.MigrateToCold(ctx, finalizedBlockRoot)
		require.NoError(t, err)
		require.Equal(t, 0, len(s.nonFinalityCache.items), "Expected the non-canonical item in the cache to be deleted")
		require.DeepSSZEqual(t, finalizedBlockRoot, s.nonFinalityCache.tail)
		u, err := beaconDB.LightClientUpdate(ctx, period)
		require.NoError(t, err)
		require.NotNil(t, u)
		require.DeepEqual(t, lastUpdate, u)
	})

	// db has update - cache has both canonical and non-canonical items. finalized height is in the middle.
	// should update the update in db and delete items in cache before finalized slot.
	t.Run("mixed cache - finality middle of cache", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)
		ctx := context.Background()

		finalizedBlockRoot := saveInitialFinalizedCheckpointData(t, ctx, beaconDB)
		require.NoError(t, beaconDB.SaveHeadBlockRoot(ctx, finalizedBlockRoot))

		s := NewLightClientStore(beaconDB, &p2pTesting.FakeP2P{}, new(event.Feed))
		require.NotNil(t, s)

		require.DeepSSZEqual(t, finalizedBlockRoot, s.nonFinalityCache.tail)

		// Save an update for this period in db
		l := util.NewTestLightClient(t, version.Altair)
		update, err := NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		period := slots.SyncCommitteePeriod(slots.ToEpoch(l.AttestedBlock.Block().Slot()))
		require.NoError(t, beaconDB.SaveLightClientUpdate(ctx, period, update))

		lastBlockRoot := finalizedBlockRoot
		lastSlot := primitives.Slot(32)
		lastUpdate := update
		lastAttestedRoot := [32]byte{}
		for i := 1; i < 4; i++ {
			l = util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(uint64(i)), util.WithSupermajority(uint64(i)), util.WithAttestedParentRoot(lastAttestedRoot))
			require.NoError(t, beaconDB.SaveBlock(ctx, l.Block))
			require.NoError(t, s.SaveLCData(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock))
			root, err := l.Block.Block().HashTreeRoot()
			require.NoError(t, err)
			lastBlockRoot = root
			lastSlot = l.Block.Block().Slot()
			update, err = NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)
			lastUpdate = update
			lastAttestedRoot, err = l.AttestedBlock.Block().HashTreeRoot()
			require.NoError(t, err)
		}

		require.Equal(t, 3, len(s.nonFinalityCache.items))

		newBlock := util.NewBeaconBlock()
		newBlock.Block.Slot = lastSlot.Add(1)
		newBlock.Block.ParentRoot = lastBlockRoot[:]
		signedNewBlock, err := blocks.NewSignedBeaconBlock(newBlock)
		require.NoError(t, err)
		blockRoot, err := signedNewBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveBlock(ctx, signedNewBlock))
		finalizedBlockRoot = blockRoot

		// Add a non-canonical item to the cache
		cacheItem := &lightClientCacheItem{
			period: 0,
			slot:   33,
		}
		nonCanonicalBlockRoot := [32]byte{1, 2, 3, 4}
		s.nonFinalityCache.items[nonCanonicalBlockRoot] = cacheItem

		require.Equal(t, 4, len(s.nonFinalityCache.items))

		for i := 4; i < 7; i++ {
			l = util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(uint64(i)), util.WithSupermajority(0), util.WithAttestedParentRoot(lastAttestedRoot))
			require.NoError(t, beaconDB.SaveBlock(ctx, l.Block))
			require.NoError(t, s.SaveLCData(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock))
			lastAttestedRoot, err = l.AttestedBlock.Block().HashTreeRoot()
			require.NoError(t, err)
		}

		require.Equal(t, 7, len(s.nonFinalityCache.items))

		err = s.MigrateToCold(ctx, finalizedBlockRoot)
		require.NoError(t, err)
		require.Equal(t, 3, len(s.nonFinalityCache.items), "Expected the non-canonical item in the cache to be deleted")
		require.DeepSSZEqual(t, finalizedBlockRoot, s.nonFinalityCache.tail)
		u, err := beaconDB.LightClientUpdate(ctx, period)
		require.NoError(t, err)
		require.NotNil(t, u)
		require.DeepEqual(t, lastUpdate, u)
	})

	// we have multiple periods in the cache before finalization happens. we expect all of them to be saved in db.
	t.Run("finality after multiple periods in cache", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)
		ctx := context.Background()

		cfg := params.BeaconConfig().Copy()
		cfg.EpochsPerSyncCommitteePeriod = 1
		params.OverrideBeaconConfig(cfg)

		finalizedBlockRoot := saveInitialFinalizedCheckpointData(t, ctx, beaconDB)
		require.NoError(t, beaconDB.SaveHeadBlockRoot(ctx, finalizedBlockRoot))

		s := NewLightClientStore(beaconDB, &p2pTesting.FakeP2P{}, new(event.Feed))
		require.NotNil(t, s)

		require.DeepSSZEqual(t, finalizedBlockRoot, s.nonFinalityCache.tail)

		// Save an update for this period1 in db
		l := util.NewTestLightClient(t, version.Altair)
		update, err := NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
		require.NoError(t, err)
		period1 := slots.SyncCommitteePeriod(slots.ToEpoch(l.AttestedBlock.Block().Slot()))
		require.NoError(t, beaconDB.SaveLightClientUpdate(ctx, period1, update))

		lastBlockRoot := finalizedBlockRoot
		lastSlot := primitives.Slot(32)
		lastUpdatePeriod1 := update
		lastAttestedRoot := [32]byte{}
		for i := 1; i < 4; i++ {
			l = util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(uint64(i)), util.WithSupermajority(uint64(i)), util.WithAttestedParentRoot(lastAttestedRoot))
			require.NoError(t, beaconDB.SaveBlock(ctx, l.Block))
			require.NoError(t, s.SaveLCData(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock))
			root, err := l.Block.Block().HashTreeRoot()
			require.NoError(t, err)
			lastBlockRoot = root
			lastSlot = l.Block.Block().Slot()
			lastUpdatePeriod1, err = NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)
			lastAttestedRoot, err = l.AttestedBlock.Block().HashTreeRoot()
			require.NoError(t, err)
		}

		period2 := period1
		var lastUpdatePeriod2 interfaces.LightClientUpdate
		for i := 1; i < 4; i++ {
			l = util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(uint64(i)+33), util.WithSupermajority(uint64(i)), util.WithAttestedParentRoot(lastAttestedRoot))
			require.NoError(t, beaconDB.SaveBlock(ctx, l.Block))
			require.NoError(t, s.SaveLCData(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock))
			root, err := l.Block.Block().HashTreeRoot()
			require.NoError(t, err)
			lastBlockRoot = root
			lastSlot = l.Block.Block().Slot()
			lastUpdatePeriod2, err = NewLightClientUpdateFromBeaconState(l.Ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock)
			require.NoError(t, err)
			lastAttestedRoot, err = l.AttestedBlock.Block().HashTreeRoot()
			require.NoError(t, err)
			period2 = slots.SyncCommitteePeriod(slots.ToEpoch(l.AttestedBlock.Block().Slot()))
		}

		require.Equal(t, 6, len(s.nonFinalityCache.items))

		newBlock := util.NewBeaconBlock()
		newBlock.Block.Slot = lastSlot.Add(1)
		newBlock.Block.ParentRoot = lastBlockRoot[:]
		signedNewBlock, err := blocks.NewSignedBeaconBlock(newBlock)
		require.NoError(t, err)
		blockRoot, err := signedNewBlock.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, beaconDB.SaveBlock(ctx, signedNewBlock))
		finalizedBlockRoot = blockRoot

		// Add a non-canonical item to the cache
		cacheItem := &lightClientCacheItem{
			period: 0,
			slot:   33,
		}
		nonCanonicalBlockRoot := [32]byte{1, 2, 3, 4}
		s.nonFinalityCache.items[nonCanonicalBlockRoot] = cacheItem

		require.Equal(t, 7, len(s.nonFinalityCache.items))

		for i := 4; i < 7; i++ {
			l = util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(uint64(i)+33), util.WithSupermajority(0), util.WithAttestedParentRoot(lastAttestedRoot))
			require.NoError(t, beaconDB.SaveBlock(ctx, l.Block))
			require.NoError(t, s.SaveLCData(ctx, l.State, l.Block, l.AttestedState, l.AttestedBlock, l.FinalizedBlock))
			lastAttestedRoot, err = l.AttestedBlock.Block().HashTreeRoot()
			require.NoError(t, err)
		}

		require.Equal(t, 10, len(s.nonFinalityCache.items))

		err = s.MigrateToCold(ctx, finalizedBlockRoot)
		require.NoError(t, err)
		require.Equal(t, 3, len(s.nonFinalityCache.items), "Expected the non-canonical item in the cache to be deleted")
		require.DeepSSZEqual(t, finalizedBlockRoot, s.nonFinalityCache.tail)
		u, err := beaconDB.LightClientUpdate(ctx, period2)
		require.NoError(t, err)
		require.NotNil(t, u)
		require.DeepEqual(t, lastUpdatePeriod2, u)
		u, err = beaconDB.LightClientUpdate(ctx, period1)
		require.NoError(t, err)
		require.NotNil(t, u)
		require.DeepEqual(t, lastUpdatePeriod1, u)
	})
}

func saveInitialFinalizedCheckpointData(t *testing.T, ctx context.Context, beaconDB db.Database) [32]byte {
	genesis := util.NewBeaconBlock()
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))
	util.SaveBlock(t, ctx, beaconDB, genesis)
	genesisState, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveState(ctx, genesisState, genesisRoot))

	finalizedState, err := util.NewBeaconState()
	require.NoError(t, err)
	finalizedBlock := util.NewBeaconBlock()
	finalizedBlock.Block.Slot = 32
	finalizedBlock.Block.ParentRoot = genesisRoot[:]
	signedFinalizedBlock, err := blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(t, err)
	finalizedBlockHeader, err := signedFinalizedBlock.Header()
	require.NoError(t, err)
	require.NoError(t, finalizedState.SetLatestBlockHeader(finalizedBlockHeader.Header))
	finalizedStateRoot, err := finalizedState.HashTreeRoot(ctx)
	require.NoError(t, err)
	finalizedBlock.Block.StateRoot = finalizedStateRoot[:]
	signedFinalizedBlock, err = blocks.NewSignedBeaconBlock(finalizedBlock)
	require.NoError(t, err)
	finalizedBlockRoot, err := signedFinalizedBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	cp := ethpb.Checkpoint{
		Epoch: 1,
		Root:  finalizedBlockRoot[:],
	}
	require.NoError(t, beaconDB.SaveBlock(ctx, signedFinalizedBlock))
	require.NoError(t, beaconDB.SaveState(ctx, finalizedState, finalizedBlockRoot))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &cp))

	return finalizedBlockRoot
}
