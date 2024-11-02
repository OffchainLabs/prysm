package lightclient

import (
	"strings"
	"testing"

	lightclient "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/light-client"
	fieldparams "github.com/prysmaticlabs/prysm/v5/config/fieldparams"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	light_client "github.com/prysmaticlabs/prysm/v5/consensus-types/light-client"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	pb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
)

// When the update has relevant sync committee
func createNonEmptySyncCommitteeBranch() [][]byte {
	res := make([][]byte, fieldparams.SyncCommitteeBranchDepth)
	res[0] = []byte(strings.Repeat("x", 32))
	for i := 1; i < len(res); i++ {
		res[i] = make([]byte, fieldparams.RootLength)
	}
	return res
}

// When the update has finality
func createNonEmptyFinalityBranch() [][]byte {
	res := make([][]byte, fieldparams.FinalityBranchDepth)
	res[0] = []byte(strings.Repeat("x", 32))
	for i := 1; i < fieldparams.FinalityBranchDepth; i++ {
		res[i] = make([]byte, 32)
	}
	return res
}

func TestIsBetterUpdate(t *testing.T) {
	config := params.BeaconConfig()
	st, err := util.NewBeaconStateAltair()
	require.NoError(t, err)

	t.Run("new has supermajority but old doesn't", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,0,1,1,1,1,1,0]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b11111100, 0b1}, // [0,0,1,1,1,1,1,1]
		})

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("old has supermajority but new doesn't", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b11111100, 0b1}, // [0,0,1,1,1,1,1,1]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,0,1,1,1,1,1,0]
		})

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("new doesn't have supermajority and newNumActiveParticipants is greater than oldNumActiveParticipants", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,0,1,1,1,1,1,0]
		})

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("new doesn't have supermajority and newNumActiveParticipants is lesser than oldNumActiveParticipants", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,0,1,1,1,1,1,0]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("new has relevant sync committee but old doesn't", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		assert.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000001,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		assert.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		newUpdate.SetSignatureSlot(1000000)

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("old has relevant sync committee but new doesn't", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000001,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		assert.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		oldUpdate.SetSignatureSlot(1000000)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		assert.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("new has finality but old doesn't", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		assert.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		assert.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("old has finality but new doesn't", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		assert.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		assert.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("new has finality and sync committee finality both but old doesn't have sync committee finality", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		assert.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		assert.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,0,1,1,1,1,1,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		assert.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		newUpdate.SetSignatureSlot(999999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 999999,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		assert.NoError(t, err)

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("new has finality but doesn't have sync committee finality and old has sync committee finality", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		assert.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		oldUpdate.SetSignatureSlot(999999)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 999999,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		assert.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		assert.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		assert.NoError(t, err)

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("new has more active participants than old", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,1,1,1,1,1,0,0]
		})

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("new has less active participants than old", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b01111100, 0b1}, // [0,1,1,1,1,1,0,0]
		})
		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("new's attested header's slot is lesser than old's attested header's slot", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		assert.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		assert.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 999999,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		assert.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		assert.NoError(t, err)

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("new's attested header's slot is greater than old's attested header's slot", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 999999,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		assert.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		assert.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		assert.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		assert.NoError(t, err)

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, false, result)
	})

	t.Run("none of the above conditions are met and new signature's slot is less than old signature's slot", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		assert.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		oldUpdate.SetSignatureSlot(9999)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		assert.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		assert.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		newUpdate.SetSignatureSlot(9998)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		assert.NoError(t, err)

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, true, result)
	})

	t.Run("none of the above conditions are met and new signature's slot is greater than old signature's slot", func(t *testing.T) {
		oldUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(1), st)
		assert.NoError(t, err)
		newUpdate, err := lightclient.CreateDefaultLightClientUpdate(primitives.Slot(config.AltairForkEpoch*primitives.Epoch(config.SlotsPerEpoch)).Add(2), st)
		assert.NoError(t, err)

		oldUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		oldAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetAttestedHeader(oldAttestedHeader)
		assert.NoError(t, err)
		err = oldUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		err = oldUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		oldUpdate.SetSignatureSlot(9998)
		oldFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		assert.NoError(t, err)
		err = oldUpdate.SetFinalizedHeader(oldFinalizedHeader)
		assert.NoError(t, err)

		newUpdate.SetSyncAggregate(&pb.SyncAggregate{
			SyncCommitteeBits: []byte{0b00111100, 0b1}, // [0,0,1,1,1,1,0,0]
		})
		newAttestedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 1000000,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetAttestedHeader(newAttestedHeader)
		assert.NoError(t, err)
		err = newUpdate.SetNextSyncCommitteeBranch(createNonEmptySyncCommitteeBranch())
		assert.NoError(t, err)
		newUpdate.SetSignatureSlot(9999)
		err = newUpdate.SetFinalityBranch(createNonEmptyFinalityBranch())
		assert.NoError(t, err)
		newFinalizedHeader, err := light_client.NewWrappedHeader(&pb.LightClientHeaderAltair{
			Beacon: &pb.BeaconBlockHeader{
				Slot: 9999,
			},
		})
		assert.NoError(t, err)
		err = newUpdate.SetFinalizedHeader(newFinalizedHeader)
		assert.NoError(t, err)

		result, err := IsBetterUpdate(newUpdate, oldUpdate)
		assert.NoError(t, err)
		assert.Equal(t, false, result)
	})
}
