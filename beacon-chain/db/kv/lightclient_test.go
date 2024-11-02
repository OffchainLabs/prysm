package kv

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	light_client "github.com/prysmaticlabs/prysm/v5/consensus-types/light-client"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"google.golang.org/protobuf/proto"
)

func createUpdate(t *testing.T, v int) (interfaces.LightClientUpdate, error) {
	config := params.BeaconConfig()
	var slot primitives.Slot
	var header interfaces.LightClientHeader
	var st state.BeaconState
	var err error

	sampleRoot := make([]byte, 32)
	for i := 0; i < 32; i++ {
		sampleRoot[i] = byte(i)
	}

	sampleExecutionBranch := make([][]byte, fieldparams.ExecutionBranchDepth)
	for i := 0; i < 4; i++ {
		sampleExecutionBranch[i] = make([]byte, 32)
		for j := 0; j < 32; j++ {
			sampleExecutionBranch[i][j] = byte(i + j)
		}
	}

	switch v {
	case version.Altair:
		slot = primitives.Slot(config.AltairForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
		})
		require.NoError(t, err)
		st, err = util.NewBeaconState()
		require.NoError(t, err)
	case version.Capella:
		slot = primitives.Slot(config.CapellaForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderCapella{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
			Execution: &enginev1.ExecutionPayloadHeaderCapella{
				ParentHash:       make([]byte, fieldparams.RootLength),
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: make([]byte, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
			},
			ExecutionBranch: sampleExecutionBranch,
		})
		require.NoError(t, err)
		st, err = util.NewBeaconStateCapella()
		require.NoError(t, err)
	case version.Deneb:
		slot = primitives.Slot(config.DenebForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderDeneb{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
			Execution: &enginev1.ExecutionPayloadHeaderDeneb{
				ParentHash:       make([]byte, fieldparams.RootLength),
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: make([]byte, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
			},
			ExecutionBranch: sampleExecutionBranch,
		})
		require.NoError(t, err)
		st, err = util.NewBeaconStateDeneb()
		require.NoError(t, err)
	case version.Electra:
		slot = primitives.Slot(config.ElectraForkEpoch * primitives.Epoch(config.SlotsPerEpoch)).Add(1)
		header, err = light_client.NewWrappedHeader(&pb.LightClientHeaderDeneb{
			Beacon: &pb.BeaconBlockHeader{
				Slot:          1,
				ProposerIndex: primitives.ValidatorIndex(rand.Int()),
				ParentRoot:    sampleRoot,
				StateRoot:     sampleRoot,
				BodyRoot:      sampleRoot,
			},
			Execution: &enginev1.ExecutionPayloadHeaderElectra{
				ParentHash:       make([]byte, fieldparams.RootLength),
				FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
				StateRoot:        make([]byte, fieldparams.RootLength),
				ReceiptsRoot:     make([]byte, fieldparams.RootLength),
				LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
				PrevRandao:       make([]byte, fieldparams.RootLength),
				ExtraData:        make([]byte, 0),
				BaseFeePerGas:    make([]byte, fieldparams.RootLength),
				BlockHash:        make([]byte, fieldparams.RootLength),
				TransactionsRoot: make([]byte, fieldparams.RootLength),
				WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
			},
			ExecutionBranch: sampleExecutionBranch,
		})
		require.NoError(t, err)
		st, err = util.NewBeaconStateElectra()
		require.NoError(t, err)
	default:
		return nil, fmt.Errorf("unsupported version %s", version.String(v))
	}

	update, err := createDefaultLightClientUpdate(slot, st)
	require.NoError(t, err)
	update.SetSignatureSlot(slot - 1)
	syncCommitteeBits := make([]byte, 64)
	syncCommitteeSignature := make([]byte, 96)
	update.SetSyncAggregate(&pb.SyncAggregate{
		SyncCommitteeBits:      syncCommitteeBits,
		SyncCommitteeSignature: syncCommitteeSignature,
	})

	require.NoError(t, update.SetAttestedHeader(header))
	require.NoError(t, update.SetFinalizedHeader(header))

	return update, nil
}

func TestStore_LightClientUpdate_CanSaveRetrieve(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.AltairForkEpoch = 0
	cfg.CapellaForkEpoch = 1
	cfg.DenebForkEpoch = 2
	cfg.ElectraForkEpoch = 3
	params.OverrideBeaconConfig(cfg)

	db := setupDB(t)
	ctx := context.Background()

	t.Run("Altair", func(t *testing.T) {
		update, err := createUpdate(t, version.Altair)
		require.NoError(t, err)
		period := uint64(1)

		err = db.SaveLightClientUpdate(ctx, period, update)
		require.NoError(t, err)

		retrievedUpdate, err := db.LightClientUpdate(ctx, period)
		require.NoError(t, err)
		require.DeepEqual(t, update, retrievedUpdate, "retrieved update does not match saved update")
	})
	t.Run("Capella", func(t *testing.T) {
		update, err := createUpdate(t, version.Capella)
		require.NoError(t, err)
		period := uint64(1)
		err = db.SaveLightClientUpdate(ctx, period, update)
		require.NoError(t, err)

		retrievedUpdate, err := db.LightClientUpdate(ctx, period)
		require.NoError(t, err)
		require.DeepEqual(t, update, retrievedUpdate, "retrieved update does not match saved update")
	})
	t.Run("Deneb", func(t *testing.T) {
		update, err := createUpdate(t, version.Deneb)
		require.NoError(t, err)
		period := uint64(1)
		err = db.SaveLightClientUpdate(ctx, period, update)
		require.NoError(t, err)

		retrievedUpdate, err := db.LightClientUpdate(ctx, period)
		require.NoError(t, err)
		require.DeepEqual(t, update, retrievedUpdate, "retrieved update does not match saved update")
	})
	t.Run("Electra", func(t *testing.T) {
		update, err := createUpdate(t, version.Electra)
		require.NoError(t, err)
		period := uint64(1)
		err = db.SaveLightClientUpdate(ctx, period, update)
		require.NoError(t, err)

		retrievedUpdate, err := db.LightClientUpdate(ctx, period)
		require.NoError(t, err)
		require.DeepEqual(t, update, retrievedUpdate, "retrieved update does not match saved update")
	})
}

func TestStore_LightClientUpdates_canRetrieveRange(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	updates := make([]interfaces.LightClientUpdate, 0, 3)
	for i := 1; i <= 3; i++ {
		update, err := createUpdate(t, version.Altair)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for i, update := range updates {
		err := db.SaveLightClientUpdate(ctx, uint64(i+1), update)
		require.NoError(t, err)
	}

	// Retrieve the updates
	retrievedUpdatesMap, err := db.LightClientUpdates(ctx, 1, 3)
	require.NoError(t, err)
	require.Equal(t, len(updates), len(retrievedUpdatesMap), "retrieved updates do not match saved updates")
	for i, update := range updates {
		require.DeepEqual(t, update, retrievedUpdatesMap[uint64(i+1)], "retrieved update does not match saved update")
	}

}

func TestStore_LightClientUpdate_EndPeriodSmallerThanStartPeriod(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	updates := make([]interfaces.LightClientUpdate, 0, 3)
	for i := 1; i <= 3; i++ {
		update, err := createUpdate(t, version.Altair)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for i, update := range updates {
		err := db.SaveLightClientUpdate(ctx, uint64(i+1), update)
		require.NoError(t, err)
	}

	// Retrieve the updates
	retrievedUpdates, err := db.LightClientUpdates(ctx, 3, 1)
	require.NotNil(t, err)
	require.Equal(t, err.Error(), "start period 3 is greater than end period 1")
	require.IsNil(t, retrievedUpdates)

}

func TestStore_LightClientUpdate_EndPeriodEqualToStartPeriod(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	updates := make([]interfaces.LightClientUpdate, 0, 3)
	for i := 1; i <= 3; i++ {
		update, err := createUpdate(t, version.Altair)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for i, update := range updates {
		err := db.SaveLightClientUpdate(ctx, uint64(i+1), update)
		require.NoError(t, err)
	}

	// Retrieve the updates
	retrievedUpdates, err := db.LightClientUpdates(ctx, 2, 2)
	require.NoError(t, err)
	require.Equal(t, 1, len(retrievedUpdates))
	require.DeepEqual(t, updates[1], retrievedUpdates[2], "retrieved update does not match saved update")
}

func TestStore_LightClientUpdate_StartPeriodBeforeFirstUpdate(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	updates := make([]interfaces.LightClientUpdate, 0, 3)
	for i := 1; i <= 3; i++ {
		update, err := createUpdate(t, version.Altair)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for i, update := range updates {
		err := db.SaveLightClientUpdate(ctx, uint64(i+1), update)
		require.NoError(t, err)
	}

	// Retrieve the updates
	retrievedUpdates, err := db.LightClientUpdates(ctx, 0, 4)
	require.NoError(t, err)
	require.Equal(t, 3, len(retrievedUpdates))
	for i, update := range updates {
		require.DeepEqual(t, update, retrievedUpdates[uint64(i+1)], "retrieved update does not match saved update")
	}
}

func TestStore_LightClientUpdate_EndPeriodAfterLastUpdate(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	updates := make([]interfaces.LightClientUpdate, 0, 3)
	for i := 1; i <= 3; i++ {
		update, err := createUpdate(t, version.Altair)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for i, update := range updates {
		err := db.SaveLightClientUpdate(ctx, uint64(i+1), update)
		require.NoError(t, err)
	}

	// Retrieve the updates
	retrievedUpdates, err := db.LightClientUpdates(ctx, 1, 6)
	require.NoError(t, err)
	require.Equal(t, 3, len(retrievedUpdates))
	for i, update := range updates {
		require.DeepEqual(t, update, retrievedUpdates[uint64(i+1)], "retrieved update does not match saved update")
	}
}

func TestStore_LightClientUpdate_PartialUpdates(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	updates := make([]interfaces.LightClientUpdate, 0, 3)
	for i := 1; i <= 3; i++ {
		update, err := createUpdate(t, version.Altair)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for i, update := range updates {
		err := db.SaveLightClientUpdate(ctx, uint64(i+1), update)
		require.NoError(t, err)
	}

	// Retrieve the updates
	retrievedUpdates, err := db.LightClientUpdates(ctx, 1, 2)
	require.NoError(t, err)
	require.Equal(t, 2, len(retrievedUpdates))
	for i, update := range updates[:2] {
		require.DeepEqual(t, update, retrievedUpdates[uint64(i+1)], "retrieved update does not match saved update")
	}
}

func TestStore_LightClientUpdate_MissingPeriods_SimpleData(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	updates := make([]interfaces.LightClientUpdate, 0, 4)
	for i := 1; i <= 4; i++ {
		update, err := createUpdate(t, version.Altair)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for i, update := range updates {
		if i == 1 || i == 2 {
			continue
		}
		err := db.SaveLightClientUpdate(ctx, uint64(i+1), update)
		require.NoError(t, err)
	}

	// Retrieve the updates
	retrievedUpdates, err := db.LightClientUpdates(ctx, 1, 4)
	require.NoError(t, err)
	require.Equal(t, 2, len(retrievedUpdates))
	require.DeepEqual(t, updates[0], retrievedUpdates[uint64(1)], "retrieved update does not match saved update")
	require.DeepEqual(t, updates[3], retrievedUpdates[uint64(4)], "retrieved update does not match saved update")

	// Retrieve the updates from the middle
	retrievedUpdates, err = db.LightClientUpdates(ctx, 2, 4)
	require.NoError(t, err)
	require.Equal(t, 1, len(retrievedUpdates))
	require.DeepEqual(t, updates[3], retrievedUpdates[4], "retrieved update does not match saved update")

	// Retrieve the updates from after the missing period
	retrievedUpdates, err = db.LightClientUpdates(ctx, 4, 4)
	require.NoError(t, err)
	require.Equal(t, 1, len(retrievedUpdates))
	require.DeepEqual(t, updates[3], retrievedUpdates[4], "retrieved update does not match saved update")

	//retrieve the updates from before the missing period to after the missing period
	retrievedUpdates, err = db.LightClientUpdates(ctx, 0, 6)
	require.NoError(t, err)
	require.Equal(t, 2, len(retrievedUpdates))
	require.DeepEqual(t, updates[0], retrievedUpdates[uint64(1)], "retrieved update does not match saved update")
	require.DeepEqual(t, updates[3], retrievedUpdates[uint64(4)], "retrieved update does not match saved update")
}

func TestStore_LightClientUpdate_EmptyDB(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	// Retrieve the updates
	retrievedUpdates, err := db.LightClientUpdates(ctx, 1, 3)
	require.IsNil(t, err)
	require.Equal(t, 0, len(retrievedUpdates))
}

func TestStore_LightClientUpdate_RetrieveMissingPeriodDistributed(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	updates := make([]interfaces.LightClientUpdate, 0, 5)
	for i := 1; i <= 5; i++ {
		update, err := createUpdate(t, version.Altair)
		require.NoError(t, err)
		updates = append(updates, update)
	}

	for i, update := range updates {
		if i == 1 || i == 3 {
			continue
		}
		err := db.SaveLightClientUpdate(ctx, uint64(i+1), update)
		require.NoError(t, err)
	}

	// Retrieve the updates
	retrievedUpdates, err := db.LightClientUpdates(ctx, 0, 7)
	require.NoError(t, err)
	require.Equal(t, 3, len(retrievedUpdates))
	require.DeepEqual(t, updates[0], retrievedUpdates[uint64(1)], "retrieved update does not match saved update")
	require.DeepEqual(t, updates[2], retrievedUpdates[uint64(3)], "retrieved update does not match saved update")
	require.DeepEqual(t, updates[4], retrievedUpdates[uint64(5)], "retrieved update does not match saved update")
}

func createDefaultLightClientUpdate(currentSlot primitives.Slot, attestedState state.BeaconState) (interfaces.LightClientUpdate, error) {
	currentEpoch := slots.ToEpoch(currentSlot)

	syncCommitteeSize := params.BeaconConfig().SyncCommitteeSize
	pubKeys := make([][]byte, syncCommitteeSize)
	for i := uint64(0); i < syncCommitteeSize; i++ {
		pubKeys[i] = make([]byte, fieldparams.BLSPubkeyLength)
	}
	nextSyncCommittee := &pb.SyncCommittee{
		Pubkeys:         pubKeys,
		AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength),
	}

	var nextSyncCommitteeBranch [][]byte
	if currentEpoch >= params.BeaconConfig().ElectraForkEpoch {
		nextSyncCommitteeBranch = make([][]byte, fieldparams.SyncCommitteeBranchDepthElectra)
	} else {
		nextSyncCommitteeBranch = make([][]byte, fieldparams.SyncCommitteeBranchDepth)
	}
	for i := 0; i < len(nextSyncCommitteeBranch); i++ {
		nextSyncCommitteeBranch[i] = make([]byte, fieldparams.RootLength)
	}

	executionBranch := make([][]byte, fieldparams.ExecutionBranchDepth)
	for i := 0; i < fieldparams.ExecutionBranchDepth; i++ {
		executionBranch[i] = make([]byte, 32)
	}

	var finalityBranch [][]byte
	if attestedState.Version() >= version.Electra {
		finalityBranch = make([][]byte, fieldparams.FinalityBranchDepthElectra)
	} else {
		finalityBranch = make([][]byte, fieldparams.FinalityBranchDepth)
	}
	for i := 0; i < len(finalityBranch); i++ {
		finalityBranch[i] = make([]byte, 32)
	}

	var m proto.Message
	if currentEpoch < params.BeaconConfig().CapellaForkEpoch {
		m = &pb.LightClientUpdateAltair{
			AttestedHeader:          &pb.LightClientHeaderAltair{},
			NextSyncCommittee:       nextSyncCommittee,
			NextSyncCommitteeBranch: nextSyncCommitteeBranch,
			FinalityBranch:          finalityBranch,
		}
	} else if currentEpoch < params.BeaconConfig().DenebForkEpoch {
		m = &pb.LightClientUpdateCapella{
			AttestedHeader: &pb.LightClientHeaderCapella{
				Beacon:          &pb.BeaconBlockHeader{},
				Execution:       &enginev1.ExecutionPayloadHeaderCapella{},
				ExecutionBranch: executionBranch,
			},
			NextSyncCommittee:       nextSyncCommittee,
			NextSyncCommitteeBranch: nextSyncCommitteeBranch,
			FinalityBranch:          finalityBranch,
		}
	} else if currentEpoch < params.BeaconConfig().ElectraForkEpoch {
		m = &pb.LightClientUpdateDeneb{
			AttestedHeader: &pb.LightClientHeaderDeneb{
				Beacon:          &pb.BeaconBlockHeader{},
				Execution:       &enginev1.ExecutionPayloadHeaderDeneb{},
				ExecutionBranch: executionBranch,
			},
			NextSyncCommittee:       nextSyncCommittee,
			NextSyncCommitteeBranch: nextSyncCommitteeBranch,
			FinalityBranch:          finalityBranch,
		}
	} else {
		if attestedState.Version() >= version.Electra {
			m = &pb.LightClientUpdateElectra{
				AttestedHeader: &pb.LightClientHeaderDeneb{
					Beacon:          &pb.BeaconBlockHeader{},
					Execution:       &enginev1.ExecutionPayloadHeaderDeneb{},
					ExecutionBranch: executionBranch,
				},
				NextSyncCommittee:       nextSyncCommittee,
				NextSyncCommitteeBranch: nextSyncCommitteeBranch,
				FinalityBranch:          finalityBranch,
			}
		} else {
			m = &pb.LightClientUpdateDeneb{
				AttestedHeader: &pb.LightClientHeaderDeneb{
					Beacon:          &pb.BeaconBlockHeader{},
					Execution:       &enginev1.ExecutionPayloadHeaderDeneb{},
					ExecutionBranch: executionBranch,
				},
				NextSyncCommittee:       nextSyncCommittee,
				NextSyncCommitteeBranch: nextSyncCommitteeBranch,
				FinalityBranch:          finalityBranch,
			}
		}
	}

	return light_client.NewWrappedUpdate(m)
}
