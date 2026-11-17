// Offline examples (cold clears anchors, not the Bolt mmap/page cache; tree saves overwrite keys):
// go test ./beacon-chain/db/kv -run 'TestStateDiffReadSaveBenchmarkWorkload$' -bench 'BenchmarkStateDiffReadRealState' -benchtime 5x -timeout 0 -args -state-diff-benchmark-db=/copy/beaconchaindata -state-diff-benchmark-slots=32,64
// go test ./beacon-chain/db/kv -run 'TestStateDiffRebuildProfile$' -v -timeout 0 -args -state-diff-benchmark-db=/copy/beaconchaindata -state-diff-rebuild-dst=/tmp/rebuilt -state-diff-rebuild-exponents=16,13,11 -state-diff-rebuild-csv=rb.csv -state-diff-rebuild-verify-rate=0.05
package kv

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/config/params"
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
	realStateDiffSlots             = flag.String("state-diff-benchmark-slots", "", "CSV of state-diff slots (empty selects the latest slot of each level)")
	realStateDiffNetwork           = flag.String("state-diff-benchmark-network", "mainnet", "source network: mainnet or hoodi")
	realStateDiffScratchDir        = flag.String("state-diff-benchmark-scratch-dir", "", "directory for writable benchmark stores (empty uses a temporary directory)")
	realStateDiffHTR               = flag.Bool("state-diff-benchmark-htr", false, "include HashTreeRoot in read timing")
	realStateDiffHotN              = flag.Int("state-diff-benchmark-hot-n", 16, "number of hot snapshots to clear")
	realStateDiffRebuildDst        = flag.String("state-diff-rebuild-dst", "", "destination directory for rebuilt state-diff tree")
	realStateDiffRebuildExponents  = flag.String("state-diff-rebuild-exponents", "", "CSV of destination exponents")
	realStateDiffRebuildToSlot     = flag.Uint64("state-diff-rebuild-to-slot", 0, "last source boundary to rebuild (zero selects latest)")
	realStateDiffRebuildCSV        = flag.String("state-diff-rebuild-csv", "", "rebuild per-boundary CSV output")
	realStateDiffRebuildVerifyRate = flag.Float64("state-diff-rebuild-verify-rate", 0, "fraction of rebuilt states to verify")
	realStateDiffRebuildCompareDB  = flag.String("state-diff-rebuild-compare-db", "", "directory containing a native destination-profile database")
	realStateDiffAuditCSV          = flag.String("state-diff-audit-csv", "", "audit per-level CSV output")
	benchmarkHdiffSink             hdiff.HdiffBytes
	benchmarkStateSink             state.BeaconState
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

	benchmarkNetwork(b)
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
	runtime.GC()
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

	store, exponents, offset := openBenchmarkSource(tb, databaseDir)
	targetSlot := requestedTargetSlot
	var err error
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

func openBenchmarkSource(tb testing.TB, databaseDir string) (*Store, []int, uint64) {
	return openBenchmarkSourceWithCache(tb, databaseDir, true)
}

func openBenchmarkSourceWithCache(tb testing.TB, databaseDir string, populateCache bool) (*Store, []int, uint64) {
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
	if populateCache {
		store.stateDiffCache, err = populateStateDiffCacheFromDB(store, offset)
		if err != nil {
			tb.Fatal(err)
		}
	}
	return store, exponents, offset
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

func benchmarkNetwork(tb testing.TB) {
	tb.Helper()
	var cfg = params.MainnetConfig()
	switch *realStateDiffNetwork {
	case "mainnet":
	case "hoodi":
		cfg = params.HoodiConfig()
	default:
		tb.Fatalf("unknown state-diff network %q", *realStateDiffNetwork)
	}
	undo, err := params.SetActiveWithUndo(cfg)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := undo(); err != nil {
			tb.Error(err)
		}
	})
}

func benchmarkSlots(tb testing.TB, store *Store, exponents []int, raw string) []uint64 {
	tb.Helper()
	if raw == "" {
		var slots []uint64
		for level := range exponents {
			slot, err := latestSlotForLevel(store, level)
			if err != nil {
				tb.Fatal(err)
			}
			if !slices.Contains(slots, slot) {
				slots = append(slots, slot)
			}
		}
		return slots
	}
	var slots []uint64
	for _, part := range strings.Split(raw, ",") {
		slot, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err != nil {
			tb.Fatalf("invalid slot %q: %v", part, err)
		}
		slots = append(slots, slot)
	}
	return slots
}

func benchmarkScratch(tb testing.TB, directory string, offset uint64, initial state.BeaconState) *Store {
	tb.Helper()
	if directory == "" {
		directory = tb.TempDir()
	}
	reset := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	tb.Cleanup(reset)
	store, err := NewKVStore(tb.Context(), directory)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := store.Close(); err != nil {
			tb.Error(err)
		}
	})
	if err := store.initializeStateDiff(primitives.Slot(offset), initial); err != nil {
		tb.Fatal(err)
	}
	return store
}

func benchmarkScratchPath(tb testing.TB, prefix string) string {
	tb.Helper()
	if *realStateDiffScratchDir == "" {
		return ""
	}
	if err := os.MkdirAll(*realStateDiffScratchDir, 0o700); err != nil {
		tb.Fatal(err)
	}
	directory, err := os.MkdirTemp(*realStateDiffScratchDir, prefix+"-")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			tb.Error(err)
		}
	})
	return directory
}

func treeEntrySizes(store *Store, level int, slot uint64) ([4]int, error) {
	var sizes [4]int
	err := store.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		key := makeKeyForStateDiffTree(level, slot)
		suffixes := [][]byte{nil, []byte(stateSuffix), []byte(validatorSuffix), []byte(balancesSuffix)}
		for i, suffix := range suffixes {
			if value := bucket.Get(append(slices.Clone(key), suffix...)); value != nil {
				sizes[i] = len(value)
			}
		}
		return nil
	})
	return sizes, err
}

func clearBenchmarkMemo(cache *stateDiffCache) {
	cache.Lock()
	cache.memo = [anchorMemoSize]anchorMemoEntry{}
	cache.Unlock()
}

