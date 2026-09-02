package kv

import (
	"context"
	"flag"
	"slices"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/consensus-types/hdiff"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/OffchainLabs/prysm/v7/testing/util"
	bolt "go.etcd.io/bbolt"
)

var (
	realStateDiffDBDir = flag.String(
		"state-diff-benchmark-db",
		"",
		"path to a beaconchaindata directory containing a state-diff database",
	)
	realStateDiffTargetSlot = flag.Uint64(
		"state-diff-benchmark-slot",
		0,
		"state-diff target slot to benchmark (zero selects the latest finest-level diff)",
	)
	benchmarkHdiffSink hdiff.HdiffBytes
)

type realStateDiffBenchmarkFixture struct {
	exponents  []int
	anchor     state.BeaconState
	target     state.BeaconState
	targetSlot uint64
	targetLvl  int
	anchorSlot uint64
	anchorLvl  int
}

// BenchmarkHdiffDiffRealState measures hdiff creation for the same anchor and target pair used
// when saving a finalized state. The database is opened directly in read-only mode because
// NewKVStore initializes buckets and metadata with write transactions.
func BenchmarkHdiffDiffRealState(b *testing.B) {
	if *realStateDiffDBDir == "" {
		b.Skip("state-diff database not provided; set -state-diff-benchmark-db")
	}

	fixture := setupRealStateDiffBenchmark(b, *realStateDiffDBDir, *realStateDiffTargetSlot)
	b.Logf(
		"read-only database; exponents=%v target_slot=%d target_level=%d anchor_slot=%d anchor_level=%d validators=%d balances=%d",
		fixture.exponents,
		fixture.targetSlot,
		fixture.targetLvl,
		fixture.anchorSlot,
		fixture.anchorLvl,
		fixture.target.NumValidators(),
		fixture.target.BalancesLength(),
	)
	b.ReportAllocs()
	b.ResetTimer()

	var err error
	for b.Loop() {
		benchmarkHdiffSink, err = fixture.diff()
		if err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	b.ReportMetric(float64(fixture.target.NumValidators()), "validators")
	b.ReportMetric(float64(fixture.targetSlot-fixture.anchorSlot), "anchor-slots")
	b.ReportMetric(float64(len(benchmarkHdiffSink.StateDiff)), "state-diff-bytes/op")
	b.ReportMetric(float64(len(benchmarkHdiffSink.ValidatorDiffs)), "validator-diff-bytes/op")
	b.ReportMetric(float64(len(benchmarkHdiffSink.BalancesDiff)), "balance-diff-bytes/op")
}

func setupRealStateDiffBenchmark(
	tb testing.TB,
	databaseDir string,
	requestedTargetSlot uint64,
) *realStateDiffBenchmarkFixture {
	tb.Helper()

	db, err := bolt.Open(StoreDatafilePath(databaseDir), 0o400, &bolt.Options{
		ReadOnly: true,
		Timeout:  5 * time.Second,
	})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := db.Close(); err != nil {
			tb.Error(err)
		}
	})
	if !db.IsReadOnly() {
		tb.Fatal("benchmark database was not opened read-only")
	}

	store := &Store{db: db}
	exponents, err := store.loadStateDiffExponents()
	if err != nil {
		tb.Fatal(err)
	}
	originalFlags := flags.Get()
	benchmarkFlags := *originalFlags
	benchmarkFlags.StateDiffExponents = slices.Clone(exponents)
	flags.Init(&benchmarkFlags)
	tb.Cleanup(func() { flags.Init(originalFlags) })

	offset, err := store.loadOffset()
	if err != nil {
		tb.Fatal(err)
	}
	targetSlot := requestedTargetSlot
	if targetSlot == 0 {
		targetSlot, err = latestSlotForLevel(store, len(exponents)-1)
		if err != nil {
			tb.Fatal(err)
		}
	}

	targetLevel := computeLevel(offset, primitives.Slot(targetSlot))
	if targetLevel <= 0 {
		tb.Fatalf("target slot %d is not stored as a diff", targetSlot)
	}
	anchorSpan := uint64(1) << uint(exponents[targetLevel-1])
	anchorSlot := targetSlot - (targetSlot-offset)%anchorSpan
	anchorLevel := computeLevel(offset, primitives.Slot(anchorSlot))
	if anchorLevel < 0 || anchorLevel >= targetLevel {
		tb.Fatalf("invalid anchor at slot %d level %d for target level %d", anchorSlot, anchorLevel, targetLevel)
	}

	ctx := tb.Context()
	return &realStateDiffBenchmarkFixture{
		exponents:  exponents,
		anchor:     loadBenchmarkState(tb, ctx, store, offset, primitives.Slot(anchorSlot)),
		target:     loadBenchmarkState(tb, ctx, store, offset, primitives.Slot(targetSlot)),
		targetSlot: targetSlot,
		targetLvl:  targetLevel,
		anchorSlot: anchorSlot,
		anchorLvl:  anchorLevel,
	}
}

