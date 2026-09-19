package eth_test

import (
	"fmt"
	"math/bits"
	"reflect"
	"testing"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestLightClientBackfillBranchDepths checks the branch lengths written in light_client_backfill.proto.
// A Merkle branch for a generalized index holds floorlog2(gindex) nodes. The SSZ code is generated
// from the ssz-size tag, which is "<vector length>,<element size>".
func TestLightClientBackfillBranchDepths(t *testing.T) {
	// Generalized indices of the proven fields. The suffix names the fork that changed the index.
	const (
		// BeaconBlockBody.sync_aggregate
		syncAggregateGindex      = 24
		syncAggregateGindexGloas = 355
		// BeaconState.current_sync_committee
		currentSyncCommitteeGindex        = 54
		currentSyncCommitteeGindexElectra = 86
		currentSyncCommitteeGindexGloas   = 2945
		// BeaconState.finalized_checkpoint.root
		finalizedRootGindex        = 105
		finalizedRootGindexElectra = 169
		finalizedRootGindexGloas   = 735
		// BeaconBlockBody.execution_payload
		executionPayloadGindex = 25
		// BeaconBlockBody.signed_execution_payload_bid.message.parent_block_hash
		executionBlockHashGindexGloas = 2856
	)
	tests := []struct {
		container any
		branch    string
		gindex    uint64
	}{
		{&ethpb.LightClientBlockData{}, "SyncAggregateBranch", syncAggregateGindex},
		{&ethpb.LightClientBlockDataGloas{}, "SyncAggregateBranch", syncAggregateGindexGloas},
		{&ethpb.LightClientBootstrapDataAltair{}, "CurrentSyncCommitteeBranch", currentSyncCommitteeGindex},
		{&ethpb.LightClientBootstrapDataCapella{}, "CurrentSyncCommitteeBranch", currentSyncCommitteeGindex},
		{&ethpb.LightClientBootstrapDataCapella{}, "ExecutionBranch", executionPayloadGindex},
		{&ethpb.LightClientBootstrapDataDeneb{}, "CurrentSyncCommitteeBranch", currentSyncCommitteeGindex},
		{&ethpb.LightClientBootstrapDataDeneb{}, "ExecutionBranch", executionPayloadGindex},
		{&ethpb.LightClientBootstrapDataElectra{}, "CurrentSyncCommitteeBranch", currentSyncCommitteeGindexElectra},
		{&ethpb.LightClientBootstrapDataElectra{}, "ExecutionBranch", executionPayloadGindex},
		{&ethpb.LightClientBootstrapDataGloas{}, "CurrentSyncCommitteeBranch", currentSyncCommitteeGindexGloas},
		{&ethpb.LightClientBootstrapDataGloas{}, "ExecutionBranch", executionBlockHashGindexGloas},
		{&ethpb.LightClientEpochDataAltair{}, "FinalityBranch", finalizedRootGindex},
		{&ethpb.LightClientEpochDataCapella{}, "FinalityBranch", finalizedRootGindex},
		{&ethpb.LightClientEpochDataDeneb{}, "FinalityBranch", finalizedRootGindex},
		{&ethpb.LightClientEpochDataElectra{}, "FinalityBranch", finalizedRootGindexElectra},
		{&ethpb.LightClientEpochDataGloas{}, "FinalityBranch", finalizedRootGindexGloas},
	}
	for _, tt := range tests {
		typ := reflect.TypeOf(tt.container).Elem()
		t.Run(typ.Name()+"."+tt.branch, func(t *testing.T) {
			field, ok := typ.FieldByName(tt.branch)
			require.Equal(t, true, ok)
			depth := bits.Len64(tt.gindex) - 1 // floorlog2(gindex)
			require.Equal(t, fmt.Sprintf("%d,32", depth), field.Tag.Get("ssz-size"))
		})
	}
}
