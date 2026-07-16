package backfill

import (
	"context"
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
	"github.com/OffchainLabs/prysm/v7/runtime/jobs"
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
	pool := &mockPool{todoChan: make(chan batch, nWorkers), finishedChan: make(chan batch, nWorkers), finishedErr: make(chan error, 1)}
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
	reg := jobs.NewRegistry()
	srv, err := NewService(ctx, su, bfs, dcs, cw, p2pt, &mockAssigner{},
		WithBatchSize(batchSize), WithWorkerCount(nWorkers), WithEnableBackfill(true), WithVerifierWaiter(&mockInitalizerWaiter{}),
		WithSyncNeedsWaiter(snw), WithJobRegistry(reg))
	require.NoError(t, err)
	srv.pool = pool
	srv.batchImporter = func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
		return &dbval.BackfillStatus{}, nil
	}
	go srv.Start()
	todo := make([]batch, 0)
	todo = testReadN(ctx, t, pool.todoChan, nWorkers, todo)
	require.Equal(t, nWorkers, len(todo))
	// The service has handed batches to the pool, so the job must report as running.
	j, ok := reg.Get(JobID)
	require.Equal(t, true, ok)
	require.Equal(t, jobs.StatusRunning, j.Status)
	require.Equal(t, jobPhaseBackfilling, j.Phase)
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
	// Drive the service to completion and verify the job reports it.
	pool.finishedErr <- errEndSequence
	require.NoError(t, srv.WaitForCompletion())
	j, ok = reg.Get(JobID)
	require.Equal(t, true, ok)
	require.Equal(t, jobs.StatusCompleted, j.Status)
	require.Equal(t, jobPhaseComplete, j.Phase)
	require.Equal(t, false, j.FinishedAt.IsZero())
}

func TestJobStatusDisabled(t *testing.T) {
	reg := jobs.NewRegistry()
	srv, err := NewService(t.Context(), nil, nil, nil, nil, nil, nil, WithEnableBackfill(false), WithJobRegistry(reg))
	require.NoError(t, err)

	// The job is registered as pending before the service starts.
	j, ok := reg.Get(JobID)
	require.Equal(t, true, ok)
	require.Equal(t, jobs.StatusPending, j.Status)

	// A disabled service completes the job immediately with an explanatory phase.
	srv.Start()
	j, ok = reg.Get(JobID)
	require.Equal(t, true, ok)
	require.Equal(t, jobs.StatusCompleted, j.Status)
	require.Equal(t, jobPhaseDisabled, j.Phase)
}

func TestJobStatusCanceledOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	db := &mockBackfillDB{}
	su, err := NewUpdater(ctx, db)
	require.NoError(t, err)
	nWorkers := 5
	var batchSize uint64 = 4
	var high uint64 = 1 + batchSize*uint64(nWorkers*2)
	originRoot := [32]byte{}
	origin, err := util.NewBeaconState()
	require.NoError(t, err)
	db.states = map[[32]byte]state.BeaconState{originRoot: origin}
	su.bs = &dbval.BackfillStatus{
		LowSlot:    high,
		OriginRoot: originRoot[:],
	}
	cw := startup.NewClockSynchronizer()
	clock := startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(primitives.Slot(high)+1))
	require.NoError(t, cw.SetClock(clock))
	pool := &mockPool{todoChan: make(chan batch, nWorkers), finishedChan: make(chan batch, nWorkers), finishedErr: make(chan error, 1), completeEntered: make(chan struct{})}
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
	reg := jobs.NewRegistry()
	srv, err := NewService(ctx, su, bfs, dcs, cw, p2pt, &mockAssigner{},
		WithBatchSize(batchSize), WithWorkerCount(nWorkers), WithEnableBackfill(true), WithVerifierWaiter(&mockInitalizerWaiter{}),
		WithSyncNeedsWaiter(snw), WithJobRegistry(reg))
	require.NoError(t, err)
	srv.pool = pool
	go srv.Start()
	testReadN(ctx, t, pool.todoChan, nWorkers, nil)
	// Wait until the service is inside pool.complete(), past the runloop's own
	// context check, so cancellation is observed on the updateComplete path.
	select {
	case <-pool.completeEntered:
	case <-time.After(10 * time.Second):
		t.Fatal("service did not call pool.complete()")
	}
	// Cancel the parent context as node shutdown does, then unblock complete()
	// with the context error the real pool returns in that case.
	cancel()
	pool.finishedErr <- context.Canceled
	select {
	case <-srv.complete:
	case <-time.After(10 * time.Second):
		t.Fatal("service did not exit after cancellation")
	}
	// The job must be recorded as canceled, not failed, and markComplete on the
	// service exit path must not overwrite the canceled status.
	j, ok := reg.Get(JobID)
	require.Equal(t, true, ok)
	require.Equal(t, jobs.StatusCanceled, j.Status)
	require.Equal(t, "", j.Error)
	require.Equal(t, false, j.FinishedAt.IsZero())
}

func TestJobStatusFailedOnPoolInternalError(t *testing.T) {
	// When the worker pool shuts down because of an internal fatal error, it cancels
	// its own child context, so complete() can surface context.Canceled while the
	// service's parent context is still healthy. That is a real backfill failure and
	// must be recorded as failed, not canceled.
	reg := jobs.NewRegistry()
	tr, err := reg.Register(JobID)
	require.NoError(t, err)
	tr.Start()
	pool := &mockPool{finishedErr: make(chan error, 1)}
	pool.finishedErr <- errors.Wrap(context.Canceled, "fatal error from backfill worker pool")
	srv := &Service{ctx: t.Context(), pool: pool, job: tr}
	require.Equal(t, true, srv.updateComplete())
	j, ok := reg.Get(JobID)
	require.Equal(t, true, ok)
	require.Equal(t, jobs.StatusFailed, j.Status)
	require.Equal(t, false, j.FinishedAt.IsZero())
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