func (f *realStateDiffBenchmarkFixture) diff() (hdiff.HdiffBytes, error) {
	return hdiff.Diff(f.anchor, f.target)
}

func loadBenchmarkState(
	tb testing.TB,
	ctx context.Context,
	store *Store,
	offset uint64,
	slot primitives.Slot,
) state.BeaconState {
	tb.Helper()
	snapshot, diffs, err := store.getBaseAndDiffChain(offset, slot)
	if err != nil {
		tb.Fatal(err)
	}
	for _, diff := range diffs {
		snapshot, err = hdiff.ApplyDiff(ctx, snapshot, diff)
		if err != nil {
			tb.Fatal(err)
		}
	}
	return snapshot
}

// TestHdiffDiffRealStateBenchmarkWorkload keeps the benchmark setup and measured operation
// covered by regular CI, where benchmarks are not executed.
func TestHdiffDiffRealStateBenchmarkWorkload(t *testing.T) {
	databaseDir, expectedAnchor, expectedTarget := createRealStateDiffBenchmarkDB(t)

	tests := []struct {
		name                string
		requestedTargetSlot uint64
	}{
		{name: "latest target", requestedTargetSlot: 0},
		{name: "explicit target", requestedTargetSlot: 96},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := setupRealStateDiffBenchmark(t, databaseDir, test.requestedTargetSlot)
			require.DeepEqual(t, []int{7, 6, 5}, fixture.exponents)
			require.Equal(t, uint64(64), fixture.anchorSlot)
			require.Equal(t, 1, fixture.anchorLvl)
			require.Equal(t, uint64(96), fixture.targetSlot)
			require.Equal(t, 2, fixture.targetLvl)

			loadedAnchor, err := fixture.anchor.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, expectedAnchor, loadedAnchor)
			loadedTarget, err := fixture.target.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, expectedTarget, loadedTarget)

			diff, err := fixture.diff()
			require.NoError(t, err)
			require.NotEqual(t, 0, len(diff.StateDiff))
			require.NotEqual(t, 0, len(diff.ValidatorDiffs))
			require.NotEqual(t, 0, len(diff.BalancesDiff))

			reconstructed, err := hdiff.ApplyDiff(t.Context(), fixture.anchor.Copy(), diff)
			require.NoError(t, err)
			reconstructedSSZ, err := reconstructed.MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, expectedTarget, reconstructedSSZ)
		})
	}
}

func createRealStateDiffBenchmarkDB(t *testing.T) (string, []byte, []byte) {
	t.Helper()

	originalFlags := flags.Get()
	fixtureFlags := *originalFlags
	fixtureFlags.StateDiffExponents = []int{7, 6, 5}
	flags.Init(&fixtureFlags)
	defer flags.Init(originalFlags)
	resetFeatures := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer resetFeatures()

	databaseDir := t.TempDir()
	store, err := NewKVStore(t.Context(), databaseDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, store.Close())
	}()

	anchor, _ := util.DeterministicGenesisStateAltair(t, 8)
	require.NoError(t, store.initializeStateDiff(0, anchor))

	middle := anchor.Copy()
	require.NoError(t, middle.SetSlot(64))
	validators := middle.Validators()
	validators[0].Slashed = true
	require.NoError(t, middle.SetValidators(validators))
	balances := middle.Balances()
	balances[0]--
	require.NoError(t, middle.SetBalances(balances))
	inactivityScores, err := middle.InactivityScores()
	require.NoError(t, err)
	inactivityScores[0]++
	require.NoError(t, middle.SetInactivityScores(inactivityScores))
	participation, err := middle.CurrentEpochParticipation()
	require.NoError(t, err)
	participation[0] = 1
	require.NoError(t, middle.SetCurrentParticipationBits(participation))
	require.NoError(t, store.saveStateByDiff(t.Context(), middle))
	expectedAnchor, err := middle.MarshalSSZ()
	require.NoError(t, err)

	target := middle.Copy()
	require.NoError(t, target.SetSlot(96))
	validators = target.Validators()
	validators[1].Slashed = true
	require.NoError(t, target.SetValidators(validators))
	balances = target.Balances()
	balances[1]--
	require.NoError(t, target.SetBalances(balances))
	inactivityScores, err = target.InactivityScores()
	require.NoError(t, err)
	inactivityScores[1]++
	require.NoError(t, target.SetInactivityScores(inactivityScores))
	participation, err = target.CurrentEpochParticipation()
	require.NoError(t, err)
	participation[1] = 1
	require.NoError(t, target.SetCurrentParticipationBits(participation))
	require.NoError(t, store.saveStateByDiff(t.Context(), target))

	expectedTarget, err := target.MarshalSSZ()
	require.NoError(t, err)
	return databaseDir, expectedAnchor, expectedTarget
}