func benchmarkRead(store *Store, slot uint64, mode string, hash bool) (int, int, error) {
	if mode != "cold" {
		if err := benchmarkReadSeed(store, slot); err != nil {
			return 0, 0, err
		}
		if mode == "anchor" {
			clearBenchmarkMemo(store.stateDiffCache)
		}
	} else {
		store.stateDiffCache.clearAnchors()
	}
	_, chain, err := store.getBaseAndDiffChain(store.getOffset(), primitives.Slot(slot))
	if err != nil {
		return 0, 0, err
	}
	bytes := 0
	if mode == "cold" || computeLevel(store.getOffset(), primitives.Slot(slot)) == 0 && len(flags.Get().StateDiffExponents) == 1 {
		baseSlot := slot - (slot-store.getOffset())%(uint64(1)<<uint(flags.Get().StateDiffExponents[0]))
		sizes, err := treeEntrySizes(store, 0, baseSlot)
		if err != nil {
			return 0, 0, err
		}
		bytes += sizes[0]
	}
	for _, diff := range chain {
		bytes += len(diff.StateDiff) + len(diff.ValidatorDiffs) + len(diff.BalancesDiff)
	}
	st, err := store.stateByDiff(context.Background(), primitives.Slot(slot))
	if err != nil {
		return 0, 0, err
	}
	benchmarkStateSink = st
	if hash {
		if _, err := st.HashTreeRoot(context.Background()); err != nil {
			return 0, 0, err
		}
	}
	return len(chain), bytes, nil
}

