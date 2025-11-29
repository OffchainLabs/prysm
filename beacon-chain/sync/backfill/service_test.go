package backfill

import (
	"context"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	p2ptest "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/verification"
	"github.com/OffchainLabs/prysm/v7/config/params"
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
	// With WithMinimumSlot(0), we backfill from high down to slot 0.
	// To get exactly nBatches, we need: high = nBatches * batchSize
	// (slot 0 is excluded since genesis block has invalid signature)
	var high uint64 = batchSize * uint64(nBatches)
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

	require.NoError(t, cw.SetClock(startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(primitives.Slot(high)+1))))
	pool := &mockPool{todoChan: make(chan batch, nWorkers), finishedChan: make(chan batch, nWorkers)}
	p2pt := p2ptest.NewTestP2P(t)
	bfs := filesystem.NewEphemeralBlobStorage(t)
	dcs := filesystem.NewEphemeralDataColumnStorage(t)
	srv, err := NewService(ctx, su, bfs, dcs, cw, p2pt, &mockAssigner{},
		WithBatchSize(batchSize), WithWorkerCount(nWorkers), WithEnableBackfill(true), WithVerifierWaiter(&mockInitalizerWaiter{}),
		WithMinimumSlot(0))
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

// TestWithBlobRetentionEpochPreserved is a regression test for the bug where
// WithBlobRetentionEpoch values were lost during service Start().
// The bug was in service.go line 264 where `syncNeeds{}.initialize(...)` was
// creating a new empty struct instead of using `s.syncNeeds.initialize(...)`
// which would preserve the flag values set by service options.
func TestWithBlobRetentionEpochPreserved(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*5)
	defer cancel()

	db := &mockBackfillDB{}
	su, err := NewUpdater(ctx, db)
	require.NoError(t, err)

	// Current slot must be:
	// 1. Beyond Fulu fork (mainnet: epoch 411392 = slot 13,164,544) so columns are relevant
	// 2. High enough that custom retention (100,000 epochs = 3,200,000 slots) doesn't underflow
	// Using slot 17,000,000 which is well past Fulu and allows meaningful retention math.
	var currentSlot primitives.Slot = 17_000_000
	originRoot := [32]byte{}
	origin, err := util.NewBeaconState()
	require.NoError(t, err)
	db.states = map[[32]byte]state.BeaconState{originRoot: origin}
	su.bs = &dbval.BackfillStatus{
		LowSlot:    uint64(currentSlot),
		OriginRoot: originRoot[:],
	}

	cw := startup.NewClockSynchronizer()
	require.NoError(t, cw.SetClock(startup.NewClock(time.Now(), [32]byte{}, startup.WithSlotAsNow(currentSlot+1))))

	p2pt := p2ptest.NewTestP2P(t)
	bfs := filesystem.NewEphemeralBlobStorage(t)
	dcs := filesystem.NewEphemeralDataColumnStorage(t)

	// The key: set a custom retention epoch larger than spec minimum (4096)
	customRetention := primitives.Epoch(100000)

	srv, err := NewService(ctx, su, bfs, dcs, cw, p2pt, &mockAssigner{},
		WithBatchSize(4),
		WithWorkerCount(1),
		WithEnableBackfill(true),
		WithVerifierWaiter(&mockInitalizerWaiter{}),
		WithBlobRetentionEpoch(customRetention),
	)
	require.NoError(t, err)

	// Use a mock pool so we can detect when Start() reaches the main loop
	pool := &mockPool{
		todoChan:     make(chan batch, 1),
		finishedChan: make(chan batch, 1),
	}
	srv.pool = pool

	// Start the service in background - it will initialize syncNeeds at line 264
	go srv.Start()

	// Wait for a batch to be scheduled (proves Start() got past line 264)
	select {
	case <-pool.todoChan:
		// Got past initialization
	case <-ctx.Done():
		t.Fatal("timeout waiting for batch - Start() may have exited early")
	}

	// Now verify the retention was preserved
	needs := srv.syncNeeds.currently()

	// The col retention should be based on our custom epoch, not the spec minimum (4096).
	// With current slot 17,000,001 and 100,000 epoch retention:
	// retention slots = 100,000 * 32 = 3,200,000
	// expected begin = 17,000,001 - 3,200,000 = 13,800,001
	expectedColBegin := syncEpochOffset(currentSlot+1, customRetention)
	require.Equal(t, expectedColBegin, needs.col.begin,
		"column retention start slot should reflect custom retention epoch, not spec minimum")

	// If the bug regresses (syncNeeds{} instead of s.syncNeeds), the retention would be
	// the spec minimum of 4096 epochs = 131,072 slots, giving begin = 17,000,001 - 131,072 = 16,868,929
	specMinimumBegin := syncEpochOffset(currentSlot+1, params.BeaconConfig().MinEpochsForDataColumnSidecarsRequest)
	require.NotEqual(t, specMinimumBegin, needs.col.begin,
		"column retention should NOT be using spec minimum - the custom flag should take precedence")
}
