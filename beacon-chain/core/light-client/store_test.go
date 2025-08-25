package light_client_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v6/async/event"
	lightClient "github.com/OffchainLabs/prysm/v6/beacon-chain/core/light-client"
	testDB "github.com/OffchainLabs/prysm/v6/beacon-chain/db/testing"
	p2pTesting "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/OffchainLabs/prysm/v6/testing/require"
	"github.com/OffchainLabs/prysm/v6/testing/util"
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
	lcStore := lightClient.NewLightClientStore(testDB.SetupDB(t), &p2pTesting.FakeP2P{}, new(event.Feed))

	// Create test light client updates for Capella and Deneb
	lCapella := util.NewTestLightClient(t, version.Capella)
	opUpdateCapella, err := lightClient.NewLightClientOptimisticUpdateFromBeaconState(lCapella.Ctx, lCapella.State, lCapella.Block, lCapella.AttestedState, lCapella.AttestedBlock)
	require.NoError(t, err)
	require.NotNil(t, opUpdateCapella, "OptimisticUpdateCapella is nil")
	finUpdateCapella, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(lCapella.Ctx, lCapella.State, lCapella.Block, lCapella.AttestedState, lCapella.AttestedBlock, lCapella.FinalizedBlock)
	require.NoError(t, err)
	require.NotNil(t, finUpdateCapella, "FinalityUpdateCapella is nil")

	lDeneb := util.NewTestLightClient(t, version.Deneb)
	opUpdateDeneb, err := lightClient.NewLightClientOptimisticUpdateFromBeaconState(lDeneb.Ctx, lDeneb.State, lDeneb.Block, lDeneb.AttestedState, lDeneb.AttestedBlock)
	require.NoError(t, err)
	require.NotNil(t, opUpdateDeneb, "OptimisticUpdateDeneb is nil")
	finUpdateDeneb, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(lDeneb.Ctx, lDeneb.State, lDeneb.Block, lDeneb.AttestedState, lDeneb.AttestedBlock, lDeneb.FinalizedBlock)
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
	lcStore := lightClient.NewLightClientStore(testDB.SetupDB(t), p2p, new(event.Feed))

	// update 0 with basic data and no supermajority following an empty lastFinalityUpdate - should save and broadcast
	l0 := util.NewTestLightClient(t, version.Altair)
	update0, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l0.Ctx, l0.State, l0.Block, l0.AttestedState, l0.AttestedBlock, l0.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, lightClient.IsBetterFinalityUpdate(update0, lcStore.LastFinalityUpdate()), "update0 should be better than nil")
	// update0 should be valid for broadcast - meaning it should be broadcasted
	require.Equal(t, true, lightClient.IsFinalityUpdateValidForBroadcast(update0, lcStore.LastFinalityUpdate()), "update0 should be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update0, true)
	require.Equal(t, update0, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, true, p2p.BroadcastCalled.Load(), "Broadcast should have been called after setting a new last finality update when previous is nil")
	p2p.BroadcastCalled.Store(false) // Reset for next test

	// update 1 with same finality slot, increased attested slot, and no supermajority - should save but not broadcast
	l1 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(1))
	update1, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l1.Ctx, l1.State, l1.Block, l1.AttestedState, l1.AttestedBlock, l1.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, lightClient.IsBetterFinalityUpdate(update1, update0), "update1 should be better than update0")
	// update1 should not be valid for broadcast - meaning it should not be broadcasted
	require.Equal(t, false, lightClient.IsFinalityUpdateValidForBroadcast(update1, lcStore.LastFinalityUpdate()), "update1 should not be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update1, true)
	require.Equal(t, update1, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, false, p2p.BroadcastCalled.Load(), "Broadcast should not have been called after setting a new last finality update without supermajority")
	p2p.BroadcastCalled.Store(false) // Reset for next test

	// update 2 with same finality slot, increased attested slot, and supermajority - should save and broadcast
	l2 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(2), util.WithSupermajority())
	update2, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l2.Ctx, l2.State, l2.Block, l2.AttestedState, l2.AttestedBlock, l2.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, lightClient.IsBetterFinalityUpdate(update2, update1), "update2 should be better than update1")
	// update2 should be valid for broadcast - meaning it should be broadcasted
	require.Equal(t, true, lightClient.IsFinalityUpdateValidForBroadcast(update2, lcStore.LastFinalityUpdate()), "update2 should be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update2, true)
	require.Equal(t, update2, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, true, p2p.BroadcastCalled.Load(), "Broadcast should have been called after setting a new last finality update with supermajority")
	p2p.BroadcastCalled.Store(false) // Reset for next test

	// update 3 with same finality slot, increased attested slot, and supermajority - should save but not broadcast
	l3 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedAttestedSlot(3), util.WithSupermajority())
	update3, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l3.Ctx, l3.State, l3.Block, l3.AttestedState, l3.AttestedBlock, l3.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, lightClient.IsBetterFinalityUpdate(update3, update2), "update3 should be better than update2")
	// update3 should not be valid for broadcast - meaning it should not be broadcasted
	require.Equal(t, false, lightClient.IsFinalityUpdateValidForBroadcast(update3, lcStore.LastFinalityUpdate()), "update3 should not be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update3, true)
	require.Equal(t, update3, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, false, p2p.BroadcastCalled.Load(), "Broadcast should not have been when previous was already broadcast")

	// update 4 with increased finality slot, increased attested slot, and supermajority - should save and broadcast
	l4 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedFinalizedSlot(1), util.WithIncreasedAttestedSlot(1), util.WithSupermajority())
	update4, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l4.Ctx, l4.State, l4.Block, l4.AttestedState, l4.AttestedBlock, l4.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, lightClient.IsBetterFinalityUpdate(update4, update3), "update4 should be better than update3")
	// update4 should be valid for broadcast - meaning it should be broadcasted
	require.Equal(t, true, lightClient.IsFinalityUpdateValidForBroadcast(update4, lcStore.LastFinalityUpdate()), "update4 should be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update4, true)
	require.Equal(t, update4, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, true, p2p.BroadcastCalled.Load(), "Broadcast should have been called after a new finality update with increased finality slot")
	p2p.BroadcastCalled.Store(false) // Reset for next test

	// update 5 with the same new finality slot, increased attested slot, and supermajority - should save but not broadcast
	l5 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedFinalizedSlot(1), util.WithIncreasedAttestedSlot(2), util.WithSupermajority())
	update5, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l5.Ctx, l5.State, l5.Block, l5.AttestedState, l5.AttestedBlock, l5.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, lightClient.IsBetterFinalityUpdate(update5, update4), "update5 should be better than update4")
	// update5 should not be valid for broadcast - meaning it should not be broadcasted
	require.Equal(t, false, lightClient.IsFinalityUpdateValidForBroadcast(update5, lcStore.LastFinalityUpdate()), "update5 should not be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update5, true)
	require.Equal(t, update5, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, false, p2p.BroadcastCalled.Load(), "Broadcast should not have been called when previous was already broadcast with supermajority")

	// update 6 with the same new finality slot, increased attested slot, and no supermajority - should save but not broadcast
	l6 := util.NewTestLightClient(t, version.Altair, util.WithIncreasedFinalizedSlot(1), util.WithIncreasedAttestedSlot(3))
	update6, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(l6.Ctx, l6.State, l6.Block, l6.AttestedState, l6.AttestedBlock, l6.FinalizedBlock)
	require.NoError(t, err, "Failed to create light client finality update")

	require.Equal(t, true, lightClient.IsBetterFinalityUpdate(update6, update5), "update6 should be better than update5")
	// update6 should not be valid for broadcast - meaning it should not be broadcasted
	require.Equal(t, false, lightClient.IsFinalityUpdateValidForBroadcast(update6, lcStore.LastFinalityUpdate()), "update6 should not be valid for broadcast")

	lcStore.SetLastFinalityUpdate(update6, true)
	require.Equal(t, update6, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should match the set value")
	require.Equal(t, false, p2p.BroadcastCalled.Load(), "Broadcast should not have been called when previous was already broadcast with supermajority")
}
