package eth_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	v1alpha1 "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// Both presets must match the same field schema.
func TestBeaconStateAltair_FieldParity(t *testing.T) {
	want := []string{
		"GenesisTime uint64",
		"GenesisValidatorsRoot []uint8 ssz-size",
		"Slot primitives.Slot",
		"Fork *eth.Fork",
		"LatestBlockHeader *eth.BeaconBlockHeader",
		"BlockRoots [][]uint8 ssz-size",
		"StateRoots [][]uint8 ssz-size",
		"HistoricalRoots [][]uint8 ssz-size ssz-max",
		"Eth1Data *eth.Eth1Data",
		"Eth1DataVotes []*eth.Eth1Data ssz-max",
		"Eth1DepositIndex uint64",
		"Validators []*eth.Validator ssz-max",
		"Balances []uint64 ssz-max",
		"RandaoMixes [][]uint8 ssz-size",
		"Slashings []uint64 ssz-size",
		"PreviousEpochParticipation []uint8 ssz-max",
		"CurrentEpochParticipation []uint8 ssz-max",
		"JustificationBits bitfield.Bitvector4 ssz-size",
		"PreviousJustifiedCheckpoint *eth.Checkpoint",
		"CurrentJustifiedCheckpoint *eth.Checkpoint",
		"FinalizedCheckpoint *eth.Checkpoint",
		"InactivityScores []uint64 ssz-max",
		"CurrentSyncCommittee *eth.SyncCommittee",
		"NextSyncCommittee *eth.SyncCommittee",
	}
	assertStateFields(t, reflect.TypeFor[v1alpha1.BeaconStateAltair](), want)
}

func assertStateFields(t *testing.T, rt reflect.Type, want []string) {
	t.Helper()
	got := make([]string, 0, rt.NumField())
	for i := range rt.NumField() {
		f := rt.Field(i)
		parts := []string{f.Name, f.Type.String()}
		for _, key := range []string{"ssz-size", "ssz-max"} {
			if _, ok := f.Tag.Lookup(key); ok {
				parts = append(parts, key)
			}
		}
		got = append(got, strings.Join(parts, " "))
	}
	require.DeepEqual(t, want, got)
}

func TestBeaconStateAltair_Copy(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		require.Equal(t, (*v1alpha1.BeaconStateAltair)(nil), (*v1alpha1.BeaconStateAltair)(nil).Copy())
	})
	t.Run("nil sub-messages stay nil", func(t *testing.T) {
		cp := (&v1alpha1.BeaconStateAltair{}).Copy()
		require.DeepSSZEqual(t, &v1alpha1.BeaconStateAltair{}, cp)
		require.Equal(t, true, cp.Fork == nil)
	})
	t.Run("deep copy does not alias", func(t *testing.T) {
		orig := &v1alpha1.BeaconStateAltair{
			Slot:              7,
			Fork:              &v1alpha1.Fork{Epoch: 1, PreviousVersion: []byte{1}, CurrentVersion: []byte{2}},
			BlockRoots:        [][]byte{{1}, {2}},
			Eth1DataVotes:     []*v1alpha1.Eth1Data{{DepositCount: 3}},
			Validators:        []*v1alpha1.Validator{{EffectiveBalance: 32}},
			Balances:          []uint64{32},
			JustificationBits: bitfield.Bitvector4{0b1010},
		}
		cp := orig.Copy()
		require.DeepSSZEqual(t, orig, cp)

		cp.Fork.Epoch, cp.BlockRoots[0][0], cp.Eth1DataVotes[0].DepositCount = 9, 9, 9
		cp.Validators[0].EffectiveBalance, cp.Balances[0], cp.JustificationBits[0] = 9, 9, 9
		require.Equal(t, primitives.Epoch(1), orig.Fork.Epoch)
		require.Equal(t, byte(1), orig.BlockRoots[0][0])
		require.Equal(t, uint64(3), orig.Eth1DataVotes[0].DepositCount)
		require.Equal(t, uint64(32), orig.Validators[0].EffectiveBalance)
		require.Equal(t, uint64(32), orig.Balances[0])
		require.Equal(t, byte(0b1010), orig.JustificationBits[0])
	})
}

func TestBeaconStateBellatrix_Copy(t *testing.T) {
	orig := &v1alpha1.BeaconStateBellatrix{
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeader{ParentHash: []byte{1}},
	}
	cp := orig.Copy()
	require.DeepSSZEqual(t, orig, cp)
	cp.LatestExecutionPayloadHeader.ParentHash[0] = 2
	require.Equal(t, byte(1), orig.LatestExecutionPayloadHeader.ParentHash[0])
	require.Equal(t, (*v1alpha1.BeaconStateBellatrix)(nil), (*v1alpha1.BeaconStateBellatrix)(nil).Copy())
}

func TestBeaconStateBellatrix_FieldParity(t *testing.T) {
	assertStateFields(t, reflect.TypeFor[v1alpha1.BeaconStateBellatrix](), []string{
		"GenesisTime uint64",
		"GenesisValidatorsRoot []uint8 ssz-size",
		"Slot primitives.Slot",
		"Fork *eth.Fork",
		"LatestBlockHeader *eth.BeaconBlockHeader",
		"BlockRoots [][]uint8 ssz-size",
		"StateRoots [][]uint8 ssz-size",
		"HistoricalRoots [][]uint8 ssz-size ssz-max",
		"Eth1Data *eth.Eth1Data",
		"Eth1DataVotes []*eth.Eth1Data ssz-max",
		"Eth1DepositIndex uint64",
		"Validators []*eth.Validator ssz-max",
		"Balances []uint64 ssz-max",
		"RandaoMixes [][]uint8 ssz-size",
		"Slashings []uint64 ssz-size",
		"PreviousEpochParticipation []uint8 ssz-max",
		"CurrentEpochParticipation []uint8 ssz-max",
		"JustificationBits bitfield.Bitvector4 ssz-size",
		"PreviousJustifiedCheckpoint *eth.Checkpoint",
		"CurrentJustifiedCheckpoint *eth.Checkpoint",
		"FinalizedCheckpoint *eth.Checkpoint",
		"InactivityScores []uint64 ssz-max",
		"CurrentSyncCommittee *eth.SyncCommittee",
		"NextSyncCommittee *eth.SyncCommittee",
		"LatestExecutionPayloadHeader *enginev1.ExecutionPayloadHeader",
	})
}
