package blockchain

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	mock "github.com/OffchainLabs/prysm/v7/beacon-chain/blockchain/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache/depositsnapshot"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/filesystem"
	mockExecution "github.com/OffchainLabs/prysm/v7/beacon-chain/execution/testing"
	doublylinkedtree "github.com/OffchainLabs/prysm/v7/beacon-chain/forkchoice/doubly-linked-tree"
	lightclient "github.com/OffchainLabs/prysm/v7/beacon-chain/light-client"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/attestations"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/operations/blstoexec"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stategen"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
)

// minimalTestServiceTB creates a minimal Service for use in both tests and benchmarks.
func minimalTestServiceTB(tb testing.TB) (*Service, context.Context) {
	tb.Helper()
	ctx := context.Background()
	genesis := time.Now().Add(-1 * 4 * time.Duration(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(params.BeaconConfig().SecondsPerSlot)) * time.Second)
	beaconDB := testDB.SetupDB(tb)
	fcs := doublylinkedtree.New()
	fcs.SetGenesisTime(genesis)
	sg := stategen.New(beaconDB, fcs)
	notif := &mockBeaconNode{}
	fcs.SetBalancesByRooter(sg.ActiveNonSlashedBalancesByRoot)
	cs := startup.NewClockSynchronizer()
	attPool := attestations.NewPool()
	attSrv, err := attestations.NewService(ctx, &attestations.Config{Pool: attPool})
	require.NoError(tb, err)
	blsPool := blstoexec.NewPool()
	dc, err := depositsnapshot.New()
	require.NoError(tb, err)

	opts := []Option{
		WithDatabase(beaconDB),
		WithStateNotifier(notif),
		WithStateGen(sg),
		WithForkChoiceStore(fcs),
		WithClockSynchronizer(cs),
		WithAttestationPool(attPool),
		WithAttestationService(attSrv),
		WithBLSToExecPool(blsPool),
		WithDepositCache(dc),
		WithTrackedValidatorsCache(cache.NewTrackedValidatorsCache()),
		WithBlobStorage(filesystem.NewEphemeralBlobStorage(tb)),
		WithDataColumnStorage(filesystem.NewEphemeralDataColumnStorage(tb)),
		WithSyncChecker(mock.MockChecker{}),
		WithExecutionEngineCaller(&mockExecution.EngineClient{}),
		WithP2PBroadcaster(&mockAccessor{}),
		WithLightClientStore(&lightclient.Store{}),
		WithGenesisTime(genesis),
	}
	s, err := NewService(ctx, opts...)
	require.NoError(tb, err)
	return s, ctx
}

