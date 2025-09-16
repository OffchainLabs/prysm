package sync

import (
	"sync"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v6/testing/require"
)

func TestNewToggle(t *testing.T) {
	ctx := t.Context()
	toggler := NewServiceToggler()
	require.Equal(t, NilService, toggler.current)
	require.Equal(t, 0, len(toggler.blocked))

	start := time.Now()
	require.NoError(t, toggler.Acquire(ctx, ToggleGroupRangeSync))
	require.Equal(t, true, time.Since(start) < time.Millisecond)
	ordered := make([]int, 0)
	go func() {
		<-time.After(2 * time.Millisecond)
		ordered = append(ordered, 1)
		toggler.Release(ToggleGroupRangeSync)
	}()
	require.NoError(t, toggler.Acquire(ctx, ToggleGroupBackfill))
	ordered = append(ordered, 2)
	// This assertion ensures that the Acquire call above blocked until the Release call in the goroutine.
	require.DeepEqual(t, []int{1, 2}, ordered)
}

func TestToggleSequence(t *testing.T) {
	ctx := t.Context()
	toggler := NewServiceToggler()
	require.NoError(t, toggler.Acquire(ctx, ToggleGroupRangeSync))
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		wg.Done()
		require.NoError(t, toggler.Acquire(ctx, ToggleGroupBackfill))
	}()
	go func() {
		wg.Done()
		require.NoError(t, toggler.Acquire(ctx, ToggleGroupBackfill))
	}()
	wg.Wait()
	<-time.After(1 * time.Millisecond)
	require.Equal(t, ToggleGroupRangeSync, toggler.current)
	require.Equal(t, 1, toggler.active)
	require.Equal(t, 2, len(toggler.blocked))
	toggler.Release(ToggleGroupRangeSync)
	require.Equal(t, ToggleGroupBackfill, toggler.current)
	require.Equal(t, 2, toggler.active)
	require.Equal(t, 0, len(toggler.blocked))
	toggler.Release(ToggleGroupBackfill)
	require.Equal(t, ToggleGroupBackfill, toggler.current)
	require.Equal(t, 1, toggler.active)
	require.Equal(t, 0, len(toggler.blocked))
	toggler.Release(ToggleGroupBackfill)
	require.Equal(t, ToggleGroupBackfill, toggler.current)
	require.Equal(t, 0, toggler.active)
	require.Equal(t, 0, len(toggler.blocked))
}
