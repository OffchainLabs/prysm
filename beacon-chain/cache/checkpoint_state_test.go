package cache_test

import (
	"testing"
	"time"

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

func TestCheckpointStateCache_Expiration(t *testing.T) {
	c := cache.NewCheckpointStateCache()
	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot: 0,
	})
	require.NoError(t, err)

	cp1 := &ethpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'A'}, 32)}
	cp2 := &ethpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{'B'}, 32)}
	cp3 := &ethpb.Checkpoint{Epoch: 3, Root: bytesutil.PadTo([]byte{'C'}, 32)}

	// Expand so all 3 entries fit without LRU eviction.
	c.ExpandCheckpointStateCache()

	require.NoError(t, c.AddCheckpointState(cp1, st))
	require.NoError(t, c.AddCheckpointState(cp2, st))
	require.NoError(t, c.AddCheckpointState(cp3, st))
	assert.Equal(t, 3, len(c.Cache().Keys()))

	// All entries are fresh — Get still works.
	s, err := c.StateByCheckpoint(cp1)
	require.NoError(t, err)
	assert.NotNil(t, s)

	// Backdate cp1 and cp2 past the TTL.
	ttl := cache.CheckpointStateTTL()
	for _, key := range c.Cache().Keys() {
		c.BackdateEntry(key, ttl+time.Second)
	}
	// Re-add cp3 so it's fresh.
	require.NoError(t, c.AddCheckpointState(cp3, st))

	// Now only cp3 should remain — the Add of cp3 swept the expired cp1 and cp2.
	assert.Equal(t, 1, len(c.Cache().Keys()))

	// cp1 is gone.
	s, err = c.StateByCheckpoint(cp1)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), s)

	// cp3 is still present.
	s, err = c.StateByCheckpoint(cp3)
	require.NoError(t, err)
	assert.NotNil(t, s)
}

func TestCheckpointStateCache_ExpandAndCompress(t *testing.T) {
	c := cache.NewCheckpointStateCache()
	st, err := state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot: 0,
	})
	require.NoError(t, err)

	// Fill to default size.
	for i := uint64(0); i < uint64(cache.MaxCheckpointStateSize()+5); i++ {
		require.NoError(t, st.SetSlot(primitives.Slot(i)))
		require.NoError(t, c.AddCheckpointState(&ethpb.Checkpoint{Epoch: primitives.Epoch(i), Root: bytesutil.PadTo([]byte{byte(i)}, 32)}, st))
	}
	assert.Equal(t, cache.MaxCheckpointStateSize(), len(c.Cache().Keys()))

	// Expand.
	c.ExpandCheckpointStateCache()

	// Fill to expanded size.
	for i := uint64(100); i < 100+uint64(cache.ExpandedCheckpointStateSize()); i++ {
		require.NoError(t, st.SetSlot(primitives.Slot(i)))
		require.NoError(t, c.AddCheckpointState(&ethpb.Checkpoint{Epoch: primitives.Epoch(i), Root: bytesutil.PadTo([]byte{byte(i)}, 32)}, st))
	}
	assert.Equal(t, cache.ExpandedCheckpointStateSize(), len(c.Cache().Keys()))

	// Compress should evict down to default.
	c.CompressCheckpointStateCache()
	assert.Equal(t, cache.MaxCheckpointStateSize(), len(c.Cache().Keys()))
}
