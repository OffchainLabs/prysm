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
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
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

// TestPrunerWaiterUnblockedOnFastEndgame reproduces the fast-endgame deadlock that parked
// backfill at 100% forever and starved the pruner, which blocks on WaitForCompletion before
// pruning anything. It drives the real runloop against the real p2pBatchWorkerPool with two
// batches. The runloop only turns when the pool delivers a finished batch, each turn yields at
// most one end-of-sequence sentinel, and the old pool required maxBatches sentinels to report
// completion:
//
//	fuel (delivery)   what the turn does                                sentinels banked
//	[1,33) arrives    parks: not importable behind unfinished [33,65)   0
//	[33,65) arrives   imports both, window drains, first sentinel       1
//	none left         complete() waits for fuel that can never arrive   1 < 2 → wedged
//
// The fixed pool detects completion synchronously on that final turn (sentinel present, no
// batches outstanding), so WaitForCompletion — called here exactly as the pruner calls it —
// returns instead of hanging.
func TestPrunerWaiterUnblockedOnFastEndgame(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	su, err := NewUpdater(ctx, &mockBackfillDB{})
	require.NoError(t, err)
	// Two workers is the smallest pool that wedges: the old code wanted one sentinel per worker.
	const nWorkers, batchSize = 2, 32
	low := uint64(1 + batchSize*nWorkers) // two batches of history: [33,65) and [1,33)
	su.bs = &dbval.BackfillStatus{LowSlot: low}
	cw := startup.NewClockSynchronizer()
	clock := startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(primitives.Slot(low)+1))
	require.NoError(t, cw.SetClock(clock))
	sn, err := das.NewSyncNeeds(clock.CurrentSlot, nil, 0)
	require.NoError(t, err)
	p2pt := p2ptest.NewTestP2P(t)
	pool := newP2PBatchWorkerPool(p2pt, nWorkers, sn.Currently)
	srv, err := NewService(ctx, su, filesystem.NewEphemeralBlobStorage(t), filesystem.NewEphemeralDataColumnStorage(t),
		cw, p2pt, &mockAssigner{}, WithBatchSize(batchSize), WithWorkerCount(nWorkers), WithEnableBackfill(true),
		WithSyncNeedsWaiter(func() (das.SyncNeeds, error) { return sn, nil }))
	require.NoError(t, err)
	srv.pool = pool
	// Preset workerCfg so Start skips verifier setup; the workers spawn but never see a batch,
	// because the mock assigner offers no peers and deliveries are injected below.
	srv.workerCfg = &workerCfg{clock: clock, currentNeeds: sn.Currently}
	srv.batchImporter = func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
		return &dbval.BackfillStatus{}, nil
	}
	go srv.Start()

	// finished builds the [begin, end) batch as a worker would hand it back: importable, carrying
	// a block (importBatches rejects empty batches), seq bumped so it replaces the sequencer's copy.
	finished := func(begin, end primitives.Slot) batch {
		blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, begin, 0)
		return batch{begin: begin, end: end, state: batchImportable, seq: 10, blocks: verifiedROBlocks{blk}}
	}
	// Deliver both batches back-to-back — the fast-net burst. Order is load-bearing: lower first
	// means turn one imports nothing (parked behind [33,65)), so the first sentinel appears only
	// on the turn fueled by the last delivery in the system. Higher first would bank a sentinel
	// on each turn and even the buggy count would reach maxBatches.
	pool.fromRouter <- finished(1, 33)
	pool.fromRouter <- finished(33, 65)

	// Block on WaitForCompletion exactly as the pruner does.
	done := make(chan error, 1)
	go func() { done <- srv.WaitForCompletion() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("WaitForCompletion did not return after backfill ran out of work; the pruner would wait forever")
	}
}
