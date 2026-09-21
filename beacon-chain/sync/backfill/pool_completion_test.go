package backfill

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
)

// completionNeeds describes a retention window that contains every slot used in this file, so that a
// router which happens to look at a queued batch never retires it early.
func completionNeeds() das.CurrentNeeds {
	return das.CurrentNeeds{Block: das.NeedSpan{Begin: 1, End: 1 << 32}}
}

// newUnspawnedPool builds a pool that a test can drive directly from its own goroutine, which is
// how the backfill runloop uses it. Neither the router nor the workers are spawned: todo() fills the
// buffered toRouter channel, and batches are handed back to complete() through the buffered
// fromRouter channel, which is all the completion logic looks at.
func newUnspawnedPool(t *testing.T, maxBatches int) *p2pBatchWorkerPool {
	p := newP2PBatchWorkerPool(p2ptest.NewTestP2P(t), maxBatches, completionNeeds)
	p.ctx, p.cancel = context.WithCancel(t.Context())
	t.Cleanup(p.cancel)
	return p
}

// completesInTime runs fn and fails the test if it does not return promptly. A completion regression
// in this package shows up as complete() blocking forever, so without this the test target would hang
// until the bazel timeout instead of reporting which guarantee broke.
func completesInTime(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not return; the backfill runloop would be parked here forever", what)
	}
}

// TestPoolCompletesOnFinalSentinel is the regression test for the end-of-backfill deadlock.
// The sequencer hands the pool at most one batchEndSequence signal per scheduling pass, and those
// signals generate no worker traffic. The runloop turns only when complete() returns a batch, so once
// the final batch had been imported there was nothing left to deliver on fromRouter: requiring one
// signal per worker meant complete() waited forever for a signal that only another turn of the runloop
// could have produced. A signal plus nothing outstanding must end the sequence immediately.
func TestPoolCompletesOnFinalSentinel(t *testing.T) {
	pool := newUnspawnedPool(t, 2)
	sentinel := batch{begin: 1, end: 1, state: batchEndSequence}

	var (
		b   batch
		err error
	)
	completesInTime(t, "complete() after the final sentinel", func() {
		pool.todo(sentinel)
		b, err = pool.complete()
	})
	require.ErrorIs(t, err, errEndSequence)
	require.Equal(t, primitives.Slot(1), b.begin)
}

// TestPoolNoPrematureCompletion covers the inverse failure: an end-of-sequence signal arriving while
// real batches are still in flight is the normal state at the start of the endgame, and the pool must
// keep draining worker results rather than declaring the sequence over with data still to import.
func TestPoolNoPrematureCompletion(t *testing.T) {
	pool := newUnspawnedPool(t, 2)
	real := batch{begin: 90, end: 120, state: batchSequenced}
	sentinel := batch{begin: 90, end: 90, state: batchEndSequence}

	pool.todo(real)
	pool.todo(sentinel)

	res := make(chan error, 1)
	var b batch
	go func() {
		var err error
		b, err = pool.complete()
		res <- err
	}()
	select {
	case err := <-res:
		t.Fatalf("pool ended the sequence with a batch outstanding, err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}

	// The worker finishes the batch; complete() must hand it back rather than end the sequence.
	pool.fromRouter <- real.withState(batchImportable)
	require.NoError(t, <-res)
	require.Equal(t, batchImportable, b.state)

	// Nothing is outstanding now and the signal was already recorded, so the sequence ends.
	var end batch
	var endErr error
	completesInTime(t, "complete() once the last batch was returned", func() {
		end, endErr = pool.complete()
	})
	require.ErrorIs(t, endErr, errEndSequence)
	require.Equal(t, primitives.Slot(90), end.begin)
}

// TestPoolWindsDownAfterRouterRetiresBatches checks the handoff between the router goroutine and the
// runloop for batches that fall outside the retention window while queued. The router used to record
// those as end-of-sequence signals itself, which mutated a slice owned by the runloop goroutine (a data
// race) and left the batch unaccounted for. It now sends them back through fromRouter, so complete()
// releases them from the outstanding count like any other delivery, and only the sequencer's own
// signal ends the sequence.
func TestPoolWindsDownAfterRouterRetiresBatches(t *testing.T) {
	ranges := [][2]primitives.Slot{{100, 200}, {50, 100}, {0, 50}}
	pool := newUnspawnedPool(t, len(ranges))

	todo := make([]batch, 0, len(ranges))
	for _, r := range ranges {
		b := batch{begin: r[0], end: r[1], state: batchSequenced}
		pool.todo(b)
		todo = append(todo, b)
	}
	require.Equal(t, len(ranges), pool.outstanding)

	// Retention starts above every queued batch, so the router retires all of them.
	pool.needs = func() das.CurrentNeeds {
		return das.CurrentNeeds{Block: das.NeedSpan{Begin: 1000, End: 2000}}
	}
	remaining, err := pool.processTodo(todo, &mockAssigner{assign: []peer.ID{"pid"}}, map[peer.ID]bool{})
	require.NoError(t, err)
	require.Equal(t, 0, len(remaining))
	// Retiring a batch is not the runloop's business until it collects it, so no signal is recorded.
	require.Equal(t, 0, len(pool.endSeq))

	// The sequencer emits the end-of-sequence signal on its next pass with nothing left to download.
	pool.todo(batch{begin: 0, end: 0, state: batchEndSequence})

	// The retired batches still have to come back to the runloop before the sequence can end.
	for _, want := range todo {
		b, err := pool.complete()
		require.NoError(t, err)
		require.Equal(t, batchEndSequence, b.state)
		require.Equal(t, want.begin, b.begin)
		require.Equal(t, want.end, b.end)
	}
	require.Equal(t, 0, pool.outstanding)

	var b batch
	completesInTime(t, "complete() once retired batches were collected", func() {
		var err error
		b, err = pool.complete()
		require.ErrorIs(t, err, errEndSequence)
	})
	require.Equal(t, primitives.Slot(0), b.begin)
}

// TestPoolSentinelRecordedOnce checks that re-emitting the end of the sequence on every scheduling
// pass does not accumulate a batch per pass; a single recorded signal is enough to wind down.
func TestPoolSentinelRecordedOnce(t *testing.T) {
	pool := newUnspawnedPool(t, 2)
	for range 10 {
		pool.todo(batch{begin: 5, end: 5, state: batchEndSequence})
	}
	require.Equal(t, 1, len(pool.endSeq))
	require.Equal(t, 0, pool.outstanding)
}

// TestPoolShutdownSignalDoesNotBlockTheRouter makes sure a failed pool cannot wedge the router.
// shutdown is called from the router goroutine, and the runloop only reads the error from complete(),
// which it may not be doing when the failure happens - it can be importing a batch or handing one out.
// A blocking send used to park the router with the busy peer map held.
func TestPoolShutdownSignalDoesNotBlockTheRouter(t *testing.T) {
	pool := newUnspawnedPool(t, 2)
	failure := errors.New("assignment failed")

	completesInTime(t, "shutdown() with no reader waiting", func() {
		pool.shutdown(failure)
		// A second failure while the first is still pending must be dropped, not deadlock.
		pool.shutdown(errors.New("context canceled"))
	})

	// shutdown also cancels the pool context, which complete() selects on alongside the error, so
	// either way it must return promptly rather than wait for a delivery that will never come.
	var err error
	completesInTime(t, "complete() after the pool failed", func() {
		_, err = pool.complete()
	})
	require.NotNil(t, err)
}
