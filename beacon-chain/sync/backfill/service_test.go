package backfill

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/das"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
)

type mockMinimumSlotter struct {
	min primitives.Slot
}

func (m mockMinimumSlotter) minimumSlot(_ primitives.Slot) primitives.Slot {
	return m.min
}

type mockInitalizerWaiter struct {
}

func (*mockInitalizerWaiter) WaitForInitializer(_ context.Context) (*verification.Initializer, error) {
	return &verification.Initializer{}, nil
}

func TestServiceInit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*300)
	defer cancel()
	db := &mockBackfillDB{}
	su, err := NewUpdater(ctx, db)
	require.NoError(t, err)
	nWorkers := 5
	var batchSize uint64 = 4
	nBatches := nWorkers * 2
	var high uint64 = 1 + batchSize*uint64(nBatches) // extra 1 because upper bound is exclusive
	originRoot := [32]byte{}
	origin, err := util.NewBeaconState()
	require.NoError(t, err)
	db.states = map[[32]byte]state.BeaconState{originRoot: origin}
	su.bs = &dbval.BackfillStatus{
		LowSlot:    high,
		OriginRoot: originRoot[:],
	}
	remaining := nBatches
	cw := startup.NewClockSynchronizer()

	clock := startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(primitives.Slot(high)+1))
	require.NoError(t, cw.SetClock(clock))
	pool := &mockPool{todoChan: make(chan batch, nWorkers), finishedChan: make(chan batch, nWorkers)}
	p2pt := p2ptest.NewTestP2P(t)
	bfs := filesystem.NewEphemeralBlobStorage(t)
	dcs := filesystem.NewEphemeralDataColumnStorage(t)
	snw := func() (das.SyncNeeds, error) {
		return das.NewSyncNeeds(
			clock.CurrentSlot,
			nil,
			primitives.Epoch(0),
		)
	}
	srv, err := NewService(ctx, su, bfs, dcs, cw, p2pt, &mockAssigner{},
		WithBatchSize(batchSize), WithWorkerCount(nWorkers), WithEnableBackfill(true), WithVerifierWaiter(&mockInitalizerWaiter{}),
		WithSyncNeedsWaiter(snw))
	require.NoError(t, err)
	srv.pool = pool
	srv.batchImporter = func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
		return &dbval.BackfillStatus{}, nil
	}
	go srv.Start()
	todo := make([]batch, 0)
	todo = testReadN(ctx, t, pool.todoChan, nWorkers, todo)
	require.Equal(t, nWorkers, len(todo))
	for i := range remaining {
		b := todo[i]
		if b.state == batchSequenced {
			b.state = batchImportable
		}
		for i := b.begin; i < b.end; i++ {
			blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, primitives.Slot(i), 0)
			b.blocks = append(b.blocks, blk)
		}
		require.Equal(t, int(batchSize), len(b.blocks))
		pool.finishedChan <- b
		todo = testReadN(ctx, t, pool.todoChan, 1, todo)
	}
	require.Equal(t, remaining+nWorkers, len(todo))
	for i := remaining; i < remaining+nWorkers; i++ {
		require.Equal(t, batchEndSequence, todo[i].state)
	}
}

func testReadN(ctx context.Context, t *testing.T, c chan batch, n int, into []batch) []batch {
	for range n {
		select {
		case b := <-c:
			into = append(into, b)
		case <-ctx.Done():
			// this means we hit the timeout, so something went wrong.
			require.Equal(t, true, false)
		}
	}
	return into
}

// runloopPool wraps the real worker pool so a test can wait until the backfill runloop has sequenced a
// batch. The runloop hands batches to todo() from its own goroutine, and a test that injects worker
// completions without synchronizing has no guarantee the pool has counted them as outstanding yet.
type runloopPool struct {
	*p2pBatchWorkerPool
	// dispatched is buffered so that a runloop which sequences more batches than a test bothers to
	// read cannot block inside todo().
	dispatched chan batch
}

func (p *runloopPool) todo(b batch) {
	// The real pool is asked first, so that a test which observes the batch here can rely on the pool
	// having counted it as outstanding before it hands back a completion for it.
	p.p2pBatchWorkerPool.todo(b)
	if b.state == batchEndSequence {
		// End-of-sequence signals are the pool's own business; the runloop emits them on every pass
		// that has nothing left to download, and a test has already stopped reading by then.
		return
	}
	p.dispatched <- b
}

var _ batchWorkerPool = &runloopPool{}

// awaitDispatched reads n batches that the runloop has handed to the pool, which is the test's signal
// that the runloop is up and about to block in complete() waiting for workers.
func awaitDispatched(ctx context.Context, t *testing.T, p *runloopPool, n int) []batch {
	t.Helper()
	out := make([]batch, 0, n)
	for range n {
		select {
		case b := <-p.dispatched:
			out = append(out, b)
		case <-ctx.Done():
			t.Fatal("runloop never sequenced the expected batches")
		}
	}
	return out
}

