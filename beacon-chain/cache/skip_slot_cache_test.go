package cache_test

import (
	"context"
	"sync"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/cache"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	state_native "github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func TestSkipSlotCache_RoundTrip(t *testing.T) {
	ctx := context.Background()
	c := cache.NewSkipSlotCache()

	r := [32]byte{'a'}
	s, err := c.Get(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), s, "Empty cache returned an object")

	require.NoError(t, c.MarkInProgress(r))

	s, err = state_native.InitializeFromProtoPhase0(&ethpb.BeaconState{
		Slot: 10,
	})
	require.NoError(t, err)

	c.Put(ctx, r, s)
	c.MarkNotInProgress(r)

	res, err := c.Get(ctx, r)
	require.NoError(t, err)
	assert.DeepEqual(t, res.ToProto(), s.ToProto(), "Expected equal protos to return from cache")
}

func TestSkipSlotCache_DisabledAndEnabled(t *testing.T) {
	ctx := context.Background()
	c := cache.NewSkipSlotCache()

	r := [32]byte{'a'}
	c.Disable()

	require.NoError(t, c.MarkInProgress(r))

	c.Enable()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		// Get call will only terminate when
		// it is not longer in progress.
		obj, err := c.Get(ctx, r)
		require.NoError(t, err)
		require.IsNil(t, obj)
		wg.Done()
	}()

	c.MarkNotInProgress(r)
	wg.Wait()
}
