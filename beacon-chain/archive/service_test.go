package archive

import (
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/kv"
	testDB "github.com/OffchainLabs/prysm/v7/beacon-chain/db/testing"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/startup"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func archiveMetric(t *testing.T, metric prometheus.Metric) *dto.Metric {
	t.Helper()
	m := new(dto.Metric)
	require.NoError(t, metric.Write(m))
	return m
}

// A round walks to the finalized epoch start and then offers the handoff. The handoff is offered with the
// first boundary the walk has *not* written, which is what stategen compares against the finalized slot.
func TestRound_WalksThenOffersHandoff(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	ctx := t.Context()
	store := testDB.SetupDB(t).(*kv.Store)
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	fEpoch := primitives.Epoch(2)
	fSlot := primitives.Slot(fEpoch) * slotsPerEpoch

	genesisState, keys := util.DeterministicGenesisState(t, 64)
	require.NoError(t, store.InitializeArchiveOrigin(ctx, genesisState))
	buildChain(t, ctx, store, genesisState, keys, fSlot, nil)
	require.NoError(t, store.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{
		Epoch: fEpoch,
		Root:  finalizedRootAtSlot(t, store, fSlot),
	}))

	sg := &mockStateManager{pending: true}
	svc := New(ctx, store, sg, nil, nil)
	as, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	svc.setStatus(as)

	// Handoff refused: the walk has caught up but the service keeps going.
	done, err := svc.round(ctx)
	require.NoError(t, err)
	require.Equal(t, false, done)
	require.Equal(t, fSlot, svc.status().RegeneratedThroughSlot)
	require.Equal(t, fSlot+slotsPerEpoch, sg.nextArg)

	// The handoff is conditional on recording it: if that fails, nothing opens and the round reports it is
	// not done, so the service retries rather than exiting with the two gates disagreeing.
	sg.handoff = true
	sg.markErr = errors.New("status write failed")
	done, err = svc.round(ctx)
	require.NotNil(t, err)
	require.Equal(t, false, done)
	require.Equal(t, true, sg.ArchivePending())
	unpersisted, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, false, unpersisted.Complete)
	_, stillPending := archivePendingForTesting(t, store)
	require.Equal(t, true, stillPending)

	// Handoff accepted: the status is marked complete and persisted.
	sg.markErr = nil
	done, err = svc.round(ctx)
	require.NoError(t, err)
	require.Equal(t, true, done)
	require.Equal(t, float64(1), archiveMetric(t, regenerationPhase.WithLabelValues("complete")).GetGauge().GetValue())
	require.Equal(t, float64(0), archiveMetric(t, regenerationPhase.WithLabelValues("walking")).GetGauge().GetValue())
	persisted, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, true, persisted.Complete)

	// A completed archive no longer blocks live writes into the tree.
	_, pendingInDB := archivePendingForTesting(t, store)
	require.Equal(t, false, pendingInDB)
}

// A service whose regeneration is already complete must not walk again.
func TestStart_NoopWhenNotPending(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	store := testDB.SetupDB(t).(*kv.Store)
	svc := New(t.Context(), store, &mockStateManager{pending: false}, nil, func() error {
		t.Fatal("backfill waiter must not be called when regeneration is complete")
		return nil
	})
	svc.Start()
}

func finalizedRootAtSlot(t *testing.T, store *kv.Store, slot primitives.Slot) []byte {
	_, roots, err := store.HighestRootsBelowSlot(t.Context(), slot+1)
	require.NoError(t, err)
	require.Equal(t, 1, len(roots))
	return roots[0][:]
}

func archivePendingForTesting(t *testing.T, store *kv.Store) (primitives.Slot, bool) {
	as, err := store.ArchiveStatus(t.Context())
	require.NoError(t, err)
	return as.RegeneratedThroughSlot, !as.Complete
}

