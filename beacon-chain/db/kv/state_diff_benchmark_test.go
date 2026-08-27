package kv

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/consensus-types/hdiff"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
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

// BenchmarkHdiffDiffRealState measures hdiff creation for the same anchor and target pair used
// when saving a finalized state. The database is opened directly in read-only mode because
// NewKVStore initializes buckets and metadata with write transactions.
func BenchmarkHdiffDiffRealState(b *testing.B) {
	if *realStateDiffDBDir == "" {
		b.Skip("state-diff database not provided; set -state-diff-benchmark-db")
	}

	db, err := bolt.Open(StoreDatafilePath(*realStateDiffDBDir), 0o400, &bolt.Options{
		ReadOnly: true,
		Timeout:  5 * time.Second,
	})
	if err != nil {
		b.Fatal(err)
	}
	if !db.IsReadOnly() {
		b.Fatal("benchmark database was not opened read-only")
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Error(err)
		}
	})

	store := &Store{db: db}
	exponents, err := store.loadStateDiffExponents()
	if err != nil {
		b.Fatal(err)
	}
	originalFlags := flags.Get()
	flags.Init(&flags.GlobalFlags{StateDiffExponents: exponents})
	b.Cleanup(func() { flags.Init(originalFlags) })

	offset, err := store.loadOffset()
	if err != nil {
		b.Fatal(err)
	}
	targetSlot := *realStateDiffTargetSlot
	if targetSlot == 0 {
		targetSlot, err = latestSlotForLevel(store, len(exponents)-1)
		if err != nil {
			b.Fatal(err)
		}
	}

	targetLevel := computeLevel(offset, primitives.Slot(targetSlot))
	if targetLevel <= 0 {
		b.Fatalf("target slot %d is not stored as a diff", targetSlot)
	}
	anchorSpan := uint64(1) << uint(exponents[targetLevel-1])
	anchorSlot := targetSlot - (targetSlot-offset)%anchorSpan
	anchorLevel := computeLevel(offset, primitives.Slot(anchorSlot))
	if anchorLevel < 0 || anchorLevel >= targetLevel {
		b.Fatalf("invalid anchor at slot %d level %d for target level %d", anchorSlot, anchorLevel, targetLevel)
	}

	ctx := b.Context()
	anchor := loadBenchmarkState(b, ctx, store, offset, primitives.Slot(anchorSlot))
	target := loadBenchmarkState(b, ctx, store, offset, primitives.Slot(targetSlot))
	b.Logf(
		"read-only database; exponents=%v target_slot=%d target_level=%d anchor_slot=%d anchor_level=%d validators=%d balances=%d",
		exponents,
		targetSlot,
		targetLevel,
		anchorSlot,
		anchorLevel,
		target.NumValidators(),
		target.BalancesLength(),
	)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		benchmarkHdiffSink, err = hdiff.Diff(anchor, target)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	b.ReportMetric(float64(target.NumValidators()), "validators")
	b.ReportMetric(float64(targetSlot-anchorSlot), "anchor-slots")
	b.ReportMetric(float64(len(benchmarkHdiffSink.StateDiff)), "state-diff-bytes/op")
	b.ReportMetric(float64(len(benchmarkHdiffSink.ValidatorDiffs)), "validator-diff-bytes/op")
	b.ReportMetric(float64(len(benchmarkHdiffSink.BalancesDiff)), "balance-diff-bytes/op")
}

func loadBenchmarkState(
	b *testing.B,
	ctx context.Context,
	store *Store,
	offset uint64,
	slot primitives.Slot,
) state.BeaconState {
	b.Helper()
	snapshot, diffs, err := store.getBaseAndDiffChain(offset, slot)
	if err != nil {
		b.Fatal(err)
	}
	for _, diff := range diffs {
		snapshot, err = hdiff.ApplyDiff(ctx, snapshot, diff)
		if err != nil {
			b.Fatal(err)
		}
	}
	return snapshot
}