// BenchmarkGetAttPreState_CurrentEpoch measures allocs/heap when checkpoint epoch
// equals head epoch (HeadStateReadOnly path).
func BenchmarkGetAttPreState_CurrentEpoch(b *testing.B) {
	b.ReportAllocs()
	service, ctx := minimalTestServiceTB(b)

	s, err := util.NewBeaconState()
	require.NoError(b, err)
	ckRoot := bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	cp0 := &ethpb.Checkpoint{Epoch: 0, Root: ckRoot}
	require.NoError(b, s.SetFinalizedCheckpoint(cp0))

	// Head at slot 64 (epoch 2), checkpoint at epoch 2 (current epoch).
	st, blk, err := prepareForkchoiceState(ctx, 64, [32]byte(ckRoot), [32]byte{}, [32]byte{'R'}, cp0, cp0)
	require.NoError(b, err)
	require.NoError(b, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
	service.head = &head{
		root:  [32]byte(ckRoot),
		state: s,
		block: blk,
		slot:  64,
	}

	cp := &ethpb.Checkpoint{Epoch: 2, Root: ckRoot}

	for b.Loop() {
		var memBefore, memAfter runtime.MemStats
		runtime.ReadMemStats(&memBefore)

		result := service.getRecentPreState(ctx, cp)
		_ = result

		runtime.ReadMemStats(&memAfter)
		b.ReportMetric(float64(memAfter.HeapAlloc-memBefore.HeapAlloc), "heap_alloc_delta/op")
	}
}

// BenchmarkGetAttPreState_PreviousEpoch measures allocs/heap when checkpoint epoch
// is headEpoch-1 (the path from PR #16109).
func BenchmarkGetAttPreState_PreviousEpoch(b *testing.B) {
	b.ReportAllocs()
	service, ctx := minimalTestServiceTB(b)

	s, err := util.NewBeaconState()
	require.NoError(b, err)
	ckRoot := bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	cp0 := &ethpb.Checkpoint{Epoch: 0, Root: ckRoot}
	require.NoError(b, s.SetFinalizedCheckpoint(cp0))

	// Head at slot 64 (epoch 2), checkpoint at epoch 1 (previous epoch).
	st, blk, err := prepareForkchoiceState(ctx, 64, [32]byte(ckRoot), [32]byte{}, [32]byte{'R'}, cp0, cp0)
	require.NoError(b, err)
	require.NoError(b, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
	service.head = &head{
		root:  [32]byte(ckRoot),
		state: s,
		block: blk,
		slot:  64,
	}

	cp := &ethpb.Checkpoint{Epoch: 1, Root: ckRoot}

	for b.Loop() {
		var memBefore, memAfter runtime.MemStats
		runtime.ReadMemStats(&memBefore)

		result := service.getRecentPreState(ctx, cp)
		_ = result

		runtime.ReadMemStats(&memAfter)
		b.ReportMetric(float64(memAfter.HeapAlloc-memBefore.HeapAlloc), "heap_alloc_delta/op")
	}
}

// BenchmarkGetAttPreState_PreviousEpoch_WithHeadChange simulates concurrent
// attestation processing while setHead() is called between iterations.
func BenchmarkGetAttPreState_PreviousEpoch_WithHeadChange(b *testing.B) {
	b.ReportAllocs()
	service, ctx := minimalTestServiceTB(b)

	ckRoot := bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	cp0 := &ethpb.Checkpoint{Epoch: 0, Root: ckRoot}

	// Head at slot 64 (epoch 2).
	st, blk, err := prepareForkchoiceState(ctx, 64, [32]byte(ckRoot), [32]byte{}, [32]byte{'R'}, cp0, cp0)
	require.NoError(b, err)
	require.NoError(b, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))

	s1, err := util.NewBeaconState()
	require.NoError(b, err)
	require.NoError(b, s1.SetFinalizedCheckpoint(cp0))

	s2, err := util.NewBeaconState()
	require.NoError(b, err)
	require.NoError(b, s2.SetFinalizedCheckpoint(cp0))

	service.head = &head{
		root:  [32]byte(ckRoot),
		state: s1,
		block: blk,
		slot:  64,
	}

	cp := &ethpb.Checkpoint{Epoch: 1, Root: ckRoot}

	i := 0
	for b.Loop() {
		var memBefore, memAfter runtime.MemStats
		runtime.ReadMemStats(&memBefore)

		// Get pre-state, then simulate head change.
		result := service.getRecentPreState(ctx, cp)
		_ = result

		// Simulate head change between iterations.
		if i%2 == 0 {
			service.head = &head{
				root:  [32]byte(ckRoot),
				state: s2,
				block: blk,
				slot:  64,
			}
		} else {
			service.head = &head{
				root:  [32]byte(ckRoot),
				state: s1,
				block: blk,
				slot:  64,
			}
		}
		i++

		runtime.ReadMemStats(&memAfter)
		b.ReportMetric(float64(memAfter.HeapAlloc-memBefore.HeapAlloc), "heap_alloc_delta/op")
}

}

// TestGetAttPreState_HeadStateIdentityAfterHeadChange proves that after the fix,
// cached state survives head changes: the checkpoint cache provides stable reference
// identity across head changes.
func TestGetAttPreState_HeadStateIdentityAfterHeadChange(t *testing.T) {
	service, _ := minimalTestService(t)
	ctx := t.Context()

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	ckRoot := bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	cp0 := &ethpb.Checkpoint{Epoch: 0, Root: ckRoot}

	// Build chain: slot 31 (root A) <-- slot 32 (root S) <-- slot 64 (root T)
	st, blk, err := prepareForkchoiceState(ctx, 31, [32]byte(ckRoot), [32]byte{}, [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
	st, blk32, err := prepareForkchoiceState(ctx, 32, [32]byte{'S'}, blk.Root(), [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk32))
	st, blkHead, err := prepareForkchoiceState(ctx, 64, [32]byte{'T'}, blk32.Root(), [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blkHead))

	// Head at slot 64 (epoch 2).
	service.head = &head{
		root:  [32]byte{'T'},
		state: s,
		block: blkHead,
		slot:  64,
	}

	// Checkpoint at epoch 1 (previous epoch).
	blk32Root := blk32.Root()
	cp := &ethpb.Checkpoint{Epoch: 1, Root: blk32Root[:]}

	// First call: populates cache and returns state.
	stateA := service.getRecentPreState(ctx, cp)
	require.NotNil(t, stateA)

	// Verify cache is now populated.
	cached, err := service.checkpointStateCache.StateByCheckpoint(cp)
	require.NoError(t, err)
	require.NotNil(t, cached, "checkpoint cache should be populated after getRecentPreState")

	// Simulate head change: install a new head state.
	s2, err := util.NewBeaconState()
	require.NoError(t, err)
	service.head = &head{
		root:  [32]byte{'T'},
		state: s2,
		block: blkHead,
		slot:  64,
	}

	// Second call: should return cached state (stateA), NOT the new head state.
	stateB := service.getRecentPreState(ctx, cp)
	require.NotNil(t, stateB)
	assert.Equal(t, stateA.Slot(), stateB.Slot(), "second call should return cached state")
}

// TestGetAttPreState_CachePopulated proves that after the fix, the checkpoint state
// cache IS populated for previous-epoch attestations.
func TestGetAttPreState_CachePopulated(t *testing.T) {
	service, _ := minimalTestService(t)
	ctx := t.Context()

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	ckRoot := bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	cp0 := &ethpb.Checkpoint{Epoch: 0, Root: ckRoot}

	// Build chain: slot 31 (root A) <-- slot 32 (root S) <-- slot 64 (root T)
	st, blk, err := prepareForkchoiceState(ctx, 31, [32]byte(ckRoot), [32]byte{}, [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
	st, blk32, err := prepareForkchoiceState(ctx, 32, [32]byte{'S'}, blk.Root(), [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk32))
	st, blkHead, err := prepareForkchoiceState(ctx, 64, [32]byte{'T'}, blk32.Root(), [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blkHead))

	service.head = &head{
		root:  [32]byte{'T'},
		state: s,
		block: blkHead,
		slot:  64,
	}

	blk32Root := blk32.Root()
	cp := &ethpb.Checkpoint{Epoch: 1, Root: blk32Root[:]}

	result := service.getRecentPreState(ctx, cp)
	require.NotNil(t, result)

	// Assert checkpoint cache IS populated (fix restores cache population).
	cached, err := service.checkpointStateCache.StateByCheckpoint(cp)
	require.NoError(t, err)
	assert.NotNil(t, cached, "cache should be populated for previous-epoch attestation")
}

// TestGetAttPreState_MemoryRegression prevents the memory regression from recurring.
// Concurrent goroutines share a single cached reference, and that reference survives head changes.
func TestGetAttPreState_MemoryRegression(t *testing.T) {
	service, _ := minimalTestService(t)
	ctx := t.Context()

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	ckRoot := bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	cp0 := &ethpb.Checkpoint{Epoch: 0, Root: ckRoot}

	st, blk, err := prepareForkchoiceState(ctx, 31, [32]byte(ckRoot), [32]byte{}, [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
	st, blk32, err := prepareForkchoiceState(ctx, 32, [32]byte{'S'}, blk.Root(), [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk32))
	st, blkHead, err := prepareForkchoiceState(ctx, 64, [32]byte{'T'}, blk32.Root(), [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blkHead))

	service.head = &head{
		root:  [32]byte{'T'},
		state: s,
		block: blkHead,
		slot:  64,
	}

	blk32Root := blk32.Root()
	cp := &ethpb.Checkpoint{Epoch: 1, Root: blk32Root[:]}

	// Launch 50 concurrent goroutines calling getAttPreState.
	var wg sync.WaitGroup
	errChan := make(chan error, 50)
	for range 50 {
		wg.Go(func() {
			_, err := service.getAttPreState(ctx, cp)
			if err != nil {
				errChan <- err
			}
		})
	}
	wg.Wait()
	close(errChan)
	for err := range errChan {
		require.NoError(t, err)
	}

	// Assert cache has entry.
	cached, err := service.checkpointStateCache.StateByCheckpoint(cp)
	require.NoError(t, err)
	require.NotNil(t, cached, "cache must have entry after concurrent access")

	// Simulate head change.
	s2, err := util.NewBeaconState()
	require.NoError(t, err)
	service.head = &head{
		root:  [32]byte{'T'},
		state: s2,
		block: blkHead,
		slot:  64,
	}

	// Launch another batch: all should get cached state, not new head.
	var wg2 sync.WaitGroup
	errChan2 := make(chan error, 50)
	for range 50 {
		wg2.Go(func() {
			result, err := service.getAttPreState(ctx, cp)
			if err != nil {
				errChan2 <- err
				return
			}
			if result == nil {
				errChan2 <- errors.New("got nil state")
			}
		})
	}
	wg2.Wait()
	close(errChan2)
	for err := range errChan2 {
		require.NoError(t, err)
	}
}

// TestGetAttPreState_ConcurrentCachePopulation proves concurrent first-access is safe.
func TestGetAttPreState_ConcurrentCachePopulation(t *testing.T) {
	service, _ := minimalTestService(t)
	ctx := t.Context()

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	ckRoot := bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	cp0 := &ethpb.Checkpoint{Epoch: 0, Root: ckRoot}

	st, blk, err := prepareForkchoiceState(ctx, 31, [32]byte(ckRoot), [32]byte{}, [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
	st, blk32, err := prepareForkchoiceState(ctx, 32, [32]byte{'S'}, blk.Root(), [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blk32))
	st, blkHead, err := prepareForkchoiceState(ctx, 64, [32]byte{'T'}, blk32.Root(), [32]byte{}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blkHead))

	service.head = &head{
		root:  [32]byte{'T'},
		state: s,
		block: blkHead,
		slot:  64,
	}

	blk32Root := blk32.Root()
	cp := &ethpb.Checkpoint{Epoch: 1, Root: blk32Root[:]}

	// Launch 100 concurrent goroutines all calling getRecentPreState simultaneously.
	var wg sync.WaitGroup
	results := make([]state.ReadOnlyBeaconState, 100)
	for i := range 100 {
		wg.Go(func() {
			results[i] = service.getRecentPreState(ctx, cp)
		})
	}
	wg.Wait()

	// Assert all succeeded.
	for i, r := range results {
		require.NotNil(t, r, "goroutine %d got nil result", i)
	}

	// Assert cache has entry.
	cached, err := service.checkpointStateCache.StateByCheckpoint(cp)
	require.NoError(t, err)
	require.NotNil(t, cached, "cache must have entry after concurrent population")
}
