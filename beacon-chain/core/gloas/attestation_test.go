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

func buildStateWithAvailability(t *testing.T, stateSlot primitives.Slot, roots map[primitives.Slot][]byte, availableSlots ...primitives.Slot) *state_native.BeaconState {
	t.Helper()

	cfg := params.BeaconConfig()
	blockRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
	for slot, root := range roots {
		blockRoots[slot%cfg.SlotsPerHistoricalRoot] = root
	}

	availability := make([]byte, cfg.SlotsPerHistoricalRoot/8)
	for _, slot := range availableSlots {
		idx := uint64(slot % cfg.SlotsPerHistoricalRoot)
		availability[idx/8] |= byte(1 << (idx % 8))
	}

	stIface, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
		Slot:                         stateSlot,
		BlockRoots:                   blockRoots,
		ExecutionPayloadAvailability: availability,
		Fork: &ethpb.Fork{
			CurrentVersion:  bytes.Repeat([]byte{0x66}, 4),
			PreviousVersion: bytes.Repeat([]byte{0x66}, 4),
			Epoch:           0,
		},
	})
	require.NoError(t, err)
	st := stIface.(*state_native.BeaconState)
	require.Equal(t, version.Gloas, st.Version())
	return st
}

func TestMatchingPayload(t *testing.T) {
	t.Run("pre-gloas always true", func(t *testing.T) {
		stIface, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)

		ok, err := MatchingPayload(stIface, [32]byte{}, 0, 0, 123)
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

		ok, err := MatchingPayload(state, rootArr, 4, 3, 1)
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

		ok, err := MatchingPayload(state, rootArr, 4, 3, 0)
		require.NoError(t, err)
		require.Equal(t, true, ok)
	})

	roots := map[primitives.Slot][]byte{
		4: bytes.Repeat([]byte{0xCC}, 32),
		3: bytes.Repeat([]byte{0xBB}, 32),
	}
	var attestedRoot [32]byte
	copy(attestedRoot[:], bytes.Repeat([]byte{0xAA}, 32))

	t.Run("non same slot checks availability at the parent slot", func(t *testing.T) {
		// Slot 4 was skipped, the attested block at slot 3 revealed its payload.
		st := buildStateWithAvailability(t, 6, roots, 3)

		ok, err := MatchingPayload(st, attestedRoot, 4, 3, 1)
		require.NoError(t, err)
		require.Equal(t, true, ok)

		ok, err = MatchingPayload(st, attestedRoot, 4, 3, 0)
		require.NoError(t, err)
		require.Equal(t, false, ok)
	})

	t.Run("non same slot ignores availability at the data slot", func(t *testing.T) {
		// Only the skipped slot 4 is marked available, the attested block at slot 3 withheld its payload.
		st := buildStateWithAvailability(t, 6, roots, 4)

		ok, err := MatchingPayload(st, attestedRoot, 4, 3, 1)
		require.NoError(t, err)
		require.Equal(t, false, ok)

		ok, err = MatchingPayload(st, attestedRoot, 4, 3, 0)
		require.NoError(t, err)
		require.Equal(t, true, ok)
	})
}

func TestParentSlotFromBid(t *testing.T) {
	t.Run("pre-gloas returns zero", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoElectra(&ethpb.BeaconStateElectra{})
		require.NoError(t, err)

		parentSlot, err := ParentSlotFromBid(st)
		require.NoError(t, err)
		require.Equal(t, primitives.Slot(0), parentSlot)
	})

	t.Run("returns the cached bid slot", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoGloas(&ethpb.BeaconStateGloas{
			Slot: 6,
			LatestExecutionPayloadBid: &ethpb.ExecutionPayloadBid{
				Slot:            3,
				ParentBlockHash: make([]byte, 32),
				ParentBlockRoot: make([]byte, 32),
				BlockHash:       make([]byte, 32),
				PrevRandao:      make([]byte, 32),
				FeeRecipient:    make([]byte, 20),
			},
		})
		require.NoError(t, err)

		parentSlot, err := ParentSlotFromBid(st)
		require.NoError(t, err)
		require.Equal(t, primitives.Slot(3), parentSlot)
	})
}
