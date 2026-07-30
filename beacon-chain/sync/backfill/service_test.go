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
}

func TestServiceReportsCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*300)
	defer cancel()

	db := &mockBackfillDB{}
	store, err := NewUpdater(ctx, db)
	require.NoError(t, err)

	const (
		nWorkers = 5
		nBatches = nWorkers - 3
	)

	batchSize := uint64(4)
	high := 1 + batchSize*uint64(nBatches) // extra 1 because upper bound is exclusive

	originRoot := [32]byte{}
	origin, err := util.NewBeaconState()
	require.NoError(t, err)

	db.states = map[[32]byte]state.BeaconState{originRoot: origin}
	store.bs = &dbval.BackfillStatus{
		LowSlot:    high,
		OriginRoot: originRoot[:],
	}

	clockSynchronizer := startup.NewClockSynchronizer()
	clock := startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(primitives.Slot(high)+1))
	require.NoError(t, clockSynchronizer.SetClock(clock))

	p2pt := p2ptest.NewTestP2P(t)
	blobStorage := filesystem.NewEphemeralBlobStorage(t)
	dataColumnStorage := filesystem.NewEphemeralDataColumnStorage(t)
	syncNeedsWaiter := func() (das.SyncNeeds, error) {
		return das.NewSyncNeeds(clock.CurrentSlot, nil, primitives.Epoch(0))
	}

	service, err := NewService(
		ctx, store, blobStorage, dataColumnStorage, clockSynchronizer, p2pt, &mockAssigner{},
		WithBatchSize(batchSize), WithWorkerCount(nWorkers), WithEnableBackfill(true), WithVerifierWaiter(&mockInitalizerWaiter{}), WithSyncNeedsWaiter(syncNeedsWaiter),
	)
	require.NoError(t, err)

	// The importer is stubbed out, so a single block is enough to keep batches out of the
	// "batch with no results" error path.
	blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	require.NoError(t, err)

	rob, err := blocks.NewROBlock(blk)
	require.NoError(t, err)

	needs, err := syncNeedsWaiter()
	require.NoError(t, err)

	service.pool = &stubRoutedPool{
		p2pBatchWorkerPool: newP2PBatchWorkerPool(p2pt, nWorkers, needs.Currently),
		blocks:             verifiedROBlocks{rob},
	}

	service.batchImporter = func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
		return &dbval.BackfillStatus{}, nil
	}
	go service.Start()

	done := make(chan error, 1)
	go func() { done <- service.WaitForCompletion() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("backfill reached the end of the sequence but never reported completion")
	}
}

// stubRoutedPool exercises the real pool bookkeeping in todo() and complete(), standing in for the
// p2p worker fleet with a router that hands every batch straight back as importable.
type stubRoutedPool struct {
	*p2pBatchWorkerPool
	blocks verifiedROBlocks
}

func (p *stubRoutedPool) spawn(ctx context.Context, _ int, _ PeerAssigner, _ *workerCfg) {
	p.ctx, p.cancel = context.WithCancel(ctx)
	go func() {
		for {
			select {
			case b := <-p.toRouter:
				b.blocks = p.blocks
				p.fromRouter <- b.withState(batchImportable)
			case <-p.ctx.Done():
				return
			}
		}
	}()
}

var _ batchWorkerPool = &stubRoutedPool{}

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
