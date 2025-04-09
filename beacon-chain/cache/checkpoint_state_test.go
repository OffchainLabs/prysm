package cache_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"google.golang.org/protobuf/proto"
)

func TestCheckpointStateCache_StateByCheckpoint(t *testing.T) {
	cache := cache.NewCheckpointStateCache()

	cp1 := &ethpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'A'}, 32)}
	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		GenesisValidatorsRoot: params.BeaconConfig().ZeroHash[:],
		Slot:                  64,
	})
	require.NoError(t, err)

	s, err := cache.StateByCheckpoint(cp1)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), s, "Expected state not to exist in empty cache")

	require.NoError(t, cache.AddCheckpointState(cp1, st))

	s, err = cache.StateByCheckpoint(cp1)
	require.NoError(t, err)

	pbState1, err := state_native.ProtobufBeaconStatePhase0(s.ToProtoUnsafe())
	require.NoError(t, err)
	pbstate, err := state_native.ProtobufBeaconStatePhase0(st.ToProtoUnsafe())
	require.NoError(t, err)
	if !proto.Equal(pbState1, pbstate) {
		t.Error("incorrectly cached state")
	}

	cp2 := &ethpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{'B'}, 32)}
	st2, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot: 128,
	})
	require.NoError(t, err)
	require.NoError(t, cache.AddCheckpointState(cp2, st2))

	s, err = cache.StateByCheckpoint(cp2)
	require.NoError(t, err)
	assert.DeepEqual(t, st2.ToProto(), s.ToProto(), "incorrectly cached state")

	s, err = cache.StateByCheckpoint(cp1)
	require.NoError(t, err)
	assert.DeepEqual(t, st.ToProto(), s.ToProto(), "incorrectly cached state")
}

func TestCheckpointStateCache_MaxSize(t *testing.T) {
	c := cache.NewCheckpointStateCache()
	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot: 0,
	})
	require.NoError(t, err)

	for i := uint64(0); i < uint64(cache.MaxCheckpointStateSize()+100); i++ {
		require.NoError(t, st.SetSlot(primitives.Slot(i)))
		require.NoError(t, c.AddCheckpointState(&ethpb.Checkpoint{Epoch: primitives.Epoch(i), Root: make([]byte, 32)}, st))
	}

	assert.Equal(t, cache.MaxCheckpointStateSize(), len(c.Cache().Keys()))
}
