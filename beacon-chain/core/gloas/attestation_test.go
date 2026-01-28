package gloas

import (
	"bytes"
	"testing"

	state_native "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func buildStateWithBlockRoots(t *testing.T, stateSlot primitives.Slot, roots map[primitives.Slot][]byte) *state_native.BeaconState {
	t.Helper()

	cfg := params.BeaconConfig()
	blockRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	for slot, root := range roots {
		blockRoots[slot%cfg.SlotsPerHistoricalRoot] = root
	}

	stProto := &ethpb.BeaconStateGloas{
		Slot:       stateSlot,
		BlockRoots: blockRoots,
	}

	state, err := state_native.InitializeFromProtoGloas(stProto)
	require.NoError(t, err)
	return state.(*state_native.BeaconState)
}

func TestMatchingPayload(t *testing.T) {
	t.Run("pre-gloas always true", func(t *testing.T) {
		stIface, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)

		ok, err := MatchingPayload(stIface, [32]byte{}, 0, 123)
		require.NoError(t, err)
		require.Equal(t, true, ok)
	})

	t.Run("same slot requires committee index 0", func(t *testing.T) {
		root := bytes.Repeat([]byte{0xAA}, 32)
		state := buildStateWithBlockRoots(t, 6, map[primitives.Slot][]byte{
			4: root,
			3: bytes.Repeat([]byte{0xBB}, 32),
		})

		var rootArr [32]byte
		copy(rootArr[:], root)

		ok, err := MatchingPayload(state, rootArr, 4, 1)
		require.ErrorContains(t, "committee index", err)
		require.Equal(t, false, ok)
	})

	t.Run("same slot matches when committee index is 0", func(t *testing.T) {
		root := bytes.Repeat([]byte{0xAA}, 32)
		state := buildStateWithBlockRoots(t, 6, map[primitives.Slot][]byte{
			4: root,
			3: bytes.Repeat([]byte{0xBB}, 32),
		})

		var rootArr [32]byte
		copy(rootArr[:], root)

		ok, err := MatchingPayload(state, rootArr, 4, 0)
		require.NoError(t, err)
		require.Equal(t, true, ok)
	})

	t.Run("non same slot checks payload availability", func(t *testing.T) {
		cfg := params.BeaconConfig()
		root := bytes.Repeat([]byte{0xAA}, 32)
		blockRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
		blockRoots[4%cfg.SlotsPerHistoricalRoot] = bytes.Repeat([]byte{0xCC}, 32)
		blockRoots[3%cfg.SlotsPerHistoricalRoot] = bytes.Repeat([]byte{0xBB}, 32)

		availability := make([]byte, cfg.SlotsPerHistoricalRoot/8)
		slotIndex := uint64(4)
		availability[slotIndex/8] = byte(1 << (slotIndex % 8))

		stIface, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Slot:                         6,
			BlockRoots:                   blockRoots,
			ExecutionPayloadAvailability: availability,
			Fork: &ethpb.Fork{
				CurrentVersion:  bytes.Repeat([]byte{0x66}, 4),
				PreviousVersion: bytes.Repeat([]byte{0x66}, 4),
				Epoch:           0,
			},
		})
		require.NoError(t, err)
		state := stIface.(*state_native.BeaconState)
		require.Equal(t, version.Gloas, state.Version())

		var rootArr [32]byte
		copy(rootArr[:], root)

		ok, err := MatchingPayload(state, rootArr, 4, 1)
		require.NoError(t, err)
		require.Equal(t, true, ok)

		ok, err = MatchingPayload(state, rootArr, 4, 0)
		require.NoError(t, err)
		require.Equal(t, false, ok)
	})
}
