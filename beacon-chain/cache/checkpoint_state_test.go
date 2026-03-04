package cache_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
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

func TestCheckpointStateCache_EvictFinalized_FinalizedEntry(t *testing.T) {
	c := cache.NewCheckpointStateCache()

	cp := &ethpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'A'}, 32)}
	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: 32})
	require.NoError(t, err)
	require.NoError(t, c.AddCheckpointState(cp, st))

	evicted := c.EvictFinalized(1)
	assert.Equal(t, 1, evicted, "expected finalized entry to be evicted")

	s, err := c.StateByCheckpoint(cp)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), s, "expected cache to be empty after eviction")
}

func TestCheckpointStateCache_EvictFinalized_NotFinalizedEntry(t *testing.T) {
	c := cache.NewCheckpointStateCache()

	cp := &ethpb.Checkpoint{Epoch: 5, Root: bytesutil.PadTo([]byte{'A'}, 32)}
	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: 160})
	require.NoError(t, err)
	require.NoError(t, c.AddCheckpointState(cp, st))

	evicted := c.EvictFinalized(3)
	assert.Equal(t, 0, evicted, "expected non-finalized entry NOT to be evicted")

	s, err := c.StateByCheckpoint(cp)
	require.NoError(t, err)
	assert.NotNil(t, s, "expected entry to still be in cache")
}

func TestCheckpointStateCache_EvictFinalized_Mixed(t *testing.T) {
	c := cache.NewCheckpointStateCache()

	cp1 := &ethpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'A'}, 32)}
	st1, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: 32})
	require.NoError(t, err)

	cp2 := &ethpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{'B'}, 32)}
	st2, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: 64})
	require.NoError(t, err)

	cp5 := &ethpb.Checkpoint{Epoch: 5, Root: bytesutil.PadTo([]byte{'C'}, 32)}
	st5, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{Slot: 160})
	require.NoError(t, err)

	require.NoError(t, c.AddCheckpointState(cp1, st1))
	require.NoError(t, c.AddCheckpointState(cp2, st2))
	require.NoError(t, c.AddCheckpointState(cp5, st5))

	evicted := c.EvictFinalized(3)
	assert.Equal(t, 2, evicted, "expected epochs 1 and 2 to be evicted")

	s, err := c.StateByCheckpoint(cp1)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), s, "expected cp1 to be evicted")

	s, err = c.StateByCheckpoint(cp2)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), s, "expected cp2 to be evicted")

	s, err = c.StateByCheckpoint(cp5)
	require.NoError(t, err)
	assert.NotNil(t, s, "expected cp5 to still be in cache")
}

func TestCheckpointStateCache_EvictFinalized_EmptyCache(t *testing.T) {
	c := cache.NewCheckpointStateCache()
	evicted := c.EvictFinalized(0)
	assert.Equal(t, 0, evicted, "expected no eviction from empty cache")
}
