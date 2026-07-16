package jobs

import (
	"errors"
	"sync"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tracker, err := r.Register("backfill")
	require.NoError(t, err)
	require.NotNil(t, tracker)

	j, ok := r.Get("backfill")
	require.Equal(t, true, ok)
	require.Equal(t, "backfill", j.ID)
	require.Equal(t, StatusPending, j.Status)
	require.Equal(t, false, j.UpdatedAt.IsZero())
	require.Equal(t, true, j.StartedAt.IsZero())
	require.Equal(t, true, j.FinishedAt.IsZero())

	_, ok = r.Get("unknown")
	require.Equal(t, false, ok)
}

func TestRegistryListOrder(t *testing.T) {
	r := NewRegistry()
	for _, id := range []string{"c", "a", "b"} {
		_, err := r.Register(id)
		require.NoError(t, err)
	}
	list := r.List()
	require.Equal(t, 3, len(list))
	require.Equal(t, "c", list[0].ID)
	require.Equal(t, "a", list[1].ID)
	require.Equal(t, "b", list[2].ID)
}

func TestRegistryRegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	first, err := r.Register("backfill")
	require.NoError(t, err)

	// Registering over an active job is an error.
	_, err = r.Register("backfill")
	require.ErrorContains(t, "already registered and active", err)

	// Once the job is terminal, a new run may be registered under the same id.
	first.Complete()
	second, err := r.Register("backfill")
	require.NoError(t, err)
	require.NotNil(t, second)
	j, ok := r.Get("backfill")
	require.Equal(t, true, ok)
	require.Equal(t, StatusPending, j.Status)
	require.Equal(t, 1, len(r.List()))
}

func TestTrackerLifecycle(t *testing.T) {
	r := NewRegistry()
	tracker, err := r.Register("op")
	require.NoError(t, err)

	tracker.Start()
	tracker.SetPhase("working")
	tracker.SetProgress(5, 10, "slots")

	j, ok := r.Get("op")
	require.Equal(t, true, ok)
	require.Equal(t, StatusRunning, j.Status)
	require.Equal(t, "working", j.Phase)
	require.NotNil(t, j.Progress)
	require.Equal(t, uint64(5), j.Progress.Current)
	require.Equal(t, uint64(10), j.Progress.Total)
	require.Equal(t, "slots", j.Progress.Units)
	require.Equal(t, false, j.StartedAt.IsZero())
	require.Equal(t, true, j.FinishedAt.IsZero())

	// Start is only valid from pending; a second call must not reset the start time.
	started := j.StartedAt
	tracker.Start()
	j, _ = r.Get("op")
	require.Equal(t, started, j.StartedAt)

	tracker.Complete()
	j, _ = r.Get("op")
	require.Equal(t, StatusCompleted, j.Status)
	require.Equal(t, false, j.FinishedAt.IsZero())

	// Terminal states latch: no further transitions or updates apply.
	tracker.Fail(errors.New("boom"))
	tracker.SetPhase("late")
	tracker.SetProgress(9, 10, "slots")
	tracker.Cancel()
	j, _ = r.Get("op")
	require.Equal(t, StatusCompleted, j.Status)
	require.Equal(t, "working", j.Phase)
	require.Equal(t, "", j.Error)
	require.Equal(t, uint64(5), j.Progress.Current)
}

func TestTrackerFail(t *testing.T) {
	r := NewRegistry()
	tracker, err := r.Register("op")
	require.NoError(t, err)
	tracker.Start()
	tracker.Fail(errors.New("did not work"))

	j, _ := r.Get("op")
	require.Equal(t, StatusFailed, j.Status)
	require.Equal(t, "did not work", j.Error)
	require.Equal(t, false, j.FinishedAt.IsZero())
}

func TestTrackerCompleteWithoutStart(t *testing.T) {
	// A job that has nothing to do may complete straight from pending,
	// in which case it never records a start time.
	r := NewRegistry()
	tracker, err := r.Register("op")
	require.NoError(t, err)
	tracker.SetPhase("disabled")
	tracker.Complete()

	j, _ := r.Get("op")
	require.Equal(t, StatusCompleted, j.Status)
	require.Equal(t, "disabled", j.Phase)
	require.Equal(t, true, j.StartedAt.IsZero())
	require.Equal(t, false, j.FinishedAt.IsZero())
}

func TestStatusTerminal(t *testing.T) {
	require.Equal(t, false, StatusPending.Terminal())
	require.Equal(t, false, StatusRunning.Terminal())
	require.Equal(t, true, StatusCompleted.Terminal())
	require.Equal(t, true, StatusFailed.Terminal())
	require.Equal(t, true, StatusCanceled.Terminal())
}

func TestNilSafety(t *testing.T) {
	var r *Registry
	tracker, err := r.Register("op")
	require.NoError(t, err)
	require.Equal(t, 0, len(r.List()))
	_, ok := r.Get("op")
	require.Equal(t, false, ok)

	// The nil tracker returned by a nil registry is usable as a no-op.
	tracker.Start()
	tracker.SetPhase("phase")
	tracker.SetProgress(1, 2, "units")
	tracker.Complete()
	tracker.Fail(errors.New("boom"))
	tracker.Cancel()
}

func TestSnapshotIsolation(t *testing.T) {
	r := NewRegistry()
	tracker, err := r.Register("op")
	require.NoError(t, err)
	tracker.SetProgress(1, 10, "slots")

	j, _ := r.Get("op")
	tracker.SetProgress(2, 10, "slots")
	// The previously taken snapshot must not observe later updates.
	require.Equal(t, uint64(1), j.Progress.Current)
}

func TestConcurrentUpdates(t *testing.T) {
	r := NewRegistry()
	tracker, err := r.Register("op")
	require.NoError(t, err)
	tracker.Start()

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			tracker.SetProgress(uint64(i), 10, "slots")
		}()
		go func() {
			defer wg.Done()
			r.List()
			r.Get("op")
		}()
	}
	wg.Wait()

	tracker.Complete()
	j, _ := r.Get("op")
	require.Equal(t, StatusCompleted, j.Status)
	require.Equal(t, false, j.FinishedAt.IsZero())
}