func BenchmarkStateDiffReadRealState(b *testing.B) {
	if *realStateDiffDBDir == "" {
		b.Skip("set -state-diff-benchmark-db")
	}
	benchmarkNetwork(b)
	store, exponents, offset := openBenchmarkSource(b, *realStateDiffDBDir)
	for _, slot := range benchmarkSlots(b, store, exponents, *realStateDiffSlots) {
		level := computeLevel(offset, primitives.Slot(slot))
		if level < 0 {
			b.Fatalf("slot %d is not a boundary", slot)
		}
		for _, mode := range []string{"cold", "anchor", "memo"} {
			name := fmt.Sprintf("slot=%d/level=%d/cache=%s", slot, level, mode)
			b.Run(name, func(b *testing.B) {
				diffs, bytes, err := benchmarkRead(store, slot, mode, *realStateDiffHTR)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				runtime.GC()
				b.ResetTimer()
				for b.Loop() {
					b.StopTimer()
					if mode == "anchor" {
						err = benchmarkReadSeed(store, slot)
						if err == nil {
							clearBenchmarkMemo(store.stateDiffCache)
						}
					} else if mode == "cold" {
						store.stateDiffCache.clearAnchors()
					}
					if err != nil {
						b.Fatal(err)
					}
					b.StartTimer()
					benchmarkStateSink, err = store.stateByDiff(b.Context(), primitives.Slot(slot))
					if err != nil {
						b.Fatal(err)
					}
					if *realStateDiffHTR {
						if _, err := benchmarkStateSink.HashTreeRoot(b.Context()); err != nil {
							b.Fatal(err)
						}
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(diffs), "diffs/op")
				b.ReportMetric(float64(bytes), "chain-bytes/op")
				b.ReportMetric(float64(level), "level")
			})
		}
	}
}

func benchmarkReadSeed(store *Store, slot uint64) error {
	store.stateDiffCache.clearAnchors()
	offset := store.getOffset()
	if computeLevel(offset, primitives.Slot(slot)) == 0 && len(flags.Get().StateDiffExponents) > 1 {
		st, err := store.stateByDiff(context.Background(), primitives.Slot(slot))
		if err != nil {
			return err
		}
		return store.stateDiffCache.setAnchor(0, st)
	}
	for level := 0; level < computeLevel(offset, primitives.Slot(slot)); level++ {
		span := uint64(1) << uint(flags.Get().StateDiffExponents[level])
		ancestorSlot := slot - (slot-offset)%span
		st, err := store.stateByDiff(context.Background(), primitives.Slot(ancestorSlot))
		if err != nil {
			return err
		}
		if err := store.stateDiffCache.setAnchor(level, st); err != nil {
			return err
		}
	}
	return nil
}

func benchmarkSaveSetup(tb testing.TB, source *Store, offset, slot uint64, directory string) (*Store, state.BeaconState, int) {
	tb.Helper()
	level := computeLevel(offset, primitives.Slot(slot))
	if level < 0 {
		tb.Fatalf("slot %d is not a boundary", slot)
	}
	initial, err := source.stateByDiff(tb.Context(), primitives.Slot(offset))
	if err != nil {
		tb.Fatal(err)
	}
	target, err := source.stateByDiff(tb.Context(), primitives.Slot(slot))
	if err != nil {
		tb.Fatal(err)
	}
	var anchor state.BeaconState
	anchorLevel := -1
	if level > 0 {
		span := uint64(1) << uint(flags.Get().StateDiffExponents[level-1])
		anchorSlot := slot - (slot-offset)%span
		anchorLevel = computeLevel(offset, primitives.Slot(anchorSlot))
		anchor, err = source.stateByDiff(tb.Context(), primitives.Slot(anchorSlot))
		if err != nil {
			tb.Fatal(err)
		}
	}
	dst := benchmarkScratch(tb, directory, offset, initial)
	if anchor != nil {
		if err := dst.stateDiffCache.setAnchor(anchorLevel, anchor); err != nil {
			tb.Fatal(err)
		}
	}
	return dst, target, level
}

func BenchmarkStateDiffSaveRealState(b *testing.B) {
	if *realStateDiffDBDir == "" {
		b.Skip("set -state-diff-benchmark-db")
	}
	benchmarkNetwork(b)
	source, exponents, offset := openBenchmarkSource(b, *realStateDiffDBDir)
	for _, slot := range benchmarkSlots(b, source, exponents, *realStateDiffSlots) {
		for _, anchorMode := range []string{"cold", "warm"} {
			b.Run(fmt.Sprintf("slot=%d/level=%d/anchor=%s", slot, computeLevel(offset, primitives.Slot(slot)), anchorMode), func(b *testing.B) {
				directory := benchmarkScratchPath(b, fmt.Sprintf("save-%d", slot))
				dst, target, level := benchmarkSaveSetup(b, source, offset, slot, directory)
				if level > 0 && anchorMode == "warm" {
					span := uint64(1) << uint(exponents[level-1])
					anchorSlot := slot - (slot-offset)%span
					anchorLevel := computeLevel(offset, primitives.Slot(anchorSlot))
					if dst.stateDiffCache.getAnchor(anchorLevel, withExactSlot(primitives.Slot(anchorSlot))) == nil {
						b.Fatal("failed to prime anchor memo")
					}
				}
				b.ReportAllocs()
				runtime.GC()
				b.ResetTimer()
				for b.Loop() {
					if anchorMode == "cold" {
						b.StopTimer()
						clearBenchmarkMemo(dst.stateDiffCache)
						b.StartTimer()
					}
					if err := dst.saveStateByDiff(b.Context(), target); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				sizes, err := treeEntrySizes(dst, level, slot)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(sizes[0]+sizes[1]+sizes[2]+sizes[3]), "entry-bytes/op")
			})
		}
	}
}

func benchmarkHotSave(store *Store, ctx context.Context, st state.BeaconState, index int) error {
	var root [32]byte
	binary.LittleEndian.PutUint64(root[:], uint64(index))
	return store.SaveHotStateSnapshot(ctx, st, root)
}

func benchmarkHotSeed(store *Store, ctx context.Context, st state.BeaconState, n int) error {
	for i := range n {
		if err := benchmarkHotSave(store, ctx, st, i); err != nil {
			return err
		}
	}
	return nil
}

func benchmarkHotClear(store *Store, ctx context.Context, st state.BeaconState, n int) error {
	if err := benchmarkHotSeed(store, ctx, st, n); err != nil {
		return err
	}
	return store.ClearHotStateSnapshots(ctx)
}

func BenchmarkHotStateSnapshotRealState(b *testing.B) {
	if *realStateDiffDBDir == "" {
		b.Skip("set -state-diff-benchmark-db")
	}
	if *realStateDiffHotN < 0 {
		b.Fatal("state-diff-benchmark-hot-n must be nonnegative")
	}
	benchmarkNetwork(b)
	source, exponents, offset := openBenchmarkSource(b, *realStateDiffDBDir)
	slot, err := latestBenchmarkBoundary(source, offset, exponents)
	if err != nil {
		b.Fatal(err)
	}
	initial := loadBenchmarkState(b, b.Context(), source, offset, primitives.Slot(offset))
	st := loadBenchmarkState(b, b.Context(), source, offset, primitives.Slot(slot))
	directory := benchmarkScratchPath(b, "hot")
	dst := benchmarkScratch(b, directory, offset, initial)
	b.Run("save", func(b *testing.B) {
		runtime.GC()
		b.ResetTimer()
		index := 0
		for b.Loop() {
			if err := benchmarkHotSave(dst, b.Context(), st, index); err != nil {
				b.Fatal(err)
			}
			index++
		}
	})
	b.Run(fmt.Sprintf("clear/n=%d", *realStateDiffHotN), func(b *testing.B) {
		runtime.GC()
		for b.Loop() {
			b.StopTimer()
			if err := benchmarkHotSeed(dst, b.Context(), st, *realStateDiffHotN); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			if err := dst.ClearHotStateSnapshots(b.Context()); err != nil {
				b.Fatal(err)
			}
		}
	})
}

type treeAuditLevel struct {
	level             int
	expected, present int
	holes, orphans    []uint64
	pendingHoles      []uint64
	incomplete        []uint64
	bytes, maxBytes   uint64
	maxSlot           uint64
	latestSlot        uint64
	componentBytes    [4]uint64
	componentMax      [4]uint64
	componentMaxSlot  [4]uint64
}

type treeAudit struct {
	offset, last uint64
	exponents    []int
	status       []byte
	archive      *ArchiveStatus
	corruptKeys  []string
	levels       []treeAuditLevel
}

func validBenchmarkPadding(key []byte) bool {
	for _, b := range key[stateDiffTreeKeySlotEnd:stateDiffTreeKeyLength] {
		if b != 0 {
			return false
		}
	}
	return true
}

func latestBenchmarkBoundary(store *Store, offset uint64, exponents []int) (uint64, error) {
	if !slices.Equal(exponents, flags.Get().StateDiffExponents) {
		return 0, fmt.Errorf("state-diff exponents differ from active flags")
	}
	var last uint64
	found := false
	err := store.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		cursor := bucket.Cursor()
		for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
			if len(key) < stateDiffTreeKeyLength || int(key[0]) >= len(exponents) {
				continue
			}
			if !validBenchmarkPadding(key) {
				continue
			}
			level, slot := int(key[0]), stateDiffTreeKeySlot(key)
			if computeLevel(offset, primitives.Slot(slot)) != level {
				continue
			}
			suffix := string(key[stateDiffTreeKeyLength:])
			if level == 0 && suffix != "" || level > 0 && suffix != stateSuffix && suffix != validatorSuffix && suffix != balancesSuffix {
				continue
			}
			if !found || slot > last {
				last, found = slot, true
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, fmt.Errorf("no state-diff boundaries found")
	}
	return last, nil
}

func auditStateDiffTree(store *Store, offset uint64, exponents []int) (treeAudit, error) {
	result := treeAudit{offset: offset, exponents: slices.Clone(exponents), levels: make([]treeAuditLevel, len(exponents))}
	for i := range result.levels {
		result.levels[i].level = i
	}
	last, err := latestBenchmarkBoundary(store, offset, exponents)
	if err != nil {
		return result, err
	}
	result.last = last
	type entry struct {
		mask  uint8
		bytes uint64
	}
	present := make([]map[uint64]*entry, len(exponents))
	for i := range present {
		present[i] = make(map[uint64]*entry)
	}
	err = store.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if bucket == nil {
			return bolt.ErrBucketNotFound
		}
		result.status = slices.Clone(bucket.Get(archiveStatusKey))
		if len(result.status) > 0 {
			var err error
			result.archive, err = decodeArchiveStatus(result.status)
			if err != nil {
				return err
			}
		}
		cursor := bucket.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			switch string(key) {
			case string(offsetKey), string(exponentsKey), string(archiveStatusKey):
				continue
			}
			if len(key) < stateDiffTreeKeyLength || int(key[0]) >= len(exponents) ||
				!validBenchmarkPadding(key) {
				result.corruptKeys = append(result.corruptKeys, fmt.Sprintf("%x", key))
				continue
			}
			level, slot := int(key[0]), stateDiffTreeKeySlot(key)
			var component int
			switch string(key[stateDiffTreeKeyLength:]) {
			case "":
				component = 0
			case stateSuffix:
				component = 1
			case validatorSuffix:
				component = 2
			case balancesSuffix:
				component = 3
			default:
				result.levels[level].orphans = append(result.levels[level].orphans, slot)
				continue
			}
			item := present[level][slot]
			if item == nil {
				item = &entry{}
				present[level][slot] = item
			}
			item.mask |= 1 << component
			size := uint64(len(value))
			item.bytes += size
			row := &result.levels[level]
			row.componentBytes[component] += size
			if size > row.componentMax[component] {
				row.componentMax[component], row.componentMaxSlot[component] = size, slot
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	span := uint64(1) << uint(exponents[len(exponents)-1])
	for slot := offset; slot <= last; slot += span {
		level := computeLevel(offset, primitives.Slot(slot))
		if level < 0 {
			return result, fmt.Errorf("slot %d is not a boundary", slot)
		}
		row := &result.levels[level]
		row.expected++
		if present[level][slot] == nil {
			if result.archive != nil && !result.archive.Complete && slot > uint64(result.archive.RegeneratedThroughSlot) {
				row.pendingHoles = append(row.pendingHoles, slot)
			} else {
				row.holes = append(row.holes, slot)
			}
		}
		if last-slot < span {
			break
		}
	}
	for level, entries := range present {
		row := &result.levels[level]
		for slot, item := range entries {
			row.present++
			if slot > row.latestSlot {
				row.latestSlot = slot
			}
			if slot < offset || slot > last || computeLevel(offset, primitives.Slot(slot)) != level {
				row.orphans = append(row.orphans, slot)
			}
			if level == 0 && item.mask != 1 || level > 0 && item.mask != 14 {
				row.incomplete = append(row.incomplete, slot)
			}
			row.bytes += item.bytes
			if item.bytes > row.maxBytes {
				row.maxBytes, row.maxSlot = item.bytes, slot
			}
		}
		slices.Sort(row.holes)
		slices.Sort(row.orphans)
		row.orphans = slices.Compact(row.orphans)
		slices.Sort(row.incomplete)
	}
	return result, nil
}

func auditInvalid(a treeAudit) bool {
	if len(a.corruptKeys) > 0 {
		return true
	}
	for _, row := range a.levels {
		if len(row.holes)+len(row.orphans)+len(row.incomplete) > 0 {
			return true
		}
	}
	return false
}

func firstAuditSlots(slots []uint64) []uint64 {
	if len(slots) > 20 {
		return slots[:20]
	}
	return slots
}

func firstAuditKeys(keys []string) []string {
	if len(keys) > 20 {
		return keys[:20]
	}
	return keys
}

func writeBenchmarkCSV(path string, header []string, rows [][]string) error {
	if path == "" {
		return nil
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	err = writer.Write(header)
	if err == nil {
		err = writer.WriteAll(rows)
	}
	if err == nil {
		err = writer.Error()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func logTreeAudit(t *testing.T, a treeAudit, csvPath string) {
	t.Helper()
	t.Logf("offset=%d exponents=%v archive-status=%+v last=%d corrupt-keys=%d %v",
		a.offset, a.exponents, a.archive, a.last, len(a.corruptKeys), firstAuditKeys(a.corruptKeys))
	var rows [][]string
	for _, row := range a.levels {
		avg := float64(0)
		if row.present > 0 {
			avg = float64(row.bytes) / float64(row.present)
		}
		t.Logf("level=%d expected=%d present=%d holes=%d %v pending-holes=%d %v orphans=%d %v incomplete=%d %v bytes=%d avg=%.1f max=%d@%d latest=%d components=[%d %d %d %d] component-max=[%d@%d %d@%d %d@%d %d@%d]",
			row.level, row.expected, row.present, len(row.holes), firstAuditSlots(row.holes),
			len(row.pendingHoles), firstAuditSlots(row.pendingHoles), len(row.orphans), firstAuditSlots(row.orphans),
			len(row.incomplete), firstAuditSlots(row.incomplete), row.bytes, avg, row.maxBytes, row.maxSlot, row.latestSlot,
			row.componentBytes[0], row.componentBytes[1], row.componentBytes[2], row.componentBytes[3],
			row.componentMax[0], row.componentMaxSlot[0], row.componentMax[1], row.componentMaxSlot[1],
			row.componentMax[2], row.componentMaxSlot[2], row.componentMax[3], row.componentMaxSlot[3])
		rows = append(rows, []string{
			strconv.Itoa(row.level), strconv.Itoa(row.expected), strconv.Itoa(row.present),
			strconv.Itoa(len(row.holes)), strconv.Itoa(len(row.pendingHoles)), strconv.Itoa(len(row.orphans)), strconv.Itoa(len(row.incomplete)),
			strconv.FormatUint(row.bytes, 10), strconv.FormatFloat(avg, 'f', 1, 64),
			strconv.FormatUint(row.maxBytes, 10), strconv.FormatUint(row.maxSlot, 10),
			strconv.FormatUint(row.latestSlot, 10),
			strconv.FormatUint(row.componentBytes[0], 10), strconv.FormatUint(row.componentBytes[1], 10),
			strconv.FormatUint(row.componentBytes[2], 10), strconv.FormatUint(row.componentBytes[3], 10),
			strconv.FormatUint(row.componentMax[0], 10), strconv.FormatUint(row.componentMaxSlot[0], 10),
			strconv.FormatUint(row.componentMax[1], 10), strconv.FormatUint(row.componentMaxSlot[1], 10),
			strconv.FormatUint(row.componentMax[2], 10), strconv.FormatUint(row.componentMaxSlot[2], 10),
			strconv.FormatUint(row.componentMax[3], 10), strconv.FormatUint(row.componentMaxSlot[3], 10),
			strconv.FormatUint(a.offset, 10), formatStateDiffExponents(a.exponents), fmt.Sprintf("%x", a.status),
		})
	}
	if err := writeBenchmarkCSV(csvPath, []string{
		"level", "expected", "present", "holes", "pending_holes", "orphans", "incomplete", "bytes", "avg_bytes", "max_bytes", "max_bytes_slot", "latest_slot",
		"snapshot_bytes", "state_bytes", "validator_bytes", "balance_bytes",
		"snapshot_max", "snapshot_max_slot", "state_max", "state_max_slot", "validator_max", "validator_max_slot", "balance_max", "balance_max_slot",
		"offset", "exponents", "archive_status",
	}, rows); err != nil {
		t.Fatal(err)
	}
	if auditInvalid(a) {
		t.Error("state-diff tree has holes, orphans, corrupt keys or incomplete entries")
	}
}

func TestStateDiffTreeAuditRealDB(t *testing.T) {
	if *realStateDiffDBDir == "" {
		t.Skip("set -state-diff-benchmark-db")
	}
	benchmarkNetwork(t)
	// Audit must inspect corrupt keys without cache initialization rejecting them first.
	store, exponents, offset := openBenchmarkSourceWithCache(t, *realStateDiffDBDir, false)
	a, err := auditStateDiffTree(store, offset, exponents)
	require.NoError(t, err)
	logTreeAudit(t, a, *realStateDiffAuditCSV)
}

type rebuildRow struct {
	slot, total uint64
	level       int
	kind        string
	sizes       [4]int
	save, read  time.Duration
	verified    bool
	compare     string
}

func (r rebuildRow) csv() []string {
	return []string{
		strconv.FormatUint(r.slot, 10), strconv.Itoa(r.level), r.kind,
		strconv.Itoa(r.sizes[1]), strconv.Itoa(r.sizes[2]), strconv.Itoa(r.sizes[3]), strconv.Itoa(r.sizes[0]),
		strconv.FormatUint(r.total, 10), strconv.FormatFloat(float64(r.save)/float64(time.Millisecond), 'f', 3, 64),
		strconv.FormatFloat(float64(r.read)/float64(time.Millisecond), 'f', 3, 64),
		strconv.FormatBool(r.verified), r.compare,
	}
}

func setBenchmarkExponents(exponents []int) {
	cfg := *flags.Get()
	cfg.StateDiffExponents = slices.Clone(exponents)
	flags.Init(&cfg)
}

func parseBenchmarkExponents(text string) ([]int, error) {
	var exponents []int
	for _, part := range strings.Split(text, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("invalid exponent %q: %w", part, err)
		}
		exponents = append(exponents, n)
	}
	encoded, err := encodeStateDiffExponents(exponents)
	if err != nil {
		return nil, err
	}
	return decodeStateDiffExponents(encoded)
}

func compareTreeEntry(dst, other *Store, level int, slot uint64) (string, error) {
	key := makeKeyForStateDiffTree(level, slot)
	suffixes := [][]byte{nil}
	if level != 0 {
		suffixes = [][]byte{[]byte(stateSuffix), []byte(validatorSuffix), []byte(balancesSuffix)}
	}
	result := "equal"
	for _, suffix := range suffixes {
		entryKey := append(slices.Clone(key), suffix...)
		var actual, expected []byte
		for _, item := range []struct {
			store *Store
			value *[]byte
		}{{dst, &actual}, {other, &expected}} {
			if err := item.store.db.View(func(tx *bolt.Tx) error {
				bucket := tx.Bucket(stateDiffBucket)
				if bucket == nil {
					return bolt.ErrBucketNotFound
				}
				*item.value = slices.Clone(bucket.Get(entryKey))
				return nil
			}); err != nil {
				return "", err
			}
		}
		if expected == nil {
			return "missing", nil
		}
		if !slices.Equal(actual, expected) {
			result = "different"
		}
	}
	return result, nil
}

func verifyRebuildState(ctx context.Context, dst *Store, st state.BeaconState) error {
	reader := &Store{db: dst.db}
	cache, err := newStateDiffCache(reader)
	if err != nil {
		return err
	}
	reader.stateDiffCache = cache
	got, err := reader.stateByDiff(ctx, st.Slot())
	if err != nil {
		return err
	}
	gotSSZ, err := got.MarshalSSZ()
	if err != nil {
		return err
	}
	expected, err := st.MarshalSSZ()
	if err != nil {
		return err
	}
	if !slices.Equal(gotSSZ, expected) {
		return fmt.Errorf("persisted state differs at slot %d", st.Slot())
	}
	return nil
}

func rebuildProfile(tb testing.TB, source *Store, sourceExponents []int, offset uint64, targetExponents []int, toSlot uint64, dstDir, compareDir, csvPath string, verifyRate float64) ([]rebuildRow, *Store) {
	tb.Helper()
	if verifyRate < 0 || verifyRate > 1 {
		tb.Fatal("verify rate must be in [0,1]")
	}
	if slices.Equal(targetExponents, sourceExponents) {
		tb.Fatal("destination exponents must differ from source")
	}
	if targetExponents[len(targetExponents)-1] < sourceExponents[len(sourceExponents)-1] {
		tb.Fatal("destination boundaries must be source boundaries")
	}
	last, err := latestBenchmarkBoundary(source, offset, sourceExponents)
	if err != nil {
		tb.Fatal(err)
	}
	if toSlot == 0 {
		toSlot = last
	}
	if toSlot > last || toSlot < offset {
		tb.Fatalf("rebuild end %d outside source range [%d,%d]", toSlot, offset, last)
	}
	if dstDir == "" {
		tb.Fatal("destination directory is required")
	}
	if _, err := os.Stat(StoreDatafilePath(dstDir)); err == nil {
		tb.Fatalf("destination database already exists at %s", dstDir)
	} else if !os.IsNotExist(err) {
		tb.Fatal(err)
	}
	var compare *Store
	if compareDir != "" {
		db, err := bolt.Open(StoreDatafilePath(compareDir), 0o400, &bolt.Options{ReadOnly: true, Timeout: 5 * time.Second})
		if err != nil {
			tb.Fatal(err)
		}
		compare = &Store{db: db}
		tb.Cleanup(func() {
			if err := db.Close(); err != nil {
				tb.Error(err)
			}
		})
		compareExponents, err := compare.loadStateDiffExponents()
		if err != nil {
			tb.Fatal(err)
		}
		compareOffset, err := compare.loadOffset()
		if err != nil {
			tb.Fatal(err)
		}
		if !slices.Equal(compareExponents, targetExponents) || compareOffset != offset {
			tb.Fatal("comparison database profile or offset differs")
		}
	}
	initial, err := source.stateByDiff(tb.Context(), primitives.Slot(offset))
	if err != nil {
		tb.Fatal(err)
	}
	originalFlags := flags.Get()
	tb.Cleanup(func() { flags.Init(originalFlags) })
	var csvFile *os.File
	var writer *csv.Writer
	if csvPath != "" {
		csvFile, err = os.Create(csvPath)
		if err != nil {
			tb.Fatal(err)
		}
		writer = csv.NewWriter(csvFile)
		tb.Cleanup(func() {
			writer.Flush()
			if err := writer.Error(); err != nil {
				tb.Error(err)
			}
			if err := csvFile.Close(); err != nil {
				tb.Error(err)
			}
		})
		if err := writer.Write([]string{"slot", "level", "kind", "state_bytes", "validator_bytes", "balance_bytes", "snapshot_bytes", "total_bytes", "save_ms", "source_read_ms", "verified", "compare"}); err != nil {
			tb.Fatal(err)
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			tb.Fatal(err)
		}
	}
	setBenchmarkExponents(targetExponents)
	dst := benchmarkScratch(tb, dstDir, offset, initial)
	rng := rand.New(rand.NewSource(1))
	var rows []rebuildRow
	span := uint64(1) << uint(targetExponents[len(targetExponents)-1])
	for slot := offset; slot <= toSlot; slot += span {
		setBenchmarkExponents(sourceExponents)
		start := time.Now()
		st, err := source.stateByDiff(tb.Context(), primitives.Slot(slot))
		readTime := time.Since(start)
		if err != nil {
			tb.Fatalf("read source slot %d: %v", slot, err)
		}
		sourceLevel := computeLevel(offset, primitives.Slot(slot))
		if sourceLevel < 0 {
			tb.Fatalf("destination slot %d is not a source boundary", slot)
		}
		if sourceLevel < len(sourceExponents)-1 {
			if err := source.stateDiffCache.setAnchor(sourceLevel, st); err != nil {
				tb.Fatal(err)
			}
		}
		// Source reads and destination writes switch the process-wide exponent profile serially.
		setBenchmarkExponents(targetExponents)
		level := computeLevel(offset, primitives.Slot(slot))
		start = time.Now()
		if slot != offset {
			err = dst.saveStateByDiff(tb.Context(), st)
		}
		saveTime := time.Since(start)
		if err != nil {
			tb.Fatalf("save destination slot %d: %v", slot, err)
		}
		sizes, err := treeEntrySizes(dst, level, slot)
		if err != nil {
			tb.Fatal(err)
		}
		row := rebuildRow{slot: slot, level: level, sizes: sizes, save: saveTime, read: readTime, compare: "not-requested"}
		row.kind = "diff"
		if level == 0 {
			row.kind = "snapshot"
		}
		for _, n := range sizes {
			row.total += uint64(n)
		}
		if rng.Float64() < verifyRate {
			if err := verifyRebuildState(tb.Context(), dst, st); err != nil {
				tb.Fatalf("verify slot %d: %v", slot, err)
			}
			row.verified = true
		}
		if compare != nil {
			row.compare, err = compareTreeEntry(dst, compare, level, slot)
			if err != nil {
				tb.Fatal(err)
			}
		}
		rows = append(rows, row)
		if writer != nil {
			if err := writer.Write(row.csv()); err != nil {
				tb.Fatal(err)
			}
			writer.Flush()
			if err := writer.Error(); err != nil {
				tb.Fatal(err)
			}
		}
		if toSlot-slot < span {
			break
		}
	}
	return rows, dst
}

func TestStateDiffRebuildProfile(t *testing.T) {
	if *realStateDiffDBDir == "" || *realStateDiffRebuildDst == "" || *realStateDiffRebuildExponents == "" {
		t.Skip("set -state-diff-benchmark-db, -state-diff-rebuild-dst and -state-diff-rebuild-exponents")
	}
	benchmarkNetwork(t)
	source, exponents, offset := openBenchmarkSource(t, *realStateDiffDBDir)
	target, err := parseBenchmarkExponents(*realStateDiffRebuildExponents)
	require.NoError(t, err)
	rows, dst := rebuildProfile(t, source, exponents, offset, target, *realStateDiffRebuildToSlot, *realStateDiffRebuildDst, *realStateDiffRebuildCompareDB, *realStateDiffRebuildCSV, *realStateDiffRebuildVerifyRate)
	compareCounts := map[string]int{}
	for _, row := range rows {
		compareCounts[row.compare]++
	}
	for level := range target {
		var count int
		var bytes uint64
		var elapsed time.Duration
		for _, row := range rows {
			if row.level == level {
				count++
				bytes += row.total
				elapsed += row.save
			}
		}
		t.Logf("level=%d entries=%d summed-entry-bytes=%d save=%s", level, count, bytes, elapsed)
	}
	info, err := os.Stat(StoreDatafilePath(*realStateDiffRebuildDst))
	require.NoError(t, err)
	t.Logf("destination file bytes=%d (includes allocation/freelist) entries=%d read-only source=%v destination=%v compare=%v (missing means absent from native DB)",
		info.Size(), len(rows), source.db.IsReadOnly(), dst.db.IsReadOnly(), compareCounts)
	if *realStateDiffRebuildCompareDB != "" && compareCounts["different"]+compareCounts["missing"] > 0 {
		t.Errorf("native comparison: different=%d missing=%d", compareCounts["different"], compareCounts["missing"])
	}
}

func benchmarkWorkloadStates(t *testing.T, last uint64) []state.BeaconState {
	t.Helper()
	initial, _ := util.DeterministicGenesisStateAltair(t, 8)
	states := []state.BeaconState{initial}
	for slot := uint64(32); slot <= last; slot += 32 {
		next := states[len(states)-1].Copy()
		require.NoError(t, next.SetSlot(primitives.Slot(slot)))
		balances := next.Balances()
		balances[slot/32%uint64(len(balances))]--
		require.NoError(t, next.SetBalances(balances))
		states = append(states, next)
	}
	return states
}

func benchmarkWorkloadDB(t *testing.T, exponents []int, states []state.BeaconState) string {
	t.Helper()
	previous := flags.Get()
	setBenchmarkExponents(exponents)
	defer flags.Init(previous)
	reset := features.InitWithReset(&features.Flags{EnableStateDiff: true})
	defer reset()
	directory := t.TempDir()
	store, err := NewKVStore(t.Context(), directory)
	require.NoError(t, err)
	require.NoError(t, store.initializeStateDiff(0, states[0]))
	for _, st := range states[1:] {
		require.NoError(t, store.saveStateByDiff(t.Context(), st))
	}
	require.NoError(t, store.Close())
	return directory
}

func TestStateDiffReadSaveBenchmarkWorkload(t *testing.T) {
	states := benchmarkWorkloadStates(t, 160)
	directory := benchmarkWorkloadDB(t, []int{7, 6, 5}, states)
	source, exponents, offset := openBenchmarkSource(t, directory)
	require.Equal(t, true, source.db.IsReadOnly())
	require.DeepEqual(t, []uint64{128, 64, 160}, benchmarkSlots(t, source, exponents, ""))
	diffsByMode := map[string]int{}
	for _, mode := range []string{"cold", "anchor", "memo"} {
		t.Run("read/cache="+mode, func(t *testing.T) {
			diffs, bytes, err := benchmarkRead(source, 96, mode, true)
			require.NoError(t, err)
			require.NotEqual(t, 0, diffs)
			require.NotEqual(t, 0, bytes)
			diffsByMode[mode] = diffs
			encoded, err := benchmarkStateSink.MarshalSSZ()
			require.NoError(t, err)
			expected, err := states[3].MarshalSSZ()
			require.NoError(t, err)
			require.DeepSSZEqual(t, expected, encoded)
		})
	}
	require.Equal(t, true, diffsByMode["anchor"] < diffsByMode["cold"])
	require.Equal(t, true, diffsByMode["memo"] < diffsByMode["cold"])
	t.Run("save", func(t *testing.T) {
		dst, target, level := benchmarkSaveSetup(t, source, offset, 96, "")
		clearBenchmarkMemo(dst.stateDiffCache)
		require.NoError(t, dst.saveStateByDiff(t.Context(), target))
		require.NoError(t, dst.saveStateByDiff(t.Context(), target))
		sizes, err := treeEntrySizes(dst, level, 96)
		require.NoError(t, err)
		require.NotEqual(t, 0, sizes[1]+sizes[2]+sizes[3])
	})
	t.Run("hot", func(t *testing.T) {
		initial := loadBenchmarkState(t, t.Context(), source, offset, 0)
		dst := benchmarkScratch(t, "", offset, initial)
		require.NoError(t, benchmarkHotSave(dst, t.Context(), states[3], 0))
		require.NoError(t, benchmarkHotSave(dst, t.Context(), states[3], 1))
		require.Equal(t, true, dst.HasHotStateSnapshot(t.Context(), [32]byte{}))
		require.NoError(t, benchmarkHotClear(dst, t.Context(), states[3], 3))
		require.Equal(t, false, dst.HasHotStateSnapshot(t.Context(), [32]byte{}))
	})
}

func TestStateDiffTreeAuditWorkload(t *testing.T) {
	for _, last := range []uint64{0, 128, 192, 320} {
		t.Run(fmt.Sprintf("frontier=%d", last), func(t *testing.T) {
			states := benchmarkWorkloadStates(t, last)
			directory := benchmarkWorkloadDB(t, []int{7, 6, 5}, states)
			store, exponents, offset := openBenchmarkSource(t, directory)
			frontier, err := latestBenchmarkBoundary(store, offset, exponents)
			require.NoError(t, err)
			require.Equal(t, last, frontier)
			a, err := auditStateDiffTree(store, offset, exponents)
			require.NoError(t, err)
			require.Equal(t, false, auditInvalid(a))
		})
	}
	t.Run("pending archive holes", func(t *testing.T) {
		directory := benchmarkWorkloadDB(t, []int{7, 6, 5}, benchmarkWorkloadStates(t, 128))
		previous := flags.Get()
		setBenchmarkExponents([]int{7, 6, 5})
		t.Cleanup(func() { flags.Init(previous) })
		db, err := bolt.Open(StoreDatafilePath(directory), 0o600, &bolt.Options{Timeout: time.Second})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, db.Close()) })
		require.NoError(t, db.Update(func(tx *bolt.Tx) error {
			bucket := tx.Bucket(stateDiffBucket)
			for _, suffix := range []string{stateSuffix, validatorSuffix, balancesSuffix} {
				if err := bucket.Delete(append(makeKeyForStateDiffTree(2, 96), suffix...)); err != nil {
					return err
				}
			}
			return bucket.Put(archiveStatusKey, (&ArchiveStatus{RegeneratedThroughSlot: 64}).encode())
		}))
		a, err := auditStateDiffTree(&Store{db: db}, 0, []int{7, 6, 5})
		require.NoError(t, err)
		require.Equal(t, false, auditInvalid(a))
		require.DeepEqual(t, []uint64{96}, a.levels[2].pendingHoles)
	})
	states := benchmarkWorkloadStates(t, 160)
	directory := benchmarkWorkloadDB(t, []int{7, 6, 5}, states)
	previous := flags.Get()
	setBenchmarkExponents([]int{7, 6, 5})
	t.Cleanup(func() { flags.Init(previous) })
	writable, err := bolt.Open(StoreDatafilePath(directory), 0o600, &bolt.Options{Timeout: time.Second})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, writable.Close()) })
	source := &Store{db: writable}
	exponents, err := source.loadStateDiffExponents()
	require.NoError(t, err)
	offset, err := source.loadOffset()
	require.NoError(t, err)
	a, err := auditStateDiffTree(source, offset, exponents)
	require.NoError(t, err)
	require.Equal(t, false, auditInvalid(a))
	csvPath := filepath.Join(t.TempDir(), "audit.csv")
	logTreeAudit(t, a, csvPath)
	_, err = os.Stat(csvPath)
	require.NoError(t, err)
	require.NoError(t, writable.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		key := makeKeyForStateDiffTree(2, 96)
		for _, suffix := range []string{stateSuffix, validatorSuffix, balancesSuffix} {
			if err := bucket.Delete(append(slices.Clone(key), suffix...)); err != nil {
				return err
			}
		}
		return nil
	}))
	a, err = auditStateDiffTree(source, offset, exponents)
	require.NoError(t, err)
	require.Equal(t, true, auditInvalid(a))
	require.DeepEqual(t, []uint64{96}, a.levels[2].holes)
	require.NoError(t, writable.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		return bucket.Put(append(makeKeyForStateDiffTree(2, 96), stateSuffix...), []byte{1})
	}))
	a, err = auditStateDiffTree(source, offset, exponents)
	require.NoError(t, err)
	require.DeepEqual(t, []uint64{96}, a.levels[2].incomplete)
	require.NoError(t, writable.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		return bucket.Put(append(makeKeyForStateDiffTree(2, 80), stateSuffix...), []byte{1})
	}))
	a, err = auditStateDiffTree(source, offset, exponents)
	require.NoError(t, err)
	require.DeepEqual(t, []uint64{80}, a.levels[2].orphans)
	require.NoError(t, writable.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateDiffBucket)
		if err := bucket.Delete(append(makeKeyForStateDiffTree(2, 96), stateSuffix...)); err != nil {
			return err
		}
		if err := bucket.Put(append(makeKeyForStateDiffTree(2, 96), "_unknown"...), []byte{1}); err != nil {
			return err
		}
		if err := bucket.Put(makeKeyForStateDiffTree(3, 96), []byte{1}); err != nil {
			return err
		}
		badPadding := makeKeyForStateDiffTree(2, 96)
		badPadding[stateDiffTreeKeySlotEnd] = 1
		return bucket.Put(badPadding, []byte{1})
	}))
	a, err = auditStateDiffTree(source, offset, exponents)
	require.NoError(t, err)
	require.DeepEqual(t, []uint64{96}, a.levels[2].holes)
	require.DeepEqual(t, []uint64{80, 96}, a.levels[2].orphans)
	require.Equal(t, 2, len(a.corruptKeys))
	_, err = auditStateDiffTree(source, offset, []int{7, 5})
	require.Equal(t, true, err != nil)
}

