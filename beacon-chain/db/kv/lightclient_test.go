package kv

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
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
	if attestedState.Version() >= version.Electra {
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

func TestStore_LightClientBootstrap_CanSaveRetrieve(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	cfg.AltairForkEpoch = 0
	cfg.CapellaForkEpoch = 1
	cfg.DenebForkEpoch = 2
	cfg.ElectraForkEpoch = 3
	cfg.EpochsPerSyncCommitteePeriod = 1
	params.OverrideBeaconConfig(cfg)

	db := setupDB(t)
	ctx := context.Background()

	t.Run("Nil", func(t *testing.T) {
		retrievedBootstrap, err := db.LightClientBootstrap(ctx, []byte("NilBlockRoot"))
		require.NoError(t, err)
		require.IsNil(t, retrievedBootstrap)
	})

	t.Run("Altair", func(t *testing.T) {
		bootstrap, err := createDefaultLightClientBootstrap(primitives.Slot(uint64(params.BeaconConfig().AltairForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)))
		require.NoError(t, err)

		err = bootstrap.SetCurrentSyncCommittee(createSampleSyncCommittee())
		require.NoError(t, err)

		err = db.SaveLightClientBootstrap(ctx, []byte("blockRootAltair"), bootstrap)
		require.NoError(t, err)

		retrievedBootstrap, err := db.LightClientBootstrap(ctx, []byte("blockRootAltair"))
		require.NoError(t, err)
		require.DeepEqual(t, bootstrap.Header(), retrievedBootstrap.Header(), "retrieved bootstrap header does not match saved bootstrap header")
		require.DeepEqual(t, bootstrap.CurrentSyncCommittee(), retrievedBootstrap.CurrentSyncCommittee(), "retrieved bootstrap sync committee does not match saved bootstrap sync committee")
		savedBranch, err := bootstrap.CurrentSyncCommitteeBranch()
		require.NoError(t, err)
		retrievedBranch, err := retrievedBootstrap.CurrentSyncCommitteeBranch()
		require.NoError(t, err)
		require.DeepEqual(t, savedBranch, retrievedBranch, "retrieved bootstrap sync committee branch does not match saved bootstrap sync committee branch")

	})

	t.Run("Capella", func(t *testing.T) {
		bootstrap, err := createDefaultLightClientBootstrap(primitives.Slot(uint64(params.BeaconConfig().CapellaForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)))
		require.NoError(t, err)

		err = bootstrap.SetCurrentSyncCommittee(createSampleSyncCommittee())
		require.NoError(t, err)

		err = db.SaveLightClientBootstrap(ctx, []byte("blockRootCapella"), bootstrap)
		require.NoError(t, err)

		retrievedBootstrap, err := db.LightClientBootstrap(ctx, []byte("blockRootCapella"))
		require.NoError(t, err)
		require.DeepEqual(t, bootstrap, retrievedBootstrap, "retrieved bootstrap does not match saved bootstrap")
	})

	t.Run("Deneb", func(t *testing.T) {
		bootstrap, err := createDefaultLightClientBootstrap(primitives.Slot(uint64(params.BeaconConfig().DenebForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)))
		require.NoError(t, err)

		err = bootstrap.SetCurrentSyncCommittee(createRandomSyncCommittee())
		require.NoError(t, err)

		err = db.SaveLightClientBootstrap(ctx, []byte("blockRootDeneb"), bootstrap)
		require.NoError(t, err)

		retrievedBootstrap, err := db.LightClientBootstrap(ctx, []byte("blockRootDeneb"))
		require.NoError(t, err)
		require.DeepEqual(t, bootstrap, retrievedBootstrap, "retrieved bootstrap does not match saved bootstrap")
	})

	t.Run("Electra", func(t *testing.T) {
		bootstrap, err := createDefaultLightClientBootstrap(primitives.Slot(uint64(params.BeaconConfig().ElectraForkEpoch) * uint64(params.BeaconConfig().SlotsPerEpoch)))
		require.NoError(t, err)

		err = bootstrap.SetCurrentSyncCommittee(createRandomSyncCommittee())
		require.NoError(t, err)

		err = db.SaveLightClientBootstrap(ctx, []byte("blockRootElectra"), bootstrap)
		require.NoError(t, err)

		retrievedBootstrap, err := db.LightClientBootstrap(ctx, []byte("blockRootElectra"))
		require.NoError(t, err)
		require.DeepEqual(t, bootstrap, retrievedBootstrap, "retrieved bootstrap does not match saved bootstrap")
	})
}

func createDefaultLightClientBootstrap(currentSlot primitives.Slot) (interfaces.LightClientBootstrap, error) {
	currentEpoch := slots.ToEpoch(currentSlot)
	syncCommitteeSize := params.BeaconConfig().SyncCommitteeSize
	pubKeys := make([][]byte, syncCommitteeSize)
	for i := uint64(0); i < syncCommitteeSize; i++ {
		pubKeys[i] = make([]byte, fieldparams.BLSPubkeyLength)
	}
	currentSyncCommittee := &pb.SyncCommittee{
		Pubkeys:         pubKeys,
		AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength),
	}

	var currentSyncCommitteeBranch [][]byte
	if currentEpoch >= params.BeaconConfig().ElectraForkEpoch {
		currentSyncCommitteeBranch = make([][]byte, fieldparams.SyncCommitteeBranchDepthElectra)
	} else {
		currentSyncCommitteeBranch = make([][]byte, fieldparams.SyncCommitteeBranchDepth)
	}
	for i := 0; i < len(currentSyncCommitteeBranch); i++ {
		currentSyncCommitteeBranch[i] = make([]byte, fieldparams.RootLength)
	}

	executionBranch := make([][]byte, fieldparams.ExecutionBranchDepth)
	for i := 0; i < fieldparams.ExecutionBranchDepth; i++ {
		executionBranch[i] = make([]byte, 32)
	}

	// TODO: can this be based on the current epoch?
	var m proto.Message
	if currentEpoch < params.BeaconConfig().CapellaForkEpoch {
		m = &pb.LightClientBootstrapAltair{
			Header: &pb.LightClientHeaderAltair{
				Beacon: &pb.BeaconBlockHeader{
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
				},
			},
			CurrentSyncCommittee:       currentSyncCommittee,
			CurrentSyncCommitteeBranch: currentSyncCommitteeBranch,
		}
	} else if currentEpoch < params.BeaconConfig().DenebForkEpoch {
		m = &pb.LightClientBootstrapCapella{
			Header: &pb.LightClientHeaderCapella{
				Beacon: &pb.BeaconBlockHeader{
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
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
				ExecutionBranch: executionBranch,
			},
			CurrentSyncCommittee:       currentSyncCommittee,
			CurrentSyncCommitteeBranch: currentSyncCommitteeBranch,
		}
	} else if currentEpoch < params.BeaconConfig().ElectraForkEpoch {
		m = &pb.LightClientBootstrapDeneb{
			Header: &pb.LightClientHeaderDeneb{
				Beacon: &pb.BeaconBlockHeader{
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
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
					GasLimit:         0,
					GasUsed:          0,
				},
				ExecutionBranch: executionBranch,
			},
			CurrentSyncCommittee:       currentSyncCommittee,
			CurrentSyncCommitteeBranch: currentSyncCommitteeBranch,
		}
	} else {
		m = &pb.LightClientBootstrapElectra{
			Header: &pb.LightClientHeaderDeneb{
				Beacon: &pb.BeaconBlockHeader{
					ParentRoot: make([]byte, 32),
					StateRoot:  make([]byte, 32),
					BodyRoot:   make([]byte, 32),
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
					GasLimit:         0,
					GasUsed:          0,
				},
				ExecutionBranch: executionBranch,
			},
			CurrentSyncCommittee:       currentSyncCommittee,
			CurrentSyncCommitteeBranch: currentSyncCommitteeBranch,
		}
	}

	return light_client.NewWrappedBootstrap(m)
}

func createRandomSyncCommittee() *pb.SyncCommittee {
	// random number between 2 and 128
	base := rand.Int()%127 + 2

	syncCom := make([][]byte, params.BeaconConfig().SyncCommitteeSize)
	for i := 0; uint64(i) < params.BeaconConfig().SyncCommitteeSize; i++ {
		if i%base == 0 {
			syncCom[i] = make([]byte, fieldparams.BLSPubkeyLength)
			syncCom[i][0] = 1
			continue
		}
		syncCom[i] = make([]byte, fieldparams.BLSPubkeyLength)
	}

	return &pb.SyncCommittee{
		Pubkeys:         syncCom,
		AggregatePubkey: make([]byte, fieldparams.BLSPubkeyLength),
	}
}

func createSampleSyncCommittee() *pb.SyncCommittee {
	listOfPubkeys := []string{
		"0xa47f6d0bb2b1b2a54080f6c304341b98e0556c1292416acea0aa59bb69e73cab9527f0d478c494672e744c7d2201599c",
		"0x9797f29a5f9fff8ef6066cc74b2f37cb655944334aec87d328c69428997e46da04dd9136e5f2673c60c6aa062db39b5a",
		"0xab2c736c9320b863918d8c6a465e1d33245961cf2a6ed06e9c5bfb582d4640f0225591043ef5f5951a36a58f3024c887",
		"0x8d6fed9b5e2955ba82d1b4410f73cda257a4c85805e133e952ca862181c62d5b7ac7ace8a16d64fa7defc150ce698d54",
		"0x8d91e3411aa20a313fa264031a5356b98f32d92b83e097a61827b1e187d4920f0dee6104fe086b6d68895103dd284aee",
		"0x89e3d4ec1951f37a60f178f1e20de4941c3079eaa9c8531c74444badf12cae3880ba08083c2dcc9bacf40f2d063750b0",
		"0xb315959d239c19659c8ad311e4b8d6560d7f3989a0fc05d48c0d3fbde03d7fe8b6c5989e2f7305363df9b48ef87b7c37",
		"0x9975d25d9c7a23fafb1be8f1af99724ddf6d624ef55a50862a2b3648a337bb3997895d45c57cde7054d396a906f998ce",
		"0x8664931ae219216c66b9edd56d27a3730d1f5aa9e135ef72a236ab768fa2165ecd4017ebfddfad85bc5744977a29d918",
		"0xa5ec128371d8ccdf6e12ddb193ea321ff47f11e338d8480e1304d6608b8d5b323df1e85c59a7347cc50510fb091217b4",
		"0xb4cb6012de9004a9712dd668b14b8f62ce98d30d0d9d2f8252b610bb349473663063878b9ebef1eab80ab7a73467ad7a",
		"0x96ef72f01fc182b7499e768da0303c6e57228b3f3e23e2a14927aa747ccccbe82163cc49498c4fd896fad0e3f60fdc72",
		"0xb0b8d2c9ddf1ee5024059932e453ba027f69d2bf102372a3f8eded4fb586a8e9290b2637c5fa31bea3d04c171e340775",
		"0x8d6fa177b17defe606419ddfb11f3ac8c0807c1a8d5328d463b66f7d335f6d4fbe4d441ff9f2c3421d7048c61d81a4d3",
		"0x8736365d88bd1654b2ac5eb80fb0abf6a928bdc343c79d5691ebf77a1d2858867d643573e86bb54202c25dcac190b2f6",
		"0xb96c7323ed3275ee4094f3d6dd2a15f901f9022160b3f6c12b012f12bb99778ab25d51da3e4a66b728f715b511f884d0",
		"0xa42a3eaea10ad999a0a1433747175cf74b30f261effb22102c807a3ea1b74583bf39d98a4a759a9994db071be0d4f1dc",
		"0x816f52b1147489cc6aaf7e7bdd4bd5d0a12942b75d1696f926a1dbaee2bb478d1c923ef6c4af06385a529a7c4c9e1838",
		"0x8338483e2660ccff28d0ec18acce3bb28417a4ab8a1e79dbba9af7a16a62c1b5c209a0b53047b3367534e45b3fc40a01",
		"0xa1610bbe8b83a25d37d3cbc11027749df970b44a65e6a0805599fab3196ea62de5bde794f6adfb8690d09c8b82f5b151",
		"0x93372953feaba9cbb7b4c865f00c355bbaaf9de3ff24187a6d1b909cbe678b17c4c9f591130cfffbcee5169f957a15ed",
		"0x97e71d64f36c6f43bf70dab58b2dadd1611b79d55aa95a6764e467b01ed25f2f995dd60d033b6a665861ee76497bd98d",
		"0x99cfdedc7a9db0f00fca3a11d8b22b7c6a4985c102f06c2e7f3a539aa1bea64f4fa107b911860be53ee40eb41be65d84",
		"0xb1a19fe5d4fbfd4e432bbddfcdfb6ea17738bfdb2899a8689897fb939963bcd037cbfadaab9ff4e398921ed1d4835874",
		"0x8190cba80bcdc2e36e5505317381ec493386be49fb20e8fc7a413ec0682449f73da1ecc5d6966454f9e41b8c16c991aa",
		"0xb59ac28ab9e3467c0fc11b36d4c6ea6b932ba7b9e983012d6b710705a463ae013a5d40d207ad4dcf97a9f86981388d85",
		"0x9232a377ad5d663ada3ee0cde1f8ee13661e1b9d8cdb65e1da7773eac290e6b21c852ecfc17b767fb60fd3e0243ddad4",
		"0x8517255f0295f1062658f3f21130954b6576ae22b1e5c75317d43999bfd65179ca8f0c2714254e806804f2b944d735ca",
		"0xb8bb1af893b63fe0856ada49ec02197b6aee4d7fe6614172d4176bcbbf8a8d73213fd30727341dda02320be5bb9ca3c4",
		"0xa28fdb9340ba94c08d2c9eb2214df5e5da3d1a82cc45d444c2d5946fc6aae1bd7009c87b27f6e6565d305eb8b64d93b1",
		"0x8cca22538b3829057eaf965cad11683ddfe4dcb6539d630e98677e65c088c69ca929de04af8106061c22bd25769cc5cd",
		"0x8efcb2a3775843e61cfe71b2149f97b9ae2561ef0ae7d307f5fba605d5ad9b8ac4bd9ff5a1a11d7825d2fd6793ce71b4",
		"0xb2d3984acf17dbc0626f8493283a699969307504cfeb39307ede7400560db7f7a1e363f02b3297c3e2d39ec129ab6231",
		"0xafe070bbb31cf9d35fd8e842ebafdb8b40b408491ed129f935ad5834ae82ec6918aae5a94b7d2c698099066583878cc0",
		"0xaba3dea5ba3763b2876a906c87268fe03b36b31ba559465c9bf76fc588f21217c4506ab4278efa63dee6ae1aca37b0e8",
		"0xab38d56d75e9c1918ee41610b8dc8d3ce35ee3cc870340583a6d1de1dd7296a546c0cb946ccc046f20a17ab08cf83117",
		"0xae3e4aa0bbc128906390964d72908e6052dce270b4bb0f10811cf4e717ebd2b2f664511c1ae5cdb0f11cd89bb98c2e15",
		"0x9505eb447ea150fb5f772c1b3326a550fa07fe910d90e34423351cf4f1393b18b2b222f2b546d5c452166d6f76149675",
		"0xb9089af459b78f0029f579d3ee9e971bbc04f0f2d0e9caaf70c90177aa1285a7ee479b35327c629a6136141097e345e8",
		"0x8a50f627c3c9179f981496101f2980df0f45999173a27e1a18e8f56034677c9eb459cf923cc20b36408c4b69b11eb38d",
		"0x937cfd796f246a9655da09cf73ecabd714d85f04a01de4991ad075bb2660cff2c2e519f8b0f129a3e7a0d376b3667811",
		"0xa593713b735731b1b3e174af539a438f831c51ca3eb9b352993cd4c14c1a3b022650cd8d8f940a9b02223b83f18c6fa9",
		"0x84681a325f9b8581ba31eed314da7f849ef58c452f51e68e56660ad574d63d82147364ca3274f99f3fd6457d268a9a23",
		"0xb6b8c7fd68881c72b9632d40a0098ccbee5e4ee5f40ebccc2cfafd7d9e9ca62119d35ee8eca5436e91c38ed5a43d710b",
		"0x8354769f6ffb119312afaad3864aeebc21feae6eadf4543bd7d46ca350d519234397c4b1edd34acc4c8380b0c07db6bb",
		"0x91e67e23821515599a9ca23f92c5c77be474acf830246716924115217a39783049e815bacb87535754918e1aed4ced27",
		"0xa96e3994353f97e3afeae60d70b3641247cf490488833834ef6dd2394876ecfad24eb8caa4834e246f6d282a55cbb96e",
		"0x88bf174a0040260e7ab11e9e28e93e110dc0b974d06b44c00fe7064e3cc169f356700e60601edda093a3992576fccdd5",
		"0xafc68fac8228aa2e84659969a6381bfc7a58c51209917ff7a11b464c8a8ccbb1940db641e41c80cdd23564d2dc364e0f",
		"0x817203dfbb04a1a9d35216468f32c1157f6812e9ff4bd39ca3a45453b708be8183fd0c25dfd6bd1579585ad938411dc3",
		"0xaba89fff586587b4fef505f430757be8fa6ced8e685733dd007fc8fcb5cf4f5b0ab93a8abeba1f829f00d1058d3fb369",
		"0x95daaa91b92c77e0646b67d02c1dcc54089498b3864d5003efe6ea68d7e70eaea01e9fb252f25d9c8500f25047cec710",
		"0x92d5262b992e761d401aedbfcbb1d82ddd63cdad6af43a3ea08e2c1a1277da3236f8e54475649664c468faedd083660d",
		"0x8c0e9c1ec6aa15d19f9ec140928dda432c5ebdba41c62049b9d04afe383c8f4d5a7f0a57db62ff5a7729d1e1bee27da0",
		"0x93aac8cc273448005419ee90960d1697bda3516904fb4bacbaba1d93d1e08e7a5ccf1a1bcf4fcac8bc9ac83e02c274d9",
		"0x877e9303bbad2a9634f0b1893fef8d3d7a90db309676edce7f43f8fa6f98c3c06cb5601c37e4983e837fcc14ce415a09",
		"0x8fea6db7c57117c34391368dba1d5818a364d226af084971e0162e835da13c968624bc6270163d427087637af3fc3c9b",
		"0x929bdc183b537570f43f83947a1d826faa0c447cef04a78e8c23b1009f414516ab5c8ff9d7845c6039a30814de883575",
		"0xa299ab7d1277cb9db497787f376213a97ffaa8b4ac8acd97bc8a58221cabdf42902b78e58e2e14951359f8aaece12492",
		"0xb09ee6f93d4fb84f990a4ff15fc66561c9864c4ad2a60a66f1759f457f81d3d11782c9dfc91500eb2fedf7d3e10b2bb5",
		"0x8582294b7805239fe03e6a0ae5d269d86162b84945445faa8b182fd99e5973eeca189692e9a67162a12786a80d33ae35",
		"0xb104f4f4b2fcbb59f91ecb0d0c8f69bce0eae1abf75b0c62e606a12a51be51c1accff591303c501698b499d2d673784e",
		"0x91ac87773efaff721eb6e1f6dea55e4ad74d98ac83737667ba6ff7ec39ad1da28b330c3d4bb1079258ae816c4d53a725",
		"0x88402651e5775574e5942190ff2fa97d766e76ed036df60d13d58c1582c4b279339452b2b7c88ebe5ffd159bd91447a2",
		"0x8422ce0aa7e3587fec39cd14e4af235c4f086dc9e8f097ffa7915a88adf225d8a90b03b3e686c61de9eac27b8b64c8e2",
		"0x8cc885e693ac92fb02ef31956126d97a0f60515e29dc9f9aba1d5ae8af06d485dd5fe2efa22735a1c5dc6c7c6857c74e",
		"0x84d1a43c56f980d171121a8496b9b9e85089e5d375280ea562fc2f2ff2cb173ba060c804334b94ddcfccf39eba0cc4da",
		"0xaeed5e8b4ab1bc896ca430ece3c3afff45b21849b621561422ac13b127114d821c08939fd35e6a00e1dc03e2f810efc8",
		"0xb1b18ca0b33624efc348ad3066855ef4fa2c5c5fa24d13d7e5435b7b851fad5495aa3c3c75ced0cf24b28831a7b3e19a",
		"0xa1e2ad724e9fad7381e7655d3e4d163880c07aec5a320dcebd571c5da8eb6e1690183071e8bfae08fb724bbba82cbead",
		"0xb9e64b0d1a55042933985424fa0734e1ac12b997e608cc8e452b607696226bf02275aa73819e993444518fd3aad02802",
		"0xa36339945d045738e77d221888842b7e7cedc75d5a582f5ac0d623a7c2f1a25353503a238a0a714bacd4d7f9a3d180b2",
		"0x89e3b262fff20c9bca829cd15868697d28eaa30cd767232ffaac127b68d6bbb7b07f63db48e56322a424b7056df2a3c0",
		"0xb89f6adff763e740e82a1280f76bc70d0454a617b78effca069646b0e4d3cafab3376807bb095a8091be667146cf433c",
		"0xa9fcc729e07ef2e16d985ad19885e4e9b9ffe1b3575de8bc4aaa1e9f2a0f1cf5e274561ae7809a12df368c797b77ca10",
		"0xa9d8e8116512e3c8910208b0097c50f7f35f91a3939ee2c48df0494a84d6271e5cb5cbed5438deb1810c2773bafc95ec",
		"0xaee68420749bcf0ced1ffb5088bc3ecbe3d133baa8d95cbea5dd950ac8632d37d2c2689164538d5dee436a7d9979f243",
		"0xa3846d4c55a52e564279bc3e8dbdc6349c5bf7ca93d3a8a0d7acffd4afba327bf5f839adbc7872f31a55c92d1114f839",
		"0xa837ac3a4bb861844596802f7267de595c4ad4f5d1f0ec3436c8f33aed377ce4ca95abed964c837fe27652cc8355fd5e",
		"0xab229c8d9a7ff7b90a0bd310aee41e472300e33250561e8b9bd4baee27abb1f57005104c0ee59ae91636ddbb65e65060",
		"0xb568554853307bd8db47623aea44f893b367b99a19f1e5c3b1506657ccd9e71a74271e0ac7e7b0a647a1873f6d10a71e",
		"0x92b324b0e4b95cd0d805f125cd3077ca8f8b6a23e269e1fbb4c6eb802add2e16f1cb13e7bcf0a4ea3ee3478e754d49b2",
		"0x84c45389a23657ee5186b7ae608bb8a3008cfab9f3a15ce7ce2722be593fccc21662be96cf40b715bfcec853c57dcf2f",
		"0xa9da3fd74f3710af267ca46a371089d2ffabc0baf8bd8b92dabfaf4252454c856ccb89bfbe6a6cf91997fab11fc63049",
		"0xb740ba50e274bd12848beb4cfa4e0189af375c68c3213415d5475608f890062b432257c374a5099fc439b0a517727092",
		"0x8516f296cdfca3ce60575144fe82d167d487391ae26d9b6b706c3eda1bca3f29ab404ca1e3223987640f37b37d482593",
		"0x8f570658527938aabbf00d1f896a2da20d1d6afbf0d1f77cfbe9e2ab6d0a4a2c10ed979c502712a285df407ff92e863b",
		"0xa51afedd2e6c3984126364dd687b2319429c4c775bc13557a29b070ccbd42257c95d51ae0d4df90d3485a60bb39efd89",
		"0x8a2927c727768215aef93519932800e685111621b34a5be716e2db7c3ae0dcf8a729be7344c2c95dfff26e415b0eae91",
		"0x8d8ee6a0263a2573a3fcd0c0fc8199173ce32afc4539c084d2d315252ea3f75d7c5b84b5e122f4a4280f0365326c41c7",
		"0x93f770c2702b1d00b7cc04e1081c1637d2c209a565d9e2ec9d58055122a77b50af4a2e550e60de8b8d797c6430d2e600",
		"0xacb01c7b1f4eb84ef2001cd255bece52e14dd81a8fcd8b223031f3fd1c5c4687e8956d9e3457795e791e8e5a0bd3d306",
		"0x904b9b71968893c14e4cc89c6b61d447b43b6faf986920123394ffb261083ca956660f9455a9b976e05109d5c3d04ca6",
		"0xb55394896996257f9a1ea2ddc8d3209fcfce3450c56e3f6fee4d4c40926d706bf133e5cdf456e44d94569ad4340ce248",
		"0x94891d44693cd0f6be6aeb4bafb8fedcd5e1edbee8fb5c1a5ef3779a405d3317095d7f00fed418d36663f719b5d9b16d",
		"0x882e777524c283263b7c9f495459e7a8e1913556be716dc38b273d3559086e6f4ad6c039824a0b531c70473312c99629",
		"0xb62836619d3004d94b5aa13cd59bec73c0cd52294ab0f25039f0a8edc9de6277aa8229611042bd882eb0f7ed254fb321",
		"0x94868157b244fe7d884c71c1293fd9c572632de993e6e9478b8404ca42a5d1238805c8cfe1a9c8276ad958669d478245",
		"0x95d64201dfebaa4fa68e8512f400cf007805539853e620f1ade52e780ec10a6edd19aeb930c8124486d977ef55a64c06",
		"0x95e3a6e8a1c725ded8b43af8601e22fdab3c09aa5a218107b00647ece17b78e1789cc30f400d5991c12aa3babc36bd25",
		"0xa242ee5dd61879bed14c967a0a5d13dee623dc62ec1de16400e182290623ffabb9ca451af85084f0a26759a4355474a0",
		"0xa2d49690fd06990e7af6d337262c7be9523a298fda89e3e716251bad77bb8850be8400c171a783a538a0fa3aaf7bcf02",
		"0xaf9efbc761a8c25f01b19152600370c51f26aee923894237ce715366e20991fc6ae24e5e02840395787af89de2bb4193",
		"0x876223482d38808a160a502ed406e2cf55299a1cd46f7c3d08877fdb92f6c47e8f4c00d0cdeeca39dca88e88c81476d8",
		"0xa16cff032ef1dff262da788b0606883a00c1c2cd5c69e445e9279a687cbf2595d2b98f19ddc1a78ba57aa2d0490f6225",
		"0xb4e553d6fcf09d2f9dcde276705f710d93061c71e34c3cfe8e9d432eda8319799de0246693ee2f88758d131e56fb7c99",
		"0x8dad093ee3cf2b6e82745f6e5cbc2106bdd31223bb80b8ec2144890b4db8e42fe38e4e7cf3d67db6d787055c99898681",
		"0xa4d559713b1bca9fb5db6115e388dae3753bb02a88b536883a207b228f3c7a85ab2d1d7b8cce3d6ab794aeee0bfedd72",
		"0xb29b0cef08c5f371ff9174417644185a04d0cdbd17d209f84ec21684c45d0d29f8eb443a667e1709b65b52745501b7cc",
		"0x96d72389862551933cd245c036c65c3a568a342f3ed89235545cd6467482098d4ccfffe33f467034f97e3e0125bc1722",
		"0xb4fde8fdde44927c342118a88c5027c74097c56b7ee7e794437df9f1e61093a341fbab4a9c2f3021acc22163ebf5dd33",
		"0x964044d9d728111a9e7a579f88cb49c4774b7fc00ee6b09cbfb94d533c6828f0faf52a19c2f5030b2301f9cc1834d37d",
		"0xb6379df809d090b1a2691978410f6d4cf3b54b1c824477126625401e9856134d3c6b547c71793c15012db4d7d65185ea",
		"0x848aed1a05cb9ffdcb31ab10932791d09b691c51ac4b1676752cb27d906f62ce3a5e12b255c8da5a76555bac83616f6a",
		"0x90f87792166cd1d329be9f46390fde7c850edf49bc9e2df3103eda76d8679fc45589532e35799a043e4cac137fcc8159",
		"0xb11dc444292e14af21b4080996273e461df44af8206a8962893f6158288294152b92d369c0e55b18b5bd1596b07fe0d8",
		"0xb5bf0de2ad0676f9b0a61e687f56c4867861b9754af3832f25d25848c93c4bc7dbbada6b728a6341fc83e42250543788",
		"0xac785c5730502a2ee330b70730ff0d12e3d1047b6e24cc7e6dc77e8b8d8e131c52aa42354d07879daa64f3087a99e94a",
		"0x8371bdee252ce8e53a64d1a08cfb3e3fe1d804149619de5660f6dd37addd7d5e640a255ae7ff440e1c87d01170843376",
		"0x8b1dad11e43b7c48ea5db7971b2f1eeb2fd4b438081840ce4562948265170cc66a523ce28996054e51d8a32f330fa54d",
		"0x8f3380fe34cc0e5b57ceb216c1191ed30efb1657221338b29fef39c1908611dbcf39d31694daf39dc428ac5064679438",
		"0xb8f91a25f7bfb42c55d6e747fa62f2867cec76fc4e891d266fda81ef1259ec1c3285f52a395310fbb21c4db34259f4a1",
		"0x9890e53cc6254601381df078cb14079a50a47071b15bc123cade0e8dcf86b2dd5e5db151df4f209346aaacb38e2edf92",
		"0xabb67f51fba98f19aa8eb3a0c4cb9f32b943f571eecc4a2c2d210d8de6c398e64876b98104e875491c520586dd8a860b",
		"0xb062533db2e4bd7f40cc5566e21aa4fee7f995a238d6916254bb8ae92bc330a3091978aeb291dbe20a0e276759a67013",
		"0xb647b559158a31e4e2a7cc9ad2540a77d9d459d2680fa6156ffa3ccbaa033476e26346cd1dbb21d380c91a9946ff1af1",
		"0x856cecb93d691ffc74cd849bff7a574b3629c9f428da0b2e2b81cc524e3870ac1d711e3096c9af5e982db58f9965d4dc",
		"0xb0e12af81784852b3e1f6cf6a05955c02796777679b831d25d656ee504df91f1f31a11b759eaa55d9805daf79803f883",
		"0xa6f7b3000e8c4ffeedc30a5e2e56b6ba678d1bc1c53d26e58dfde8ac7b3e9f3db93f9f864b560e8a9539cdcdd42cd6a1",
		"0x92ea6fcad24e70199b199b9a1f6debf5ac503589dd6aa89a671c61962435763d88fdda512c28e28d5e3b7157269c15c3",
		"0x8320da3034eb2600d606600c0238847a2a547c873d67d26e75b2637a9580e2bb5092e38e61b00d713e6bb98847668104",
		"0x81ff19899ae8f3aec4862f2f538250881f358037e77428a6d96cc204be4c655c4b754087657ee224f741af77d6a746b6",
		"0x837e59c57181989b922be25e7b9f334ee488340405b72ca57c793e930c9da3a4a9e442206743417ebfa2e900775ad848",
		"0x8dd6c3f60b0cd3fec97d2f866ea3ec45e621b534621140fe357e6b119bd049f0a7558a063eccbfa90d1b85412a86cc88",
		"0x927bc46dcb1129cf6570fd8c84393e65fa7648b08d73b8233956cc469abcae58e2aa78838d09efcab1e0af68e77aa6b9",
		"0x82ee7411af7122185cbb1b234d6c19a89e7d0bccfae64e1aa5582316cff79d2f6f0ba86fdf7aef80a63066daeaf69555",
		"0x8e2ce05bcc7dc61ae2081d8d80d23102e1b1f65e5f7988416ad357c8ac61faf170bba581db5489f23d9b1f4be8174fd6",
		"0xb93d630c7346b48fdb83e05c6019f29995a5fcc6d00725e1ff187018127ce0a6948edcfcd0a48feb5397cf6503dbab33",
		"0x83221be75bd6deeff2b21b2eaf98965f9034a018dc345edc1c4a5f04f9592898a2df1b497f46fc81d5bdd57d73b0581f",
		"0xae40d6b8f7ae892a3e2a2c55ef385ace1421396e48241eebefa45d588adba6b57bb395d45a849515315d67154b03b86f",
		"0x8b4ec9a6fded1235cbcca5f10d358a255c0948090da0449bd01e118888d06f60d69e757008988afd2d3916b0932c1dce",
		"0x998e2dbfcecfd1d76487daabc133f2683d360c07d44b0ca3dc8a0c38285513a4349737bcfdb9bed5fac075588a110a10",
		"0xaee9d1d44b2b9f0b7265d58de8a1d09acf8ee94d9de582007842b07af806036d6564547a28f517a38a4312b92ce2cca0",
		"0xa009421b775ed38c4d53c3a6ce01ccf8e2ce4bbcd64b3af28ca446f809038a01efe15ac7546154b253af6efd30c77e9f",
		"0x8e370217a9a370955703457916bd410b5ecf9e030506eafeb9c02d75ba5eedb6d5187630682ad130999c15f063d7a884",
		"0xb21d3c5023422da0de8028a273b3624e662b75e5f0b5e5835bd3b8790671e872f635bc991c839a5b38e6ed9b49f94f8d",
		"0x83707b81ff4c06018ac7a7966af1327cf9ae117c57e987f21d11a11ca1b09ee7bba2965b5273232bbae7f22210b0c661",
		"0x9240aba086040c41379f91a8592c0bd45af6679874c842721c38f41de2ebcf18c05f06bea90b7e3ff32a1ba6bf72cb69",
		"0x87cb7184ee396f9eff17394b25b22d33e1c3f1f82756de62ff5a2f7822bb0790ac927a3e4deb30e0e9a4e231183048db",
		"0x95244b34700b97a6b85aead7f1b5650acc4317865f3654187027ed2ecf216f967b3e188136270843396b0511bf3256c0",
		"0xb462c3876a6552d0465d2f618fdc9437217106c3c61e6612ce1b5fa2bdfec970fdc8b80db96fb2dc1410535546f18b45",
		"0x816f9bacbbf35203b70bb24023c85cf3b4fb01879a7ef354cb68cb94433a7207970afbb84b092de0c661cf80a0f7fc3a",
		"0x899f6fc00d8e89063999007f21a4009f7dcd467203dd6b78ee0d002e590912493787789d554b543922611cd95ee36a65",
		"0x90b1db519cb8cd4d6bd9d9a1899df536f0eca9346b01e24ff029de8134815a145c772871aceda4a51c88107e890c4ba0",
		"0xb114ffbc029e7cba89bf57f8769accd0e9fb4d5e2236a9d6e8e5f85e5a36b5dfdc76aefa0f8fa84746e38628751eb4bb",
		"0xa32e2590763ad1f2cd7178739c9d893c4c5b7d04ab9422b28f7d3f0eec23fa78b726af7794e92405902dd7281899db68",
		"0x94ff3a9bcd796b81fea39ec7a0c7f01f313551e326e1ea74bedbe345a0dd70406de621298babb0b4537107aa17125744",
		"0x8ca6337c5500a6014740d057017ac26f964c0bf966a21717cab15cc0804261972d6067a26f3205b82abb5113a25428de",
		"0xb51d9d4b55716232349cd412aea2c319b4270f159421fe78db67bb769f3d51e3e44476897b19300644a988694beb45e1",
		"0xa4e472c07809004c3b628bedd902f74fd2af08e3700f73c76f5555e363cc1254edfa0bf9ed9b96fa4287be35c70cd715",
		"0xae784f0e8b7cffeacb44be2775aa5e555af7877181f5b538c325e556b0abb12fcea64d0fafcbf06d9ec69eb209894431",
		"0x93dacd520089515a847087e27ff38bb50ba2d0b0e268681d996f686c7a38d62cdb33655ff99ca03e15ad9788e7788cc9",
		"0xb0fc62487c8d2d3363808fdb9c4bea9e15106630826a68c4015f9dc9dab3db8575468c8e32690628ca7e6f802f5fe0e4",
		"0x8ef0149c1389a746deb829a298ef5b5dfd276a85375636ba572feaefedd39f595ffcc5db9f108c7c86b87f4a8a68fdd6",
		"0x801afe26a14207af59e458508cfe6ff8f7fe10d7819dba08fe05b43372780fdfd9310695184f245306810936075d5306",
		"0xae32ce272d590db88254ad0ba50a6e1f2c813596888481f7b62b9023d2caf8a07c44ee704bd4a2a6ea5d8c6f6e905910",
		"0x98679abf88626dbdf92a824203a47c8dc9b66b62e43c37b4ca17f914bc2dc2e0e628cbdd28e3abd2ce4ec0f558989d02",
		"0xad93bf539ce66f6156aec529d0732d8739ca5742748d1a9ef7d9ad3d248f186c77d8d12a50f89fe89140014ff4c2de39",
		"0xb6b860f67c5370f8aac0582160a2262a19c6102b22ac21c0d15516b8f8d9a6312f0a8fffedd7bc593cfdbd631136bc54",
		"0x80e1a48e8acbfeb12acf24865e4c5be89cfb420fe9d9e5a2c67ed24f056276b6e6cd889b312b054f66dd679415251ae7",
		"0xa36b8037d2e34621fdbe0eb66974d50bd126e8d57b0d081ad17c1e43cce8b8d0d2cd3ac5568e85004dd5cc7597e85c97",
		"0x893557115c783f3b4ebacd8663e3bc5795e55e1e0aec787d096dd3ddf58f71c16a502643f0f9b99f53adc1f568051d00",
		"0x9318dff1d2e41787494d6e20c48c4108630c8f6026504a7acda4ea52faf060589f5f86fe61f3c87e03f3f132598a9866",
		"0xb1e06f09b796efd36e5b63dda44bfe5b2ec216fb6d75d051751f0429133947c04b93d0c29bcb2b4e9fdfd2a13a7d0350",
		"0x98364b12d7d870c26a54f932aa9d8f71abc313f33fd787b5c314749a165e63b1122474683d1920d2118b98b646b8a05a",
		"0x806f1212d19aa24a56e22809c97e380ded346ba0fa962827efdfd632ba248b962fe463af44580e9a10a22b25ef1465bb",
		"0xb3f472c9a79d6f7d9b5168d7d671eb43ad4b7b23fdead29e18fc10dd6c90fec0dabbca72427de24892f1d0d5df8428ff",
		"0x8991a4319627664640dd0b85c338217b5f1e4bbbdcad3c49197febeae2af4dd4fd5a24c18064e20c047f39d033a55f7c",
		"0xaf473e51bcce010fead62fc9e5bfde17339fc6749bc4030b98a916fc65e80336a64617af8b15cea0e7e9d47b359ec70c",
		"0xadaf46bd9ef754be1182dfe097c7b5e1098b7bdee0af5e7f68e1debd4b89513912c401c78146ca1703b35c70e21bc5bb",
		"0x975fc714a7207cb7f617f90aee339291807af61118849f95bd9605834460dbb3284d8ce744beea86c1c43089ace4e0db",
		"0xb366f1368b05cef7c4a68a715140ccd76bd15456799a8c39e137a943ee00c906a516e2ff18b8438c1a6466c24610e96a",
		"0xa09934a1ba30e9b4ac07a314fb31eeb663491770e8df8dead20b4e9a6d775846cdd1c810ccc64f46cc820b3c013023ab",
		"0xa1e68ddab3d73168980e62f3c04e5460c1f7f11281e471e8d166a2ec984fad7a843792f2941cba607e16eaa70d768d76",
		"0xb698a0edd0d67d38abc50b14b6c59eae0d5de531163bfe15a79c177dbfa71349764e7ba27d7b2f861fc362de9d7754c8",
		"0x910671f4856a113478eed5267b9e630e62b6c7c047ae94bbb5f67568241ec95c76b24b0beb535499c8e2830e6909c7a4",
		"0xa06f1ab62b9b33befd515dbdbffe536a76b9ec7eb52b47f10d80bef7e3f329a4aca7ddfa08c105493fa8b37c719d9871",
		"0xa436a88863de25e336cdf6de59cce60d86218c38c7d00da0f37de4def3d1a5ac5be06e76cb138479147f80e2218f86f6",
		"0x97436be0cf96ddaab135e924e4318362fa2ae0ed164d303ad61e236c1980ed363c90d0692d20042c3c138f2012b7c914",
		"0xb55735a2ecdccee31a8016a2a339624e8b4cb07ed65a01f96f66464c476ba8a3812aac13713b4d528cd4ee5a201f957d",
		"0xa962fe4d0fcc91c0aa6a57397ad558f5c1f33b412faeebfc89aaf97f9aaf1e673d8e81046e54510b7dd2f3ab3175dfc8",
		"0x95569d7f3cecff412623ea41cdbbb1f825bcafd29864570d0726b7f55911ebf3624cb16e0e1a13eee765b37bedf5d5dd",
		"0xb9d4ca7717488c699a305104e50300ca88f82dd2ef09ee63166476507ca718d4ed2ef01a9f88d65dd06f91ea2a055ac9",
		"0x8294bc86a32f926dcf08609ee044ce7592d91811645051ce8080b830c6d434689e18269148d110952d00306d967df209",
		"0x99a3b72acd59a09462c539f5918b7ef0fa1aaf4279753ff3964ad36816e8aaefd732276da22834dc385e7c47df5c6a37",
		"0xa23cb5682ba05cfda2e6677f9cacc07a84adbf3dc34da5f668b72437aff7d9387337b52d96db8f9d590dc9f966a615ff",
		"0xabbc98dc19872b11b28f5fbe3993f4744753abf5523da2f953bba135396c9ed20a11ebd129ec8966246d029ca7be84ac",
		"0xaba7447f68070dc0d3fe6a3d8f2cda5f764bc2c223016b2470ee4738fea4b87bba13350f3dbff26afa7545de9c3feedb",
		"0x80033507dde373b92bd596c2e953fce851d18c90ebb6dac9e29116cef134368444f901cc4c84e1e1a6b2007bff71ed63",
		"0xa6dacfd29b4a10df5b2c347e1ba990a174e079cc5739c45a1e1bf2964908e5ca7acff2494eacb82ce384f9900a9514d3",
		"0x809a173c79d175f9450d1b3c072d31d998d7ad16be3a0ee86f6467ca994e46120def8a6821ca720469add2ca3296af5f",
		"0xacfa72a44e228e5956fc9c78212a1f339f7c2ca3d3b21cf60d3781eed073f5ec892f6a5614625fed8a75cdaf48971764",
		"0xb165573330499d77b4e96e4b664848729891c9371ab31fbcb8f2b6aad40caa8a47463f977bc7f6e5374ed62f26f43a5a",
		"0x921252e65cb740f8253186f5bc25b8f84c472af854016865d1a1597a770b08b9a494362c9ebf10b9135077567b90278a",
		"0xa9d91d7be8f054cdabad208ab63e5e17ea24f31c842fe2464b7e5523356833bf8e1c2fb3029cd9037d9292cbdbdd40aa",
		"0xa3d6e4b99121f28f2c3cf93d1eb8a20d5c80adfc9b36dbf0d6e928d1b679b9b1dfa939368a57912a1e2e5da00beca628",
		"0x943c91d81f891b57aa769f87aa61a3d755326809841b2ed0e09e43185cb737e15a3b524df3c8e76a795cb57b2d8b9720",
		"0xb295f98a1d22225f4fc288a25484597ff1e12dd1ac0853a9287bf20fcceacd4a7b90ee172215ac30c996a8e4e0281615",
		"0x83aef3f585b8c0bed4643596301ad56f629a3bef91b677e4ce19817ccb183f7cf4d48272fb7fd4376fac07f901b05c96",
		"0xa545caf088ad8610c0fc1fa199ad98565da863c75f1bbf4de7a9bb0f3fcd9bbcbaac133c6d3b38172089e9901bc16cb8",
		"0x90cee5d561af2181419272261961500e2df28c9113922044fc2f6d97cdb1be56366d5e6b50514965b89be23672fbd8d9",
		"0xb9cb85dd7a774216b5f674851afe815a608b16b7afc7c1e7096de0df0c2e921c2a84714f8f60a47a45d030ab8c6bf249",
		"0x84d4e55f9fcc8e8d6bb6ab08a13c280f6df0a6a314b90519cefc5419030937b2ba652ea12432fc2da106887c73b2f5ab",
		"0xac1738c27848ba61091c2524dfda428008cd03b1cb954bc0d66b12cccf4f04606ae7d75a41c330e06bc25464b709e185",
		"0x8b4eee8041a2a2c2a578dea197f88be474af3021f5d150ccecbaef36b5fb44ad85dd5e3dd9b9c647b6c17c10c835c571",
		"0xaec7255fbf89108ebe749c9f57847610e019a8273461820bbb3d84921a3b360b3ff39886f6c45991008f0ec0d5cb7fae",
		"0xa3bda9321b009c40bb539b994b4b62c8126f0330959a0fa428086dd61e671d4532bdce67e59a0d9191eeabf62f24206e",
		"0xa4b41ea0bb38a4e96b2a04df59bfc3b025b1050e35f0f59d33e47b3ab7254fda35976e6834e9107203cb3131fea2ad7b",
		"0xa8d40fcb84e48cab8871ed6fd3b13dc1ed41d53632de5547bad4cb45a1bebd2e87b5b80e5c8d933a5a119f4c8a9a9739",
		"0xa1ab2b88d91fce3fbd3264d98eee3a40a7be24c6b2b36b774c35241f1b4b1c71a8d0e1a5ba0430694e1ec7454aed032c",
		"0x8382ecad8d43d7ab1468f6606a37b51e08c29040214c4e41812d1d56a931ec2627e53e86aa6067ece6d8b72205ca9da7",
		"0xab4982f0a87cfe037aa518c585968f8e3febd2730a862f8c3468e4df0304135e686eaec8e93156c4af7279e1515c3556",
		"0xae5e1db64d2ac13ee29a0938933a6d04481c80999fb8df471fcac83a31631840ece58ef4199c70dd7d434ecc04193dbe",
		"0x8e73614b568e0747d382bd95aa4a367c9e58315421564d604635be083f7df5ce54203857ea21c67b565d71b1a174026f",
		"0xb536b862b67262534c9899c517248aa6a210bbf92d4a8fc3ea8f15aacd7a6fdefc13cc4876f80bf6dbdf7f014a3eae31",
		"0x9043e459acdfa32555d0274e77fafce73cf3aa18aaa97c3cc288824d774666b8f271528bd2138acb685ff54c4f6321c4",
		"0x8e983195381e581c43dd26f149de438ba2f2cbd8e9b6a243505cda8547e4a668cb2ee18c5aad1be959307d2cc64836c5",
		"0x902053b100b4059e9d08b92761b0784cc9071a8ab204c8ce3001937e4e45f0b94b4b5e8672fb726eba80369ba6333fa2",
		"0x98d99cae35fe5a11416e432a95d59a84305ca99b86c0b086a1a25000ba9b51abb88ea6de8dcc54e5c2b739886450a1ec",
		"0x94be6905b4dc8d56a7f82fd2fec0766444a0e4acf3673fed558ff29e27c5b48d1f187e777f24c605d1d01bcbfbe39c87",
		"0x87391174dccbefb4428de2821f66913798ece73567175a13116f7f5575177f7bf714fb7eba2e01a10b233f51538cd79a",
		"0x9227b7714f7770a89c4955fa7555cbcd431200a0ddb78eccc71e917f6b89d16223c90e7a0ca93b958ea385a4dd967a6e",
		"0x99aeb15cd69f24dab39cd5648ab69e179c62b576ebd7bc9a70c87b852ff66f8a9863ad34c144ca91c3efcbc79e26a09c",
		"0x8283860b11f4e8cd053c64f0b6444859694b95b23553211522f45a7b45c7a0c8907a658382f4cababec4c2ac33cbb809",
		"0x90bf06f0e2e77c9aa82e7b284ea62c45203f45ee914639bf7aef58caf574ad0a2c1c07a6280213c61aaaa6f273d16f70",
		"0x850cfa4b57f7d23afe3043a4e2876910c5c8ed515f45aa74e08eb8d479b8809bf583fdc1eda753d4cf3683fddee3507f",
		"0x96ab62c8e0f552311a5165cb597567a94a6e0a6557ce53739ff776ec48ba7a434b360086c0a8d80f422fcee0bea4c3c5",
		"0x96bb1e46f0daeb32a10c496750530990b242bd3b2293157987d69c9cc1911bc8a5fc580786319d66353b3605c80f24c2",
		"0x8be1419802bbe7df60b5abd56b9499c4907e0c0cad14c207718ff5a8c44e2c3606b5e5877eef1b264233098c7396ead4",
		"0x84f7296c50113e6902d24f0a8b31cccbe02571026d54ecd785ae52b9236a1db833a44e3d71d5d04749c7b6dceb0fa101",
		"0xa53df4c53e2e6877b75c599f709d0355b2ad9f2f6000716f6d18580305e5ac4125d9e370d42efca78a1ce8f06746f3da",
		"0x984c3a3941191f0ed3a6f9e460cec7ce379591f48d4a356639b6f983b48a3aaf8ba848f96125005f15438f3223b88037",
		"0x89a7d650a5bd91e287fc9862a4ad143c4a752619e95733d303cd61b3f668ca4f837c7ad21a6a84d2abd85e80c1e0c8e1",
		"0xb70b4a1204cab399965d17b513a2cc24bcb2011e1bd68e18cf11db45daafbaa0f3d76d7a008b1228d17ce2da6296cb96",
		"0x94a910b162596f6f1049630f8b8b152108c5b9c8aec1f99778ea0bb07fddd5d482b9f01c88d64c638ed0e7b135c3a150",
		"0xacf4ce2b67a5e1320aaea3ac6db5c7eeac84e30449ebf7aa25c9f11f0394097544e40d505fe3f2404f3ce6300e7cde81",
		"0x8079e1876c59314988dd810db3e53a8e130f4eebbe6e2eca7865b532927c0b2227cb0dc95f036de7805364d416a9d27a",
		"0x89d78852d0edc3fb6b0bddaa7157904a4de73d84e3e10ee4bccdc5a8282ba27d98c46464c2c01c609f038a58791fa42a",
		"0xa8f303ee40e33cb50a5a3743d1ae253547801f23e7217f9a6e26460d7670e29e24994d8b8044d215a39fc44bff1227f0",
		"0x82cded206ebe3e817e4faa8d4f38e4b0146d4586f9579afeacb61d9435c4eba35f6bfd1b610d91ac646444e5769a9533",
		"0xb95db83d4bec08bdbb5f80450d9cdfc1002102bbb4b4ce3a35a8f498d19f772ea5cd37d649fe4ff2bc51a992e02e333e",
		"0xa9376e6e241068ae8b7868e920201fa06113991db00402d5d388dd5fa5dcaa52dbe535464933599b8d491dd7f3956289",
		"0xa60787a49a241d97e6d4a072905eba18f8177f7ed9c63adaa96a7ef3551f95382bfc62c12c3bef75925e4ab9d48f4d57",
		"0xaa8e71c8b25ceb02d11bd9f90a8f35108d2c111858087b3c3e6d919f5b0a05e69ee872c795ba22e8a77385258c443950",
		"0xb9a3ddf91574273a3a7c97af72e6a7218e95891b46f28751c0aee22b741dbd4d4129e5365d0f96e07ed9fef816d86221",
		"0xacaa476d3f3830607544b99a1b63bfb256979efde7983e8b5bb0208d5a009381724c4303ab1b7e4ecc3bdb3ce87fe249",
		"0xa2ef389a18e2d287b77600db42561190f4996e2e4e61e266fdfc6afad050600ad019628cf7d5ddbcaf77e46118966e8b",
		"0xa7220ac8c4b2099c4b6b71fc4f27ac4ad1c1f3549cd882a63c8ae0ba45decd089a7ae9f1b79883e921d2374f0d4a66fc",
		"0x8ed029fdcb5200484b7c645265e2d9b430e1907c37111768927e20d1612459ed1a9dec629ce90b0a23bd0f2f81f814e6",
		"0x994f49512fc7f8499d9ba4c9906df67c2f4c4e1f801180a179b7fc4978c5b4f7d690c4e1f40c473d5ab75ab7546f09a0",
		"0x87c2e4c53da9f267d6bd763fa3c731b0ce0d0ebb29ee09c209bec92d70b4d523ce816a8c9d09ef1b561ee54601875e7a",
		"0xb4d6ccb7b18c6f03e68280f2862f3b46ce59b116e4762a7c9227f9dfd356e078c538513dbc81cc1172baab4c7d9f99ae",
		"0xa9ddef75684cec7e2fa29d1bf2740e93bdd5a38e1443124a8b62897f79ddb74654d7d6316637f19cdaac0a94158f508f",
		"0x8ae99e2e0b27628ea1f7aadbac384510cebe0a7c0288a666e0fa43e00b772f07810717ec00c85df4177ff09aef5a879f",
		"0x9438d061cc8d199c659c25dd01decf249bf96e4bf2bd3e2f2117b35632b5ff81e8a8d87d9a3f88d8d2fa8f0d6a6f737e",
		"0xaad8b2a49ef936861cae8b2b90ec1b7dbd69723c505ba1eb5c3d60fc014f8a4fd952e21d5d90ca14804c638135e889e6",
		"0x904cdef3d78e146e7ac2929a71ff7c7bd02a02ce3f6f2d9072caaeb1e523bc0e8cc7d5865daddd2165f75caa8e523d0d",
		"0xa63d4f426e76b8c7531d8dafaa8f51d78291f454be5178af323e88e08d9e72c7e2c5cc7695926dbe575a222faf5ffcba",
		"0xa638202c2e3d64d1e4efe5d7cfd44204ad0ef610dad324337185d278d641d8c90be07957a5c5f0e58af90a399c44eb88",
		"0x946f4eb21dfdeb478445913b609601a7af1fd8746bd13bb6e334048be6908386fad5e99d61424532749c078b7d64c732",
		"0x8bd7e2ffd6985d4b20250919b1f7bdec01a65ef2f0b0c885e197b2aa1073e46c2a4e6da95183fa8cfab187ac698dcfb7",
		"0x8ac3aca7b3eb774a047126a8d3d03cd69dc79fe424ad8cad9e2af089f098c051daa4c627fcc73304597a0e5230cc8c83",
		"0x8b95c66c56ee551a94c6f7635ad857784bd4a84cf50474194e21f182b3d5b99d7a3133d1a14cf2bc29ba0c37164b501a",
		"0x841b767ec1adfb21a800bee621f4461a808a619a75b3da3b4f83bb7b301111328419f24fc4ca7ba31ced522be37978ba",
		"0x8c658bb36b507f423e3588b98c372e23f2b871348878f0544d174ce7fed5ddedeadb98391f8b941c362cc5831313b6a4",
		"0x8f9660625c7909a11e7d4db1198924b3c2dcfbeee387ed7b5f291b095db9ca942e74e7aeb59a05abfebb89d2cb971478",
		"0xa79580a9e1bf0f63a06ba42a1009962c57ce20de81416739b849076b66702994628cba3833cff8678de5e70d381a0018",
		"0x867968f9aa8d37bb50f7b5012ea6c294f07eecdd3c780a735049f42d446566f58e08a1ed4b33c733a73143ba79a20d74",
		"0x81bbd3759f5817926958ebaaef4bf22f2ae61102d0f5740918f24849cd69f9d5a84dc7edd6c0990697db4f935f387fe9",
		"0xab799e1dda9af7f6954244f2d50e8ec0734ca86d6348b35d7906a0bff63afed6e72ad971ef3ff22c580a703acebaf0c7",
		"0xaa54782e649be42f4dc772c7ce0e7c88caef54f2137817a59ab0439a06aba3da1d565137d2ff022802a60e3f96accc70",
		"0x92faebdac1028e92ecfe0a8bf092ac2dc7e479a9a4dc317f7b0b88ba9968611ce34be2c13839539ed005bd0a56e5fe7f",
		"0xb25b56ba2ace6f79e2d7b035086529af5b7d910241d4404d0fc775f880bd8c4280bf12992ffbdd55ec57ef55a8672998",
		"0x816eed628266da5a8c423a43ce43c26ef5ffd70f6c0f4fdf403d8f1b1982ba7b1607390afe59950dd1072cb06aa70244",
		"0x8cae5d83e1a185ee5515cb1471d6069083e6f900a111159c3caa504d5dbf602d43df7ff2f08a583d6ff2bb68b37a931d",
		"0x88da62e02fa45dea6fd479a26d2306c01db45ba8d54b549c8b7f3579b4b890f0eaab2e5940c2c8b09dfb40268e3ec4aa",
		"0xab6923131e40c84a775811b5a1150ec54419644c68efaad02c37c03ee8bd3bb24defe7acc311f3b57f8dc0f227249730",
		"0x94890289c97fe2a7ee5c138437eefb4b08c38b61d33258e272067ee25b481cb6ab8a35b34c705982479f153f382a4f94",
		"0xadf4dc57cb95efe94b8edb46df069efbbd3708b4f6095dfa2e3a58317366f747143cd77b88a25fff12034bed286e8ff5",
		"0x8759a364801a702ca719967a6399d549f4a340e77072e59b177aba9bfd6e6c4e7e67ef5ab94396f7421cd2e77b3270dd",
		"0xb668e1c4f197a926e7a7eab84ac7c252cca9ac6b6b9b9ea0ad70387b5a6504845767bcf18253706ada21b1b818bc8616",
		"0x89b17365bb79ce5054f668790c23f797d3f5f38beaa30ba1003371f98d78a324b8e7a4bb36dbc45e2b86c9cadc02caee",
		"0x8c58da59b5ce6b956d994245e3feff4cb3ecbc0cf818aa21c1b02af6f9a6633119c5f422c627d9d64e69cccff8e1f0c7",
		"0x92fdc5435ad74e604e446d36c629ad1eb57c7b6822b2dbec674f251abb1d7937dd9a0f2b9f32c5f155935758838f207e",
		"0xb3e369bad1b2eb72c8e2327ca5b03c98d0d9d5eff5647ad761baeeb7891f49b127bc40c90e5f2d8fb417b260b4b04506",
		"0xa4f01c8f654c71e0b690f9c3a7842923d57557ee42b6e5e975e1727ec228a5d895e473ece5621bec6076a71222baf3e8",
		"0x91a42aabc7bea9573aa0c9184c1815de88515f46b939db64efb73f0179350d7b23bff570acfcca4d0a426384f91fd676",
		"0x808200366183f72526de3db97705238666416e22ef64052e927712e8c341246fd595f584f80945dd63d96203162997c0",
		"0x89e0b92c33cd25ea120e3eafafeb3d76e7724b6d9c090bd24f09c2010a0345347525c06595e2bfda72f0b4dddaa1af71",
		"0xb81eb99bf73c65d6ae72d252abd6026664e18f081d9c55664dec5ba2add77fb674ba77c1aece9abc3b7dc4287caba40a",
		"0x898a5e08438de460a22a7bcd416db11872e5a9c3294a64904ba1d26a84b320f80e40b0c90422857405e7cda7443c7d3b",
		"0xa3caecea1d75333eb55d755057e583715b51032fc7d838d659d773e5003686bcfbd9ff7e950f86045d75f3bcd47ed224",
		"0xb495ef0f953470f711fb0e5a249cb1e34f1fe15adf9cc0230bc83b6cac0fd6898cb2c4e3a3efb080a420c55262f7baaf",
		"0xb557d939618379ec8333721a696b570541b621de55dcb262ff3836ebe97f672a0145f52734555695f7fed4297ec67ba1",
		"0xa6887a887ab84769a8ac4c4eef87d3d2cefa8e6b83b34fe351b37419597acc48b69d20cfc368856cdd4d3e2be94b0953",
		"0xa6b316ed606e4d4920aa06c0917f78afaab6d5073a64e72504d94a8a032c62163726ac1826162bf866fa04e45e18f522",
		"0xb00495a498cc5e8d89d6e770642568f29e520f02e75e6e3c1a5e534aa1baa95ac0f9dcb6840470e14a3a9be0cbf5f267",
		"0x831aa092ebe175d4d1faa3ad0e47f4a571418f6c034dd24a3e08e115ec805e8be9d80015f3cea0a6acd439a2a142fc65",
		"0x8a3b4ad3073bfbe349c4ff390918af498ae29480509e1170432461206fe75efcc424798c02531cea6e0ffb78987264a1",
		"0xa896dcba3624cb550d9ff7e0837ac9712bc09c91ea095e2183d570604beabce1254d1d69507231ffd502ef9b416012fa",
		"0x97b8e349a52b0ee77b6ffaad10f7bf637c4346236f13d8981b7a511274a0ab9ad40675f3f48ec5f770a79ea4cf38804f",
		"0xb5afd552b2256ed35533d642b10a1f8c73749e72ac95ab5a8db6c11194aa81340b1121a7f02f8916598e9318c346fe56",
		"0xa2e55e7dc630685a473472380f6c42503951b779ef55b6a0c74b03d5870c29b4c2a0225c0da6c731ffd873fbe7b17bee",
		"0xab7fa5d85334364f9101df3b1fd9d2ceee069f6ffbcf2b1131a4aba0a9cce961ef4676ebd77fe43c2245d0e32d22bd66",
		"0xa3b642f212e4f2ce2b8662f61492e8ac8891f5a3364f26d758ee25710d1ad5f4294c456e700eadd694de78fbbcff6132",
		"0x8e2db3b4d445f3278be273182472384c083ae8d801b5316adf401bccc12258d6504e8ba0810bf2344e1f6dcde0c57774",
		"0xb3f41ab416ea914252a81193cc69b28529e167a8498fb85b8ae867a7578ac6a41decc3a340252c4f1c0b6377685d75e1",
		"0xb42e0eb943593a1e608c590de70e40313c3dc17cda3b42d45c4fd6e27eb7d55caf86155bda800900e12b8e83c1f42bb5",
		"0x94e142aeb1dfd7785d1e989f969d13477183fa226dc0f86d63eacc85c90ecbdbb767fd5574b1994960aae3f86cea506c",
		"0x8aa37f8a656a41ccc3baa23413ddd5d3f79ec33f6aba0027951a482000f519348740fa0d5ea4f56e7e561a27485aa048",
		"0x8a5400b3ab90225c33a8ecc900853e1ca2608d9c71f655914b8fafce84ed35f640d024633cfe749486adae1f3033dd08",
		"0x992b8694727041496054599a33da62943ccd0178dc52832d6ae99cb71b352ddb67b921295098dc6ff514cc668bf58bc7",
		"0x816248303ef6ca03504d6c89632889d165b83763df881e78ae86646b3a61fd4019d7f68e55101fa9ed458c6fffcf82c9",
		"0x80a22f8180abe1eafa61b08e39b7ff6037b4b1b2d4e45d8844178ba8265f483a8ad39c7ef46d4ed4a7460654c8d8ffdb",
		"0x994da39d923eb16020d3feff2f857115e26da50976aca3da7d8031f9a095cadbeeb5b24abbc694c9497fc28ae6a0bd19",
		"0x9563bc90cd3f93b4adadd55ae345ddd58c23bd0b3096362b3df9eac2f5ed00f61ffdabc81e2f686edad6a890576bbe6d",
		"0xa1a2ac824fb5b85e1e57ba56e961f076979871fd236a12e1d3d2837296aa78b2603e4e7ed53522517401fa6d6dfebe06",
		"0xb16862609e40c6a4fa67abad93609abbc206ca29c23b473899aa1a637a402e531e1140160305baba89cde516e7b1334e",
		"0xb27fd5b04f21accf81dfbf97422319d8d5fbeb218e3cdc9699ffb32b9cfd86b01edfa6ad1c35d5cb0c4806a043cc879e",
		"0x808d923e56bdd78ec88b917c0723f566bb5eca010fdeaf5580da745188b8b4e42d5a27663ba3037a5342e498938f6b15",
		"0x8b72a8da7a7722cd1a33464539e40e269b2ef9926aa792aef56388c2289554fc95a3ed9703131b2082ab3f26e3925bbd",
		"0xa430f34b18d33b377b0a519143471495ba178cb0dc552656f3f6c12940da4bbe87e2735cb0602173cbadd88e08830c03",
		"0x88cf8e4789a8f95474b3768f56ac01a9c20aeea7fa1a4d23df08ef3b92dc3730306a8eef0892c121a6cc137fe5a3853e",
		"0xafac15297f363d36f89303baa2f0cbb90f0d804c73dc8b2e471b9704b6c4497b46fd86d6c9d51fdcce05108e42b83900",
		"0x8e60bf7190311a756abe7480e607feefbf85b10120f29685b5a215c2b996baa71b024028de2fe5db89c5e3c81d551332",
		"0xb7cec4c2de1f4dcc6dd4f006a0a14756a9ac510b656c3317952e2840ba7c6a8447e951338f45ebc71bbc3a65c01b632c",
		"0x87240bcff55418a136aa1151af2a9b9c215c8bb8c85ab42f931405caac39dbd73805af6844cf776b10eaa8f032e369e7",
		"0xb666c57afb523a2a4a45831818520b344968aa1e9a04f9f906847151b91a2116b73584264fb69dfc04e0cebf9023f879",
		"0xb60098041be63764edfd61bd99466535106707072f97484e79f0e2fe068d8dfc1a6d8ee2b8955d69e0271f0272480ac1",
		"0x89d80c86b8771aec42db8f7ed2ee8c58e181202df6b63aee67b29eff6489c612cf9997f4870ebc224a1a62e04b6de561",
		"0xb80647a7af80fa4a62febe1a146780965e1dc2d42be0dd11004fdc03fa02087c9e6ee837feb56a3966f5ff4e9a52b653",
		"0xb2aa3c29955321800fa06adec06ca8cc98ca22c0f78f2b33e1b15d06d26674826f1abcdda947ccf60b86b52f3f017482",
		"0xb1ae5316fad5ff0a27b3421fbd426441aa47238b6123afba9b52eb4a8daa0f48904a8a1d2c1f9860de5a3b54a574383c",
		"0x81959f44b71b1e22cdbaf7c6a1e83563a3bcb820ac45cf4bfe71a4a9cdbeac8ef17fcb9a960438b1b521aad3b013bfad",
		"0xa2b66e0bafd0d9ec8f9d8623dce84daa6ff62b9a9599e5de0cffb60c2af0cf12d2fde163f3631db7d337a31e4f8fbbd0",
		"0xa0ff998ea3658d980afd48819a7e9a4090d6979544d4460a1238fee329a01eb0dcf26c5666a75116f430d13411a06726",
		"0x96974713a4eed231fe7fc1b691dcfc014d85bb69070d479c29d037766ec5bd15e43f95dca795bb3d37c0adab9a758caf",
		"0x83820f04fd326a899db2df8410d73342ed7d4c77f0558d8866ad7ab8640f47d2eb312c0011dd1638fb2416cafbf88aa5",
		"0x934d1f1e0d4a2bbe3ea2f98ca66dc40090a2d5a867c69304cafbcbdfd1aeb39a13328cca2071990c0098f33881c11733",
		"0xa0b9cb2ff247e6100053335047d9de99044c49355835aaeb9fecd1be2e692ab539620149d0680a04e30709681bbce40c",
		"0xb375250903b2b8a182300e8344505104948fb9a994ede175162c549b3a3cefc16f4c4b7757807218a86266a3180e7edc",
		"0x8dc6954b0e777fa53c5e2806636682caf10bf4e48a84df17740e1a5d788dd530e1ff0c941cda90f08b716e4c327dd10f",
		"0x817340d1543351dfb2dd9eda5fb1ec5b84921da9b657436eed4657d0d70f6d7a50941bd2361e6187c0e87f6b10c65a0d",
		"0xb8601dd08c803531a6c92f7267e99e0e650cddfebd968d91300e543a86ef25693bf955eb67e807f73d1441b6b3bd8988",
		"0xa6fbda1c6693b71cfde0171670d9f26eb732541970d9e56255140fadf1eaa7e6d9102d982ff48af4b3f5913a0f913b78",
		"0x97bcde1c6973c0daa33089552409bbbe9977a0f2d278caaf955f7ad8a1fee77dfb8a9b3b16531de85b8ca0cc1a539094",
		"0xa768c94c2221b27957e0d475c43780bb7da3e85752228165670c4e5d5744a10f2d92498cac197da8078f1d5b74e7aa3c",
		"0xa0a4a8a619f0bf9b6ca75805e6c3888d7aa5fba1e5d7b3d139f785d40b2db55a3a2778d5ed418d79fac5677d55d5657f",
		"0x91561e1ea839e038a4614408f00d6866a84d0c40a786862b8f8402df220145131f2409e949a83854b5489710cb85a0a2",
		"0x81dc89c7b9ef8a0acd05cce4faa12c0390c6e468afeb4aa648b80f62523be8501024b6c83adb3667a5303c41705ff4c0",
		"0xa988ce7c50dfa14355aa7b01cdaf8c62e4030b939e05ee61ee194be11b4d735fe83d23b067171b3eb8a82c773456d7de",
		"0x8c34c8905f0182cb06b01a8a5b498f8fe70cccece83790120cfd29d54f7dc1cf9d5fd190ac3cebadfb006f5dd46ced9a",
		"0x8005565651976d9ffae75f60cf0301caec924c97a06ca91d7a29f90e74dae103525adb740a7c1eebf9be11e6e52acb6b",
		"0x95980d3673be2c2cfe35dfae1ae84d88c372815f19c4e09c33a79dbc6a705e24148980d0f7713ab317903013123b16f8",
		"0xa84364bb11ff62bc6a2553d0d927dcf1c0047e81dadcd69df6002323382f833fe6f0e767bdc44e4c3064e45e279bd79d",
		"0xa0eaab4297c535e860d56a8244a78aca79eb056eabdba6a762005765118f9a044efbd306d5bfb03c1bcfaeb343db007b",
		"0x80bb3c793ec664d932845569f8cd299580376f1450fe4cce296bc878db73efaf108f85f0cd01fd8f5978b6d0f79337a4",
		"0xa544f78f625db93f18389bc7e7328c22b4b767c7eac7973068f4ccd707864ff8c39e84dd78ad83e506b1f1d267bacc1e",
		"0xb0810e2221ddf791645fe861f8d08cd0160de627f3e819994a00d84f2be693cbfcdeda60b314ef6861437ac28eb8aa3c",
		"0x98d1c2cbbe89a6570f43da401c54d4de28e40836a510968609ad539687d83bd8eebc54ab0a9d863534c370943a3ed27a",
		"0xa86a12e48955566cefbb810c87834f39864849fba9e74918086a4a52d4759aba06b6bcf7c508b270211b7ebf8f97cfae",
		"0x8ab1f50d7fa520b1bb7542c3f2403e5d059dc10100c61e159f1ddc16c77ad52b6921fb75249498d20205ceb1fb18ffe8",
		"0x94f27bafd43706fdba660337422d092b91b0434db3a2620ebcb6e6be76c2273d2eae91f2b448bbdf3c08a82968f80495",
		"0xb62652577d18f67c0f733a696b4022d5ec64e8270048d5c7a6775094bd52bd793c92e477c3c0d9876c949c62c915b795",
		"0xa7e19d4ae8bde48514a31c15522f3205178abfbf4ea84c2551dd3e47ff6cd10cf2227ef8515dafa6c38f02d660463a7d",
		"0xb331aa12862837c9239888889711d753bd430d4919ff0ff8e5b58f07d34f080005c282a4d09db9cd699d3569d4844c22",
		"0xa97c73f2ebb60ae625e5c422f8bfb1dbc6973739dc11e444508d15c5c2645e053e5f9c82b73ffcd57c999d209adfc299",
		"0xaac7bc67c6c9d0dbf4694786538ae63943f40938df38cdd0180c2c8faf21eb6d3216e274d28bca01f19e6d1ec2b5800b",
		"0x8a0061aa0f6dae50c91e0209f9de9d79358275a9b78e4aaed5185323a12720aedda5d2cbb3172c92c8f33caa67586bd2",
		"0x986690924cca214d87f6ca07a85ff6256aa530df5e9392c56bc55e8e762766fb1dc6a9939ec674a38ce6e39a140f7c37",
		"0x8f29086079de7ccc871bd0ca5973ca0a423fb0871dfb784beb03a2e53aff4736bc37d35e8f9dfd1f9009127fd22ceb78",
		"0x8ae94f4679318ad7b9e2e7a85f33ebcbe8094dc2902911a2f369c8fdb8c4730f81d571bcf40f2b863c3890dbdb5ffd20",
		"0x902e68c5c38ba0f8b2615035b203b32b7b4852cb4d9f92b1e070b5133c7645c528867d6e66562122a0f41e94dd9a3c3e",
		"0x88cbebee1b69fde43486ef27e82cf82aad9232535e8717e60b44b319714bf1328bf138d94b1c337f5ddd3da07d49c067",
		"0xa9fd822001de2b6f222ff0d8ec86c6dbed6b59febfbff1eafbcf2437b52acbaf0b6ee1d63c59d61701e073b4cf6097d0",
		"0x8c449a2ff5fa885b594ce1d6fe9c85e930fe593158f1250050f1ac352427c9f5a5e21b9f7eb23836782d7cdc58b9cdeb",
		"0x8e5e68fef82310d567154b686b6f5e21bb94eb2f280a291318683b64f3a31dd782b4d2e8642c346702d32865dfb06d0d",
		"0x8f04aa4934df687d5a525ea71084a9f5d1d8013336f7cfe066a9a65f2af6e50abf573f1c4c5d28ea14cc63ff65d2f794",
		"0x9651fbd0d1d0905c17fa3c7104be17910b4f71c0b87819fbe17f23604adbf2520fc8b2f6197789e563e1b2a2ce29f353",
		"0xa643e3a95e290119361737b48db6eab8ab2e0aa7d73fa5fd611047fbf1546c33e95a233495251072fd7f96b48a47a183",
		"0x91f3fc4c5f27485c1664f789a775185b939e8e0d20aaf8996f5549ec076d7b3fc57a7b9ce775582c06ddac2c5d89cb72",
		"0xa2a435d4cb2343706d9244dc0cd1da7559bdd414c3dce53747d40453551d0b80a50c51f9f3e76b1aa50c91f0093b1e4f",
		"0xb5d99a8219559fe207bd18183db4f528302b17e863642f03d9a2c122efa15cfc6c0bff9872b9b76a17e7acef7e380d49",
		"0x80927197137aad806462ab2f70edde8813477068db210655cc52fbb169efa374e3d30df296ffb4cd85f5a2e4602944a4",
		"0x92c178c9f53070966bda6a449e56a0b74baa5679c7cc1cc64c0355138342867848cf3cf82ad240f528777579c51a1fa9",
		"0xb569f8c4d29925dcf0a3cfd780f061f29f1f0e93c538c2177d709161eda631dca5c80578866104f197851ca6255033bc",
		"0xa175ad6ce078d1e611c6990ba98033195b752be08ffc44df5bb0e084e4b71366135a814897f3235c60cec9ab8871d575",
		"0xaddaccf3fa937fccd3235d3864eb6fadbb93f9f68d8127a2dae4337bed74140791aa7766c6c4939f0b2324ffe1954f1f",
		"0x830f6d478b887dd625c700f641758f7f25fc2ced52fa220b11fc02cd9ec65ce8a8cbbd0a50bc589a1e838b0759e8d8ad",
		"0x99a0478b19de72c5fa8bef0a78f51a48814cf3522b5d78e6e302cb29f7db1e6c2e2b480d0bcb78d71f659c563f438715",
		"0x96a069a1619bd665276075e7b6277ab736f99bb410587677b5f5da2227d5b517e4463828987a5ae8d10bbd4ea91d0c81",
		"0x99141409092bd321204ab1c191ae8c01aa4ff88d59d5ca1d6e6c36f62287c253b10c90e3e47a74eb7f79690a43fc355a",
		"0xb3ad9f4d1ad663b664f6c0a11a319acd11eec780d67b1ded7c28089bbdb30b5b703f7b78ab42b98275a6ea9004d00ce2",
		"0xa1d69ed78ee410de4377545cd1a169993f01689c6e260b662959b83eb820a876cd025bfe87b12567e207beef688fe7ed",
		"0xadaf55c3833e274750297572638c406b3ff76d98543b21631ffcae443861fbb950bde72c9148c9f830712263c19b44cc",
		"0x8fa85658573f1d77544fa5cc14fc323d400d28bf34a2180d0af6b479073c4bcbe129a38fffdf185c9f2fd0a575257fe0",
		"0x876416556335fcc5a233ab3f9b2af33fda0b4bce4cc986fb9a0c5acbc8c70e7b1e9919b6fe127c4e8cbe428c81d149ac",
		"0x8388de5d4be55d97723989c1ae823c73c7143316205eda0cd428a30da0264cd0165a144e19e9c713425835b45ef44315",
		"0xa4ea46fd2f6f88720ada114c38d909cf54f4b07d19c307631d9472ef535bcc8f3ed82414021740228f6b364214a83421",
		"0xb841d23630d971ac43279ee788209a8ad50944901205186a4ef3d2b07b2f50c7c31e734783f6126e31894cd858560ce5",
		"0x82ca93476def4359333e45e7e0657099408442a8b702e9c71679d9b4ffd6c08b9756709a52ee079b861e5f092ed096bc",
		"0x96362ab4aad47bf11f96dc09c1ba601af717c9a4c5e6c869750872764258c1ea84a0c3bd350535cffb60ae8e78b5ccb0",
		"0x90e06b8b1111b4a2a4b2e5f48cccc4d05d7c47288795a5abc2cfbd7fad92d24b2b0e540cc2751a1942a044218608d0e5",
		"0x91c2b5f5cc7f05ec48b5a99d09b3e5de7d1da41db036d492e80e42d65fcf3e3f6019c64b5c76e844040d4a06d87f4f4f",
		"0xb3d2e546cfd9385c02341188ce82b8061cbffe3fe8772fe8c8ab5b849992c00ff71b36489284864b97caa0c34f21f953",
		"0xb7f558eadcc017ceb17b1d6ae03747deae2bbf8bb88c0b5b54f2c5f183f197c26062a187dad9d40c41aa3817ebbd15f6",
		"0x87ea3b083d150537572b11f3945d92e639b32cb6a0ac01254d2a957f5c7abf1cf0a32e4e345b27f2347dc2a0bf8b04bb",
		"0x941f5285ce18ebffe165ac1613d42f1189bc90e53a107cb8f1ffd7a73d28c588dec40c4f12b35a50f1a5a677803ba2eb",
		"0xb39a1a95be6655c68a8450e3fa65e2efeaab6af2f6453f422d828a582c75f506641c56b933537adeb5ade22a3ec49f24",
		"0x81f6c08e2905e820eab3b96b672a9980500ad5a6f999e89a5dcec77af9dd811ad81e77b2186467cf7449f307d3d87d87",
		"0x8ad8528ffdca423366a2c089ced277dfcdcbe6030864a2e98cf6327f8212e0c714f314fdc58f19122d95c4bab7bf5db2",
		"0xb04febd01c05ebda9cf550995cfb112fa8a700d872a3ff9ce0d29287fbcfe0f29cb4e57c47d737eebe54417202f9be88",
		"0x98e2ac5a2bdd174d664a1e5ce301c3c95964f2ce39e5f82232412133e35cc0cc16c57a258c354f207e09c024a9b84314",
		"0xa2aad1f8c462e13cc8eb3fe146563ec38f8cbe5f3e69ec989db77fe839fde1e8ddc3c3959da815046de044dbca99b0f8",
		"0xaa4d91f1e636a713670e6b9111bd0ad3cf0854b54e69c3df7d32784f3166045439d715e8764b6550a0fcc5af266bafb3",
		"0x90c62e953a88a7c84d3747b550fcf90705cff80a452de47391c6bc4e4755cdb4a41d67647cffc5edc4ae34a9c7e9ffcb",
		"0xb384774993b4b7d53cf1b1d2d65f6b60bfc540fcb0c585a88938b286f1423c07bb37a1006fc66a2e5e5c019e6e910076",
		"0x965972fe66ff287c3974638a7e4a92525c9f3ae0b84d2b53fad922e86aeb0bbbc0368f62ae1f93bec293e0c0a1a87b33",
		"0xb9d242a2b2bd2f210d0bb371de23d31b9fc29221b19b0f5ddf4265e58a6f7e053003f5b7d73917015dd67234c940025f",
		"0x85be54d6fa3cb9b3a61fd31c76f330b41f075f73774ad4e4144316c32f7c3c4675cd0ce528038c9e94b1956df9adff6e",
		"0x85ccbc3bfcde735347c583e331564b1ae21ea79ab9b2b67efbbe21ed0ad1de82350d7eafc28848d9cc80b8cc0a1048b9",
		"0x8d6a4bf122558476eaf7fa3e0a44d31b6a9fd190c619e8a5f52f8610df9ed063ebf663b1fa14699d12568d17203e2e6e",
		"0xa6dffa42dc34fe0570b265ffa1113c7f45211a823d30817540b215ea61e9141871100928e6f0007eba7a8ca38b3fb391",
		"0x935b955fc37608b5078195ec7e2b67e971cd8579c448b1939736af72d6bef5c56e1bab2c6238015bf382de507624955e",
		"0x95f33e58a140991274734f62bb0d22805c8c6f5422361808ba271b7ddbf908b279e7a65eac3850195194cd9f589f99c0",
		"0x8b2e4ae3defb2a67252832e158bbf1df1927daf46ac80849448d440505edb8261e87ecba03ade03d6e9f0a5dd91ea8a6",
		"0xa1425e840e0aa1b05b6fb6b96eb9fc6d1cd8687aacb8660df158c4d7368a069da0b48cf758f9a3ed62342591a4ad6f6e",
		"0x8a10af23eb118dcba83b9b9ae155ae7319d948e5fbba614a1e1c9e407d327ea80c0ac8f22ae9ec76085238349ec34fd4",
		"0xa28a9e3ccc2fdb9de9eb044104d3e1e999c1a7e0e13840a5e5823b48ad5f4d40c6bd1001c65c91e91d3cc5a769a274bd",
		"0xa67d55751c734ba4dd8dfa498316a5176572fbcc9784a5f78754b7fcfc02da9c5578863042a1a9caaa69273eadaabb44",
		"0xaff8b4c7d2255928cf8277575977ed037275e6387e37e22dfbf31d299cf67bcaf997e3b8c44d9ca52fe17d6103e6ef67",
		"0x851100345117fc3c00176f3b52a410f1755fbbd09cdbb043a8bbff9a4d68aafe7b42ab24a5e3a56daa908ff0bafb5c38",
		"0x88ab6cdb970e9862db38e572c7174300f546d532a742be8aa78da67c4665e7aa78010f0edfc32b7e5c41e021c2519608",
		"0x93bb46641586dd519a78edb3f6991bd6be0bc9ddd64ec307ecd4faaae6c853fd845e073bdaddf28c519cca7df58f3412",
		"0xb4a3b26b7a24c297f0abb795fcfefaf3b41eda63867d3e7f4a36a51fa8fa71bebd3253f22c5d81802e51b1f78512b2b3",
		"0xb4f8d70684021bdb597d62fd29c8ed17cfa22a701a15f5f1ec755aabb4ec0f4defff3595947a467594b7d75e146e5478",
		"0x916c2007dbb0de7c20dfe2f509c6cf49965b44a361034be542e0a1f234cb083d2c474317f0e93160c98ad9c9ed45e4af",
		"0xaeb82459f87e58f5e94e217629961f594c88f1f9d23c0e57c4e97621697037275d0bfe044a8530a6ea818814b0c5accb",
		"0x98e61b925adcd48d49c7a9820b463810320f411e2b9711d509c5b1d7bf3ed33c0cd72cba56bbe7e35d74c834267c60b1",
		"0x95a0b5af82bc133212bef1416b9c828c2539404cc243a2bdcf626e11200cc06cc139c117faa791ba117e0bda8c2d2a85",
		"0xb6575745d3b3bd12a57f95c8a61433eeea3db1c1d026880b1f1a1e59b12bd8e5924152b55abe20e093bf11a947b32d60",
		"0xa372b366822a324a6bf7a441f94b869bd8af5cbd72fa88429a4606c76fbcd5c3460dc918702187e35ce870f945b87a90",
		"0x968e30d6a24918ec6c4618eeb39a4b3645d3efa26ae6f4d16c56ee5c08a67fab96f2ada5e16a94eb20e4b3f3d8fc23da",
		"0xb6e2b532e30cf3ae8859d673df4cd2b1847c2add10743b48933d03a2d5558ddc1066584669747c6a5c612c865bf02194",
		"0xb2d3624c250d7b6a4b6d44734a6f54a13269d26cd4bffab1831d76c2775cab45f24c50e2f416692dd6837f8f63efbac4",
		"0x89e532f862611b855ca1abe9119bbdc9b9e0d40cac51fb7d95e81da1e7505145061f4eed8fb42f3988d16b334f27e2b6",
		"0xb90e95d4ff451460d2f84c402864c58cc2bc31598d23a51ad68a4fdfd6a6d9cd170a8a874ab254621a324a0ef105e7d3",
		"0xa239c4b9c472be7a208a536a825c6673eaa4e21247d7220d93eafb1075a35f726bd9f07292bac535b41ff43ab18452c0",
		"0xb4e6be39326fec50a737567882e0b56f7f7afa02a141fc9fc7c278736ceeede21d66ba3c2b5f5881db632ea49f88b2ad",
		"0xac9cadcf329ba91f5168137efbe64c4a4d81eccc237d36f925bf208b89fd6498ccba2ad1fc37da440a7896cff6fe14b0",
		"0x861b23434ace8dc8ac0174cad2499db8f7fbb677bad1c61dd29458cd024d6f0a7a927a11d30e6f43e5412fb9ad178325",
		"0x8fc53026b3f7801a6777d304b5ccd9a8548821ccdb8e1ca59ca01c6031162107294fbad08ef62a29defa22b7af23fe50",
		"0xaa559e29a3d1ba052fb15d242857428c81bb37b3ab6b07d4603abefa2750fa2cfe2498db2a97521a1acc5aa6388d7388",
		"0xb1d550d3d7286d4e0feca58b7f6585c53cf904b4be65979ba8197f5d145430ca3fd4d49edc05589e355f65eabf210882",
		"0x82a948dadbf4c0372ae04d91ebac4a71a7d38be839a804f876d57fb82117eec1295c00f970b957f469b47b73df0eae4c",
		"0xb3a1cdb7ccf8ef581e369925a760127798766e88133ba9128d5160e0dde41b3a7d75ff89733eaf899abc82d6e16d0dd9",
		"0x8d27a37b3ef25128032f86b0a8ca51e3805f728eaafaec4498496cf835ca8541d099a2947299460c570fd27d7ce6ea25",
		"0xa0556b7204fc5f91ce0a07a687bc214bc770e19c9df64430dac67d049a9a4b28f7d8c0f3b697ff2e5e0d7ce6971795d2",
		"0x949e4ca9c2b4c05fb1c85a5cfa8ef8dab8394cb18797ae8f22c67d928c7a05929018e6de95e7d94fe5d52ae0884337a4",
		"0x870dabf23bc17491bcd3fb4933d2e97e3d929b2ba8c27307aca2274d7fd902c2398d9375350a5fb11ae221a11b5b2933",
		"0xa0be815d75d24b7ee89efecae8ac88aa990d00894b57c6587ce0a4c9a97b8f68e5a4478a2fc11c1832c5978341be30ba",
		"0x806e7281239f30d92b9f0d6fe3dc37d4a867fa49b622b2c695f5dba4e9ed928d8ac1c2aaaea671da518bb02231faa6cf",
		"0x8321217a517514e7f4b0f7a5b644d93ce7ac933662b5b8abd69df029ade5f761d5e9fc877a7557644c147564e76bc424",
		"0x97a4a22dde2747dfaf0f86033a68a661e9892d370b63fc18419be736a3e210cbd3055332fdc90575989a00775b5c44f1",
		"0xa6dd2c4c9c2d19ab2b5b9efa8a1587d02f1afbbcb99911f8c324a5527e5c50f0629f72843864854acb97b2b36cb87a85",
		"0x96daaad9ab48d09bfbd00e938995a55182d0f8ac6377069565ae829bf06490de66b8d76aade13fca839aef17397bd703",
		"0x89e6681d952e4c70286dc1ec347cad7534c3ea0aab540ef7880f7740fb369a4502b96346bccf39a740521f09b27ad3a3",
		"0xa4859ca356e3346c131560497ad4a589dd5bc3d9f27c4cadf02bf2c4bd0a3a711ee6ab76088d543c2abe69a8c0dc35ad",
		"0xaf0db2cd9613b472840977c56b508271599603232838259803d7be24a0d57133d2192ba75dc9740ef51d4770499125d4",
		"0x9690e854e5d308dffa69fb5cb82d971f8b1530fd1b48349b6ac4c4931e0b26bde4a3c529ffa5b02717b1bd397ed4763d",
		"0xab92a962df22342425dc6f23dc6970fb010deb6e40174d80a1718fe23c97b894a0124e62edd0812a1a7d23e3c65cbba5",
		"0xb4280d4f2cf1d5d8df6559867e4dba736d531cc11bc4501692fad30523b1ab58802053dfcfbb90f96c976dbc52db3150",
		"0xa452104ca88aaddb239f46d9225ef12ce00e74455c8b79699f3ef3443b829abad5b1a3ef958a50e818ca75f249e456ea",
		"0xb29774a7c2fdaafcfe84b57d8f303673993ffbae72fba9c11ad0d633cbd8ba5df74cfb2a405928e1a324b9950df42481",
		"0xb7a78fdb7330c6891ce0ff30f34542f92d1afbdd615742bfaea78346055b799066310f84cd6922ef61bd328804e08d7c",
		"0xa166f297678513fba085e6e4e0be82d43425ae8cb4c0283a3f8261ca7a463ede8ffaf9f4fa02c803d2bc0ef8bbe68b40",
		"0xb180920b216c07378b5b3859a86f030f1d545053d0fc8009d818051d038e438d831f513549b48f7cc297f7ef5ac66464",
		"0xacf83d14651b5ccea61a10ed582bae1c5a8524a7303068cadb51ee59283711199d9a03a9e9a3acbdb2f1cce624323b72",
		"0x80e735cf51dd2ea2fa8cb9731d79aad27f5f7a09a02477535a58479f190055b3695e6e00331408aad38abccd8eef52d1",
		"0x8314c8ff21c588b47338cfd9a1ba734d2bcda2b5f86b7f21cc3f7a7ebdffadf8514af782c9ac86e1680654e1cf0bd379",
		"0x8d884afc0a88870156b116463dba256cc36c1d4649506442b877dcc187955261d84cdf34a59e27c4ea9eccfba5b64c0b",
		"0xae51b44a8d55009b7a204d4c06005fd3cac93fd2a55ba992e23813b74d03e8fb7caf00c6faf43708dba42c6cc0192717",
		"0xa8a3390ebd72f594f8a46acce7d0608b1d49fa06e1e0f18a88f6b534c5fc776dbc5a3b7c830819e353317f6cd23f0caa",
		"0xb6a179773e5845f5ca1642d040660c25d3fa9f9e9eb88d004a1e0ee002874605a8353b7001e9e4fadec15b5a7401f5d1",
		"0xb9ea88225480050ea8c8d6511a94b2db1cae51444959ba8bbfe75dea1c144d991ef7d451cd408ba421be2e01a5299f76",
		"0x951289b71eff4e43a9a0b84ed0b29c012a7b052b43e18c7f1af99938dad0c51307dd9a90b470cd0ab81f1ef7e522bdfc",
		"0xb0f370a32034abbbab93c2703fa328c7cb74ad026ff7c15e0fe7ca8526dab690e5c21de7f6ffb0a2357963c18835b48b",
		"0xb97a6d588bc1b8d84c79f703405f51b3eb949f9ee402b9779b26b6b685515e7633b389bfadd547203b2a57b2dd0e86a3",
		"0x8e715e5cdcc3d7d7984dd6de159e168c8dc41d1dc0da14dfe2496c5e13863f2cbb99e0f5b492ee87d8125682dea54d7c",
		"0x95d432d4ed50bee3e27b7051234490d90072c1bc87775a3105f9214f94339c7ecd269cb4bfaa0b5192eea7f1bbb68207",
		"0xb10f7a7bd67be8fd3893589ed4bbbcb3e3c8d7043c692b5743cda53272a06b07005a8b48d1f14bfb5b9b3602ac08096a",
		"0x9655693e0fc092a1246783c73683216ab61ceda5ea3da513c9301bcd6809edb0f6e359a40561c50d19732ab73ce53de3",
		"0xa1b5b86d939d727b2b91b28064e225aa1e923ec7e63cf2b2607c6e7fbdf37de3c83cb536d4009d4a4ba96b2b1cba0dea",
		"0x90173c252dcf9544951bbd8af6ac2b3b5056aa6b5d63ae3d6d9409234052e5929125092a7ee7ecafe9042ebeaa5167a7",
		"0xa5e92dfb8393c4a575c37c426dc1d3a945aa5393390b279f787debfb18521ee25ad2a4ff9a849879cf97c240900a20b2",
		"0x86e03cf51eb0ef6740689f9cb6f6e7d6cf30a6773949fdd4733ad1395a0969ac5f787f4b2e6e8ac121225dc33eb889c4",
		"0xa400f90ab901c131964a1ec5877f9d2a2dc9cfc66ea9efb8331873362e568841ec8b5d5c3130821283dff9a8ff5aa2b8",
		"0x960088216cd88415b235b27b2b55252199e89ecea022a5061cace3ddf117157bad5f641ebfdb613179b5b73b43ade5d4",
		"0x9640705f30da122f138a49ee5bf76a0f8724fdc98f986689d4aaab16b297e60c024467f4d4f955d967bf61a4db1efe3a",
		"0xa1aadf656ca0676e1cc0cc12a4154e6cb5e2f4ecb4839da36d999fe30bf034091c98f88be12ad955b60ab4b2a18bbd8a",
		"0xb6b92a0d9eca4399bbf5c201b20dbfe8651f39800ef69c11afbd350620235ee616afa6895b1db3dd463e9f6aab2aa323",
		"0xb16561bf9b0d2a681899c047f3ab83ab391faef2d3c63f793f8c2194cf1f71f4f691b2e15613b9fd3d45a907345f0663",
	}
	syncCom := make([][]byte, params.BeaconConfig().SyncCommitteeSize)
	for i := 0; uint64(i) < params.BeaconConfig().SyncCommitteeSize; i++ {
		pubkeyHex := listOfPubkeys[i]
		pubkeyHex = strings.TrimPrefix(pubkeyHex, "0x")
		rawBytes, err := hex.DecodeString(pubkeyHex)
		if err != nil {
			panic(err)
		}
		syncCom[i] = rawBytes
	}

	aggHex := "959f9bf225a62ecf9ca1f5aed0a921fd7113e57fadee671e37c6a3835f5bb7dc361cfc793cd87885361d6614ac76b628"
	rawBytesAgg, err := hex.DecodeString(aggHex)
	if err != nil {
		panic(err)
	}

	return &pb.SyncCommittee{
		Pubkeys:         syncCom,
		AggregatePubkey: rawBytesAgg,
	}
}
