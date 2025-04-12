package light_client_test

import (
	"testing"

	lightClient "github.com/OffchainLabs/prysm/v6/beacon-chain/core/light-client"
	"github.com/OffchainLabs/prysm/v6/config/params"
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
	lcStore := &lightClient.Store{}

	// Create test light client updates for Capella and Deneb
	lCapella := util.NewTestLightClient(t).SetupTestCapella(false, 0, true)
	opUpdateCapella, err := lightClient.NewLightClientOptimisticUpdateFromBeaconState(lCapella.Ctx, lCapella.State.Slot(), lCapella.State, lCapella.Block, lCapella.AttestedState, lCapella.AttestedBlock)
	require.NoError(t, err)
	require.NotNil(t, opUpdateCapella, "OptimisticUpdateCapella is nil")
	finUpdateCapella, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(lCapella.Ctx, lCapella.State.Slot(), lCapella.State, lCapella.Block, lCapella.AttestedState, lCapella.AttestedBlock, lCapella.FinalizedBlock)
	require.NoError(t, err)
	require.NotNil(t, finUpdateCapella, "FinalityUpdateCapella is nil")

	lDeneb := util.NewTestLightClient(t).SetupTestDeneb(false, 0, true)
	opUpdateDeneb, err := lightClient.NewLightClientOptimisticUpdateFromBeaconState(lDeneb.Ctx, lDeneb.State.Slot(), lDeneb.State, lDeneb.Block, lDeneb.AttestedState, lDeneb.AttestedBlock)
	require.NoError(t, err)
	require.NotNil(t, opUpdateDeneb, "OptimisticUpdateDeneb is nil")
	finUpdateDeneb, err := lightClient.NewLightClientFinalityUpdateFromBeaconState(lDeneb.Ctx, lDeneb.State.Slot(), lDeneb.State, lDeneb.Block, lDeneb.AttestedState, lDeneb.AttestedBlock, lDeneb.FinalizedBlock)
	require.NoError(t, err)
	require.NotNil(t, finUpdateDeneb, "FinalityUpdateDeneb is nil")

	// Initially the store should have nil values for both updates
	require.IsNil(t, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should be nil")
	require.IsNil(t, lcStore.LastOptimisticUpdate(), "lastOptimisticUpdate should be nil")

	// Set and get finality with Capella update. Optimistic update should be nil
	lcStore.SetLastFinalityUpdate(finUpdateCapella)
	require.Equal(t, finUpdateCapella, lcStore.LastFinalityUpdate(), "lastFinalityUpdate is wrong")
	require.IsNil(t, lcStore.LastOptimisticUpdate(), "lastOptimisticUpdate should be nil")

	// Set and get optimistic with Capella update. Finality update should be Capella
	lcStore.SetLastOptimisticUpdate(opUpdateCapella)
	require.Equal(t, opUpdateCapella, lcStore.LastOptimisticUpdate(), "lastOptimisticUpdate is wrong")
	require.Equal(t, finUpdateCapella, lcStore.LastFinalityUpdate(), "lastFinalityUpdate is wrong")

	// Set and get finality and optimistic with Deneb update
	lcStore.SetLastFinalityUpdate(finUpdateDeneb)
	lcStore.SetLastOptimisticUpdate(opUpdateDeneb)
	require.Equal(t, finUpdateDeneb, lcStore.LastFinalityUpdate(), "lastFinalityUpdate is wrong")
	require.Equal(t, opUpdateDeneb, lcStore.LastOptimisticUpdate(), "lastOptimisticUpdate is wrong")

	// Set and get finality and optimistic with nil update
	lcStore.SetLastFinalityUpdate(nil)
	lcStore.SetLastOptimisticUpdate(nil)
	require.IsNil(t, lcStore.LastFinalityUpdate(), "lastFinalityUpdate should be nil")
	require.IsNil(t, lcStore.LastOptimisticUpdate(), "lastOptimisticUpdate should be nil")
}