func TestStateDiffRebuildProfileWorkload(t *testing.T) {
	t.Run("frontier=160", func(t *testing.T) {
		states := benchmarkWorkloadStates(t, 160)
		sourceDir := benchmarkWorkloadDB(t, []int{7, 6, 5}, states)
		compareDir := benchmarkWorkloadDB(t, []int{7, 5}, states)
		source, exponents, offset := openBenchmarkSource(t, sourceDir)
		dstDir := filepath.Join(t.TempDir(), "rebuild")
		rows, dst := rebuildProfile(t, source, exponents, offset, []int{7, 5}, 0, dstDir, compareDir, filepath.Join(t.TempDir(), "rebuild.csv"), 1)
		require.Equal(t, true, source.db.IsReadOnly())
		require.Equal(t, false, dst.db.IsReadOnly())
		require.Equal(t, 6, len(rows))
		for _, row := range rows {
			require.Equal(t, true, row.verified)
			require.Equal(t, "equal", row.compare)
			require.NotEqual(t, uint64(0), row.total)
		}
	})
	for _, last := range []uint64{0, 128, 192, 320} {
		t.Run(fmt.Sprintf("frontier=%d", last), func(t *testing.T) {
			subset := benchmarkWorkloadStates(t, last)
			srcDir := benchmarkWorkloadDB(t, []int{7, 6, 5}, subset)
			nativeDir := benchmarkWorkloadDB(t, []int{7, 5}, subset)
			src, profile, off := openBenchmarkSource(t, srcDir)
			entries, _ := rebuildProfile(t, src, profile, off, []int{7, 5}, last, filepath.Join(t.TempDir(), "rebuilt"), nativeDir, "", 1)
			require.Equal(t, int(last/32+1), len(entries))
			for _, entry := range entries {
				require.Equal(t, true, entry.verified)
				require.Equal(t, "equal", entry.compare)
			}
		})
	}
}

