package backfill

import (
	"testing"
	"time"

	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// poolTurns runs pool interactions on a single goroutine, mirroring the service runloop,
// with a deadline so a regression shows up as a test failure instead of a hung test.
func poolTurns(t *testing.T, turns func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		turns()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("pool interaction blocked; complete() did not return")
	}
}

// TestPoolCompletesAfterFinalSentinel is a regression test for a completion deadlock:
// the sequencer hands the pool at most one batchEndSequence sentinel per scheduling pass,
// and sentinels generate no worker traffic. After the final real batch was imported there
// was nothing left to deliver on fromRouter, so complete() blocked forever and backfill
// never reported completion. With no work outstanding, a single sentinel must end the
// sequence immediately.
func TestPoolCompletesAfterFinalSentinel(t *testing.T) {
	pool := newP2PBatchWorkerPool(p2ptest.NewTestP2P(t), 2, nil)
	pool.spawn(t.Context(), 0, &mockAssigner{}, nil)

	sentinel := batch{begin: 1, end: 1, state: batchEndSequence}

	var (
		endB   batch
		endErr error
	)
	poolTurns(t, func() {
		pool.todo(sentinel)
		endB, endErr = pool.complete()
	})
	require.ErrorIs(t, endErr, errEndSequence)
	require.Equal(t, primitives.Slot(1), endB.begin)
}

// TestPoolNoPrematureCompletion is a regression test for the inverse failure: a sentinel
// showing up while real batches are still in flight is the normal endgame state, and the
// pool must keep draining worker results rather than declaring the end of the sequence
// while work is outstanding.
func TestPoolNoPrematureCompletion(t *testing.T) {
	pool := newP2PBatchWorkerPool(p2ptest.NewTestP2P(t), 2, nil)
	pool.spawn(t.Context(), 0, &mockAssigner{}, nil)

	real := batch{begin: 90, end: 100, state: batchSequenced}
	sentinel := batch{begin: 90, end: 90, state: batchEndSequence}

	var (
		firstB, endB     batch
		firstErr, endErr error
	)
	poolTurns(t, func() {
		pool.todo(real)
		// The scheduling pass that hands out the sentinel runs while the real batch is
		// still downloading.
		pool.todo(sentinel)
		// The worker finishes the real batch; the router forwards completed work on
		// fromRouter, simulated directly here.
		pool.fromRouter <- real.withState(batchImportable)
		// complete() must return the finished real batch, not end the sequence.
		firstB, firstErr = pool.complete()
		if firstErr != nil {
			return
		}
		// Next scheduling pass hands out another sentinel; with nothing outstanding the
		// pool must now report the end of the sequence.
		pool.todo(sentinel)
		endB, endErr = pool.complete()
	})
	require.NoError(t, firstErr)
	require.Equal(t, batchImportable, firstB.state)
	require.ErrorIs(t, endErr, errEndSequence)
	require.Equal(t, primitives.Slot(90), endB.begin)
}
