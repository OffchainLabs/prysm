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
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
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

	// Check the termination of the service after all batches have been processed.
	done := make(chan error, 1)
	go func() { done <- srv.WaitForCompletion() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("backfill did not complete after the last batch imported")
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

func TestDefaultBatchImporter(t *testing.T) {
	_, fuluSlot := testDenebAndFuluSlots(t)
	current := fuluSlot + 100
	parentHash, lastHash := [32]byte{'p'}, [32]byte{'l'}

	// setup returns a batch whose last block has blobs and an undecided payload, and a store
	// whose lowest backfilled block is the child of that last block.
	setup := func(t *testing.T, childParentHash [32]byte) (*Service, batch, *Store) {
		last := testGloasBlock(t, fuluSlot+1, [32]byte{}, parentHash, lastHash, 1)
		child := testGloasBlock(t, fuluSlot+2, last.Root(), childParentHash, [32]byte{'c'}, 0)
		childRoot, lastRoot := child.Root(), last.Root()
		su := &Store{
			store: &mockBackfillDB{blocks: map[[32]byte]blocks.ROBlock{childRoot: child}},
			bs: &dbval.BackfillStatus{
				LowSlot:       uint64(child.Block().Slot()),
				LowRoot:       childRoot[:],
				LowParentRoot: lastRoot[:],
			},
		}

		sn, err := das.NewSyncNeeds(func() primitives.Slot { return current }, nil, 0)
		require.NoError(t, err)
		p := p2ptest.NewTestP2P(t)
		store := filesystem.NewEphemeralDataColumnStorage(t)
		blks := []blocks.ROBlock{last}
		cb, err := buildColumnBatch(t.Context(), last.Block().Slot(), child.Block().Slot(), blks, p, store, sn.Currently())
		require.NoError(t, err)
		require.Equal(t, true, cb.toDownload[lastRoot].undecidedPayload)

		retain := func(primitives.Slot) bool { return true }
		colStore := das.NewLazilyPersistentStoreColumn(store, nil, p.NodeID(), params.BeaconConfig().CustodyRequirement, nil, retain)
		b := batch{blocks: blks, columns: &columnSync{columnBatch: cb, store: colStore}}
		return &Service{syncNeeds: sn}, b, su
	}

	t.Run("empty payload of the last block is imported without columns", func(t *testing.T) {
		s, b, su := setup(t, parentHash)
		status, err := s.defaultBatchImporter(t.Context(), current, b, su)
		require.NoError(t, err)
		lastRoot := b.blocks[0].Root()
		require.DeepEqual(t, lastRoot[:], status.LowRoot)
	})

	t.Run("full payload of the last block requires columns", func(t *testing.T) {
		s, b, su := setup(t, lastHash)
		_, err := s.defaultBatchImporter(t.Context(), current, b, su)
		require.ErrorContains(t, "no sidecar in cache for block commitment", err)
		require.Equal(t, false, b.columns.toDownload[b.blocks[0].Root()].undecidedPayload)
	})
}

// TestPrunerWaiterUnblockedOnFastEndgame is a regression test which #17282 describes:
// Deadlock when waiting for backfill service completion.
func TestPrunerWaiterUnblockedOnFastEndgame(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	su, err := NewUpdater(ctx, &mockBackfillDB{})
	require.NoError(t, err)

	// Two workers is the default worker count.
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
	srv.workerCfg = &workerCfg{clock: clock, currentNeeds: sn.Currently}
	srv.batchImporter = func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
		return &dbval.BackfillStatus{}, nil
	}

	go srv.Start()

	// finished builds the [begin, end) batch as a worker returns it.
	finished := func(begin, end primitives.Slot) batch {
		blk, _ := util.GenerateTestDenebBlockWithSidecar(t, [32]byte{}, begin, 0)
		return batch{begin: begin, end: end, state: batchImportable, seq: 10, blocks: verifiedROBlocks{blk}}
	}

	// Deliver the lower batch first.
	pool.fromRouter <- finished(1, 33)
	pool.fromRouter <- finished(33, 65)

	// Block on WaitForCompletion exactly as the pruner does.
	done := make(chan error, 1)
	go func() { done <- srv.WaitForCompletion() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForCompletion did not return after backfill ran out of work; the pruner would wait forever")
	}
}
