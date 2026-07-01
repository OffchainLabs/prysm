package transition_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/transition"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

func TestTrailingSlotState_RoundTrip(t *testing.T) {
	ctx := t.Context()
	r := []byte{'a'}
	s := transition.NextSlotState(r, 0)
	require.Equal(t, nil, s)

	s, _ = util.DeterministicGenesisState(t, 1)
	require.NoError(t, transition.UpdateNextSlotCache(ctx, r, s))
	s = transition.NextSlotState(r, 1)
	require.Equal(t, primitives.Slot(1), s.Slot())

	lastRoot, lastState := transition.LastCachedState()
	require.DeepEqual(t, r, lastRoot)
	require.Equal(t, s.Slot(), lastState.Slot())

	require.NoError(t, transition.UpdateNextSlotCache(ctx, r, s))
	s = transition.NextSlotState(r, 2)
	require.Equal(t, primitives.Slot(2), s.Slot())

	lastRoot, lastState = transition.LastCachedState()
	require.DeepEqual(t, r, lastRoot)
	require.Equal(t, s.Slot(), lastState.Slot())
}

func TestTrailingSlotStateReadOnly_RoundTrip(t *testing.T) {
	ctx := t.Context()
	r := []byte{'a'}
	require.Equal(t, nil, transition.NextSlotStateReadOnly(r, 0))

	s, _ := util.DeterministicGenesisState(t, 1)
	require.NoError(t, transition.UpdateNextSlotCache(ctx, r, s))
	ro := transition.NextSlotStateReadOnly(r, 1)
	require.NotNil(t, ro)
	require.Equal(t, primitives.Slot(1), ro.Slot())

	// Cache a second root so the first becomes prevRoot, and confirm both still hit.
	r2 := []byte{'b'}
	s2, _ := util.DeterministicGenesisState(t, 1)
	require.NoError(t, transition.UpdateNextSlotCache(ctx, r2, s2))
	ro = transition.NextSlotStateReadOnly(r2, 1)
	require.NotNil(t, ro)
	require.Equal(t, primitives.Slot(1), ro.Slot())
	ro = transition.NextSlotStateReadOnly(r, 1)
	require.NotNil(t, ro)
	require.Equal(t, primitives.Slot(1), ro.Slot())
}

func TestTrailingSlotStateReadOnly_StateAdvancedBeyondRequest(t *testing.T) {
	ctx := t.Context()
	r := []byte{'a'}
	require.Equal(t, nil, transition.NextSlotStateReadOnly(r, 0))

	s, _ := util.DeterministicGenesisState(t, 1)
	assert.NoError(t, s.SetSlot(2))
	require.NoError(t, transition.UpdateNextSlotCache(ctx, r, s))
	require.Equal(t, nil, transition.NextSlotStateReadOnly(r, 1))
}

func TestTrailingSlotState_StateAdvancedBeyondRequest(t *testing.T) {
	ctx := t.Context()
	r := []byte{'a'}
	s := transition.NextSlotState(r, 0)
	require.Equal(t, nil, s)

	s, _ = util.DeterministicGenesisState(t, 1)
	assert.NoError(t, s.SetSlot(2))
	require.NoError(t, transition.UpdateNextSlotCache(ctx, r, s))
	s = transition.NextSlotState(r, 1)
	require.Equal(t, nil, s)
}