func TestStateDiffRebuildVerificationWorkload(t *testing.T) {
	states := benchmarkWorkloadStates(t, 160)
	sourceDir := benchmarkWorkloadDB(t, []int{7, 6, 5}, states)
	source, profile, offset := openBenchmarkSource(t, sourceDir)
	_, dst := rebuildProfile(t, source, profile, offset, []int{8, 6, 5}, 0, filepath.Join(t.TempDir(), "rebuilt"), "", "", 1)
	for _, test := range []struct {
		name, suffix string
		level        int
		slot         uint64
		state        state.BeaconState
	}{
		{name: "snapshot", level: 0, slot: 0, state: states[0]},
		{name: "diff", level: 1, slot: 128, suffix: stateSuffix, state: states[4]},
	} {
		t.Run(test.name, func(t *testing.T) {
			key := append(makeKeyForStateDiffTree(test.level, test.slot), test.suffix...)
			var original []byte
			require.NoError(t, dst.db.Update(func(tx *bolt.Tx) error {
				bucket := tx.Bucket(stateDiffBucket)
				original = slices.Clone(bucket.Get(key))
				return bucket.Put(key, []byte{0xff})
			}))
			t.Cleanup(func() {
				require.NoError(t, dst.db.Update(func(tx *bolt.Tx) error {
					return tx.Bucket(stateDiffBucket).Put(key, original)
				}))
			})
			// The writer's cached anchor still reads cleanly despite the corrupt persisted entry.
			_, err := dst.stateByDiff(t.Context(), primitives.Slot(test.slot))
			require.NoError(t, err)
			err = verifyRebuildState(t.Context(), dst, test.state)
			require.Equal(t, true, err != nil)
		})
	}
}
