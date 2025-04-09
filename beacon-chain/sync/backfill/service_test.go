package backfill

import (
	"context"
	"testing"
	"time"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/db/filesystem"
	p2ptest "github.com/prysmaticlabs/prysm/v5/beacon-chain/p2p/testing"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/startup"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/verification"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/proto/dbval"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*300)
	defer cancel()
	db := &mockBackfillDB{}
	su, err := NewUpdater(ctx, db)
	require.NoError(t, err)
	nWorkers := 5
	var batchSize uint64 = 100
	nBatches := nWorkers * 2
	var high uint64 = 11235
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
	require.NoError(t, cw.SetClock(startup.NewClock(time.Now(), [32]byte{})))
	pool := &mockPool{todoChan: make(chan batch, nWorkers), finishedChan: make(chan batch, nWorkers)}
	p2pt := p2ptest.NewTestP2P(t)
	bfs := filesystem.NewEphemeralBlobStorage(t)
	srv, err := NewService(ctx, su, bfs, cw, p2pt, &mockAssigner{},
		WithBatchSize(batchSize), WithWorkerCount(nWorkers), WithEnableBackfill(true), WithVerifierWaiter(&mockInitalizerWaiter{}))
	require.NoError(t, err)
	srv.ms = mockMinimumSlotter{min: primitives.Slot(high - batchSize*uint64(nBatches))}.minimumSlot
	srv.pool = pool
	srv.batchImporter = func(context.Context, primitives.Slot, batch, *Store) (*dbval.BackfillStatus, error) {
		return &dbval.BackfillStatus{}, nil
	}
	go srv.Start()
	todo := make([]batch, 0)
	todo = testReadN(ctx, t, pool.todoChan, nWorkers, todo)
	require.Equal(t, nWorkers, len(todo))
	for i := 0; i < remaining; i++ {
		b := todo[i]
		if b.state == batchSequenced {
			b.state = batchImportable
		}
		pool.finishedChan <- b
		todo = testReadN(ctx, t, pool.todoChan, 1, todo)
	}
	require.Equal(t, remaining+nWorkers, len(todo))
	for i := remaining; i < remaining+nWorkers; i++ {
		require.Equal(t, batchEndSequence, todo[i].state)
	}
}

func TestMinimumBackfillSlot(t *testing.T) {
	oe := helpers.MinEpochsForBlockRequests()

	currSlot := (oe + 100).Mul(uint64(params.BeaconConfig().SlotsPerEpoch))
	minSlot := minimumBackfillSlot(primitives.Slot(currSlot))
	require.Equal(t, 100*params.BeaconConfig().SlotsPerEpoch, minSlot)

	currSlot = oe.Mul(uint64(params.BeaconConfig().SlotsPerEpoch))
	minSlot = minimumBackfillSlot(primitives.Slot(currSlot))
	require.Equal(t, primitives.Slot(1), minSlot)
}

func testReadN(ctx context.Context, t *testing.T, c chan batch, n int, into []batch) []batch {
	for i := 0; i < n; i++ {
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

func TestBackfillMinSlotDefault(t *testing.T) {
	oe := helpers.MinEpochsForBlockRequests()
	current := primitives.Slot((oe + 100).Mul(uint64(params.BeaconConfig().SlotsPerEpoch)))
	s := &Service{}
	specMin := minimumBackfillSlot(current)

	t.Run("equal to specMin", func(t *testing.T) {
		opt := WithMinimumSlot(specMin)
		require.NoError(t, opt(s))
		require.Equal(t, specMin, s.ms(current))
	})
	t.Run("older than specMin", func(t *testing.T) {
		opt := WithMinimumSlot(specMin - 1)
		require.NoError(t, opt(s))
		// if WithMinimumSlot is older than the spec minimum, we should use it.
		require.Equal(t, specMin-1, s.ms(current))
	})
	t.Run("newer than specMin", func(t *testing.T) {
		opt := WithMinimumSlot(specMin + 1)
		require.NoError(t, opt(s))
		// if WithMinimumSlot is newer than the spec minimum, we should use the spec minimum
		require.Equal(t, specMin, s.ms(current))
	})
}