// finishedBatch returns b the way a worker would hand it back: importable and carrying a block, since
// importBatches refuses to run the importer for a batch with no results.
func finishedBatch(t *testing.T, b batch) batch {
	t.Helper()
	blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, b.begin, 0)
	f := b.withState(batchImportable)
	f.blocks = verifiedROBlocks{blk}
	return f
}

// newEndgameService builds a backfill service with a small block history to download, wired to the real
// worker pool but with the block importer and the p2p interactions stubbed out, so that a test can play
// the part of the network by handing batches back through the pool.
func newEndgameService(ctx context.Context, t *testing.T, nWorkers int, importer batchImporter) (*Service, *runloopPool) {
	t.Helper()
	var batchSize uint64 = 32
	// The extra 1 is because the batch interval is exclusive of its upper bound, so this leaves exactly
	// nWorkers batches of history between the retention window and the lowest backfilled slot.
	low := uint64(1) + batchSize*uint64(nWorkers)
	db := &mockBackfillDB{}
	su, err := NewUpdater(ctx, db)
	require.NoError(t, err)
	su.bs = &dbval.BackfillStatus{LowSlot: low}

	clock := startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(primitives.Slot(low)+1))
	cw := startup.NewClockSynchronizer()
	require.NoError(t, cw.SetClock(clock))
	sn, err := das.NewSyncNeeds(clock.CurrentSlot, nil, primitives.Epoch(0))
	require.NoError(t, err)
	p2pt := p2ptest.NewTestP2P(t)
	pool := &runloopPool{
		p2pBatchWorkerPool: newP2PBatchWorkerPool(p2pt, nWorkers, sn.Currently),
		dispatched:         make(chan batch, 4*nWorkers),
	}
	srv, err := NewService(ctx, su, filesystem.NewEphemeralBlobStorage(t), filesystem.NewEphemeralDataColumnStorage(t),
		cw, p2pt, &mockAssigner{}, WithBatchSize(batchSize), WithWorkerCount(nWorkers), WithEnableBackfill(true),
		WithSyncNeedsWaiter(func() (das.SyncNeeds, error) { return sn, nil }))
	require.NoError(t, err)
	srv.pool = pool
	// Presetting workerCfg lets Start skip the verifier setup. Workers and router do spawn, but the mock
	// assigner offers no peers so no batch is ever handed to a worker; completions come from the test.
	srv.workerCfg = &workerCfg{clock: clock, currentNeeds: sn.Currently}
	srv.batchImporter = importer
	return srv, pool
}

// TestServiceCompletesWhenWorkRunsOut is the end-to-end regression test for a backfill that reached the
// end of the block history without ever reporting completion, which also left the db pruner parked
// forever on WaitForCompletion. The runloop only turns when the pool hands it a finished batch, and each
// turn can produce at most one end-of-sequence signal for the pool. Delivering the oldest batch first
// parks that batch behind the newer one it chains to, so the first turn yields no signal at all:
//
//	delivery            what the turn does                                  signals banked
//	[1,33) arrives      parked, not importable behind unfinished [33,65)     0
//	[33,65) arrives     imports both, window drained, first signal           1
//
// With one signal banked and no batch left in flight, the old pool waited for a second signal that only
// another turn could produce, and that turn could only be produced by another signal.
func TestServiceCompletesWhenWorkRunsOut(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	const nWorkers = 2
	srv, pool := newEndgameService(ctx, t, nWorkers,
		func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
			return &dbval.BackfillStatus{}, nil
		})
	go srv.Start()

	dispatched := awaitDispatched(ctx, t, pool, nWorkers)
	require.Equal(t, nWorkers, len(dispatched))
	// Ascending by begin, so the oldest batch is delivered first and the newest one last.
	sort.Slice(dispatched, func(i, j int) bool { return dispatched[i].begin < dispatched[j].begin })
	for _, b := range dispatched {
		pool.fromRouter <- finishedBatch(t, b)
	}

	// This is exactly the call the pruner blocks on before pruning anything.
	done := make(chan error, 1)
	go func() { done <- srv.WaitForCompletion() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("WaitForCompletion never returned; backfill stopped without reporting completion")
	}
}

// TestServiceStopsOnUnrecoverableBatchError checks that a batch which cannot be recovered from does not
// leave the runloop waiting for progress that can never happen, and does not end up being reported as a
// successful completion either. That batch is never sequenced again, and every older batch depends on it
// to chain parent roots, so waiters like the pruner must be told the backfill ended in an error.
func TestServiceStopsOnUnrecoverableBatchError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	const nWorkers = 2
	srv, pool := newEndgameService(ctx, t, nWorkers,
		func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
			return nil, errors.Wrap(errUnrecoverable, "test import cannot be recovered")
		})
	go srv.Start()

	dispatched := awaitDispatched(ctx, t, pool, nWorkers)
	pool.fromRouter <- finishedBatch(t, dispatched[0])

	done := make(chan error, 1)
	go func() { done <- srv.WaitForCompletion() }()
	select {
	case err := <-done:
		require.ErrorIs(t, err, errUnrecoverable)
	case <-time.After(30 * time.Second):
		t.Fatal("WaitForCompletion never returned after an unrecoverable batch error")
	}
}