func TestStop_UnblocksBackfillWait(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	ctx := t.Context()
	store := testDB.SetupDB(t).(*kv.Store)
	genesisState, _ := util.DeterministicGenesisState(t, 64)
	require.NoError(t, store.InitializeArchiveOrigin(ctx, genesisState))

	waiting := make(chan struct{})
	block := make(chan struct{})
	defer close(block)
	cs := startup.NewClockSynchronizer()
	require.NoError(t, cs.SetClock(startup.NewClock(time.Now(), [32]byte{})))
	svc := New(ctx, store, &mockStateManager{pending: true}, cs, func() error {
		close(waiting)
		<-block
		return nil
	})
	go svc.Start()
	<-waiting
	require.Equal(t, float64(0), archiveMetric(t, archiveOriginSlot).GetGauge().GetValue())
	require.Equal(t, float64(0), archiveMetric(t, regeneratedThroughSlot).GetGauge().GetValue())
	require.Equal(t, float64(1), archiveMetric(t, regenerationPhase.WithLabelValues("waiting_for_backfill")).GetGauge().GetValue())

	stopped := make(chan error, 1)
	go func() {
		stopped <- svc.Stop()
	}()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return while the service was waiting for backfill")
	}
}

func TestStart_ResumesAndCountsFailedRounds(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	ctx := t.Context()
	store := testDB.SetupDB(t).(*kv.Store)
	genesisState, keys := util.DeterministicGenesisState(t, 64)
	require.NoError(t, store.InitializeArchiveOrigin(ctx, genesisState))
	buildChain(t, ctx, store, genesisState, keys, 32, nil)
	require.NoError(t, newTestService(t, store).walk(ctx, 32))
	as, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(32), as.RegeneratedThroughSlot)
	as.OriginSlot = 10
	require.NoError(t, store.SaveArchiveStatus(ctx, as))

	cs := startup.NewClockSynchronizer()
	require.NoError(t, cs.SetClock(startup.NewClock(time.Now(), [32]byte{})))
	waiting := make(chan struct{})
	unblock := make(chan struct{})
	svc := New(ctx, store, &mockStateManager{pending: true, handoff: true, markErr: errors.New("handoff failed")}, cs, func() error {
		close(waiting)
		<-unblock
		return nil
	})
	before := archiveMetric(t, roundFailures).GetCounter().GetValue()
	// The handoff fails after the waiter releases.
	go svc.Start()
	<-waiting
	require.Equal(t, float64(as.OriginSlot), archiveMetric(t, archiveOriginSlot).GetGauge().GetValue())
	require.Equal(t, float64(as.RegeneratedThroughSlot), archiveMetric(t, regeneratedThroughSlot).GetGauge().GetValue())
	require.Equal(t, float64(1), archiveMetric(t, regenerationPhase.WithLabelValues("waiting_for_backfill")).GetGauge().GetValue())
	close(unblock)
	require.Eventually(t, func() bool {
		return archiveMetric(t, roundFailures).GetCounter().GetValue() >= before+1
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, float64(1), archiveMetric(t, regenerationPhase.WithLabelValues("walking")).GetGauge().GetValue())
	require.Equal(t, float64(0), archiveMetric(t, regenerationPhase.WithLabelValues("waiting_for_backfill")).GetGauge().GetValue())
	require.NoError(t, svc.Stop())
}

func TestRound_NoHandoffBeforeOriginIsVerified(t *testing.T) {
	setStateDiffExponents([]int{11, 9, 5})
	resetCfg := features.InitWithReset(&features.Flags{EnableStateDiff: true, EnableArchive: true})
	defer resetCfg()

	ctx := t.Context()
	store := testDB.SetupDB(t).(*kv.Store)
	genesisState, _ := util.DeterministicGenesisState(t, 64)
	require.NoError(t, store.InitializeArchiveOrigin(ctx, genesisState))
	saveGenesisBlock(t, ctx, store, genesisState)
	require.NoError(t, store.SaveFinalizedCheckpoint(ctx, &ethpb.Checkpoint{
		Epoch: 0,
		Root:  finalizedRootAtSlot(t, store, 0),
	}))

	sg := &mockStateManager{pending: true, handoff: true}
	svc := New(ctx, store, sg, nil, nil)
	as, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	svc.setStatus(as)

	done, err := svc.round(ctx)
	require.NoError(t, err)
	require.Equal(t, false, done)
	require.Equal(t, true, sg.ArchivePending())
	require.Equal(t, primitives.Slot(0), sg.nextArg)
	persisted, err := store.ArchiveStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, false, persisted.Complete)
}
