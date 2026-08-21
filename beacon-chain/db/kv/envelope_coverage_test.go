package kv

import (
	"context"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/db/iface"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/proto/dbval"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	bolt "go.etcd.io/bbolt"
)

// coverageTestEnvelope builds a saveable envelope with the given identity.
func coverageTestEnvelope(slot primitives.Slot, root, blockHash [32]byte) *ethpb.SignedExecutionPayloadEnvelope {
	return &ethpb.SignedExecutionPayloadEnvelope{
		Message: &ethpb.ExecutionPayloadEnvelope{
			Payload: &enginev1.ExecutionPayloadGloas{
				ParentHash:    make([]byte, 32),
				FeeRecipient:  make([]byte, 20),
				StateRoot:     make([]byte, 32),
				ReceiptsRoot:  make([]byte, 32),
				LogsBloom:     make([]byte, 256),
				PrevRandao:    make([]byte, 32),
				BaseFeePerGas: make([]byte, 32),
				BlockHash:     blockHash[:],
				SlotNumber:    slot,
			},
			ExecutionRequests:     &enginev1.ExecutionRequestsGloas{},
			BeaconBlockRoot:       root[:],
			ParentBeaconBlockRoot: make([]byte, 32),
		},
		Signature: make([]byte, 96),
	}
}

// saveCoverageEnvelope persists an envelope and returns its fingerprinted
// index entry.
func saveCoverageEnvelope(t *testing.T, db *Store, slot primitives.Slot, seed byte) iface.RevealedEnvelopeIndexEntry {
	t.Helper()
	root := bytesutil.ToBytes32([]byte{seed, 1})
	hash := bytesutil.ToBytes32([]byte{seed, 2})
	require.NoError(t, db.SaveExecutionPayloadEnvelope(context.Background(), coverageTestEnvelope(slot, root, hash)))
	_, fp, err := db.ExecutionPayloadEnvelopeWithFingerprint(context.Background(), root)
	require.NoError(t, err)
	return iface.RevealedEnvelopeIndexEntry{Slot: slot, Root: root, PrimaryFingerprint: fp}
}

func testCoverage(low, high primitives.Slot, anchor [32]byte) *dbval.EnvelopeCoverage {
	return &dbval.EnvelopeCoverage{
		FormatVersion:  1,
		LowSlot:        uint64(low),
		HighSlot:       uint64(high),
		HighAnchorRoot: anchor[:],
	}
}

func TestEnvelopeCoverageRoundtrip(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	_, err := db.EnvelopeCoverage(ctx)
	require.ErrorIs(t, err, ErrNotFound)

	anchor := bytesutil.ToBytes32([]byte("anchor"))
	require.NoError(t, db.SaveEnvelopeCoverage(ctx, testCoverage(3, 9, anchor)))
	cov, err := db.EnvelopeCoverage(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), cov.LowSlot)
	assert.Equal(t, uint64(9), cov.HighSlot)
	assert.DeepEqual(t, anchor[:], cov.HighAnchorRoot)

	// The dedicated record survives block-status saves: an old binary
	// rewriting BackfillStatus cannot strip coverage.
	require.NoError(t, db.SaveBackfillStatus(ctx, &dbval.BackfillStatus{LowSlot: 1}))
	cov, err = db.EnvelopeCoverage(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(9), cov.HighSlot)
}

func TestSaveEnvelopeCoverageValidation(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	anchor := bytesutil.ToBytes32([]byte("anchor"))

	bad := testCoverage(3, 9, anchor)
	bad.FormatVersion = 2
	require.ErrorContains(t, "format_version", db.SaveEnvelopeCoverage(ctx, bad))

	bad = testCoverage(9, 3, anchor)
	require.ErrorContains(t, "invalid EnvelopeCoverage interval", db.SaveEnvelopeCoverage(ctx, bad))

	bad = testCoverage(3, 9, anchor)
	bad.HighAnchorRoot = []byte{1, 2, 3}
	require.ErrorContains(t, "high_anchor_root", db.SaveEnvelopeCoverage(ctx, bad))

	require.ErrorContains(t, "nil", db.SaveEnvelopeCoverage(ctx, nil))
}

func TestCommitEnvelopeCoverageRangeReplacement(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	anchor := bytesutil.ToBytes32([]byte("anchor"))

	e4 := saveCoverageEnvelope(t, db, 4, 4)
	e5 := saveCoverageEnvelope(t, db, 5, 5)
	e7 := saveCoverageEnvelope(t, db, 7, 7)
	require.NoError(t, db.CommitEnvelopeCoverage(ctx, testCoverage(4, 8, anchor), []iface.EnvelopeIndexReplacement{
		{Start: 4, End: 8, Entries: []iface.RevealedEnvelopeIndexEntry{e4, e5, e7}},
	}))

	roots, err := db.RevealedEnvelopeRoots(ctx, 0, 100, 100)
	require.NoError(t, err)
	require.Equal(t, 3, len(roots))

	// Replace [5, 8): slot 4 must survive, 5 and 7 must be cleared, and the
	// replacement writes exactly one new entry at slot 6.
	e6 := saveCoverageEnvelope(t, db, 6, 6)
	require.NoError(t, db.CommitEnvelopeCoverage(ctx, testCoverage(4, 8, anchor), []iface.EnvelopeIndexReplacement{
		{Start: 5, End: 8, Entries: []iface.RevealedEnvelopeIndexEntry{e6}},
	}))
	roots, err = db.RevealedEnvelopeRoots(ctx, 0, 100, 100)
	require.NoError(t, err)
	require.Equal(t, 2, len(roots))
	assert.Equal(t, primitives.Slot(4), roots[0].Slot)
	assert.Equal(t, e4.Root, roots[0].Root)
	assert.Equal(t, primitives.Slot(6), roots[1].Slot)
	assert.Equal(t, e6.Root, roots[1].Root)

	// Entries outside their replacement range are rejected up front.
	err = db.CommitEnvelopeCoverage(ctx, testCoverage(4, 8, anchor), []iface.EnvelopeIndexReplacement{
		{Start: 5, End: 6, Entries: []iface.RevealedEnvelopeIndexEntry{e7}},
	})
	require.ErrorContains(t, "outside replacement range", err)
}

func TestCommitEnvelopeCoveragePrimaryRecheck(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	anchor := bytesutil.ToBytes32([]byte("anchor"))

	e4 := saveCoverageEnvelope(t, db, 4, 4)
	require.NoError(t, db.CommitEnvelopeCoverage(ctx, testCoverage(4, 5, anchor), []iface.EnvelopeIndexReplacement{
		{Start: 4, End: 5, Entries: []iface.RevealedEnvelopeIndexEntry{e4}},
	}))

	t.Run("missing primary aborts publication atomically", func(t *testing.T) {
		e6 := iface.RevealedEnvelopeIndexEntry{Slot: 6, Root: bytesutil.ToBytes32([]byte("nope"))}
		err := db.CommitEnvelopeCoverage(ctx, testCoverage(4, 7, anchor), []iface.EnvelopeIndexReplacement{
			{Start: 4, End: 7, Entries: []iface.RevealedEnvelopeIndexEntry{e4, e6}},
		})
		require.ErrorIs(t, err, iface.ErrEnvelopeCoveragePrimaryMismatch)
		// Nothing committed: coverage record and index unchanged, including
		// the range delete that preceded the failing put.
		cov, err := db.EnvelopeCoverage(ctx)
		require.NoError(t, err)
		assert.Equal(t, uint64(5), cov.HighSlot)
		roots, err := db.RevealedEnvelopeRoots(ctx, 0, 100, 100)
		require.NoError(t, err)
		require.Equal(t, 1, len(roots))
		assert.Equal(t, e4.Root, roots[0].Root)
	})

	t.Run("replaced primary value aborts publication", func(t *testing.T) {
		// Re-save a different envelope under the same root, invalidating the
		// fingerprint prepared before the transaction.
		replaced := coverageTestEnvelope(4, e4.Root, bytesutil.ToBytes32([]byte("other-hash")))
		require.NoError(t, db.SaveExecutionPayloadEnvelope(ctx, replaced))
		err := db.CommitEnvelopeCoverage(ctx, testCoverage(4, 7, anchor), []iface.EnvelopeIndexReplacement{
			{Start: 4, End: 5, Entries: []iface.RevealedEnvelopeIndexEntry{e4}},
		})
		require.ErrorIs(t, err, iface.ErrEnvelopeCoveragePrimaryMismatch)
	})
}

func TestRevealedEnvelopeRoots(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	anchor := bytesutil.ToBytes32([]byte("anchor"))

	var entries []iface.RevealedEnvelopeIndexEntry
	for _, slot := range []primitives.Slot{2, 4, 6, 8} {
		entries = append(entries, saveCoverageEnvelope(t, db, slot, byte(slot)))
	}
	require.NoError(t, db.CommitEnvelopeCoverage(ctx, testCoverage(2, 9, anchor), []iface.EnvelopeIndexReplacement{
		{Start: 2, End: 9, Entries: entries},
	}))

	// Half-open bounds.
	roots, err := db.RevealedEnvelopeRoots(ctx, 4, 8, 100)
	require.NoError(t, err)
	require.Equal(t, 2, len(roots))
	assert.Equal(t, primitives.Slot(4), roots[0].Slot)
	assert.Equal(t, primitives.Slot(6), roots[1].Slot)

	// Limit caps returned entries.
	roots, err = db.RevealedEnvelopeRoots(ctx, 0, 100, 3)
	require.NoError(t, err)
	require.Equal(t, 3, len(roots))

	// Empty window.
	roots, err = db.RevealedEnvelopeRoots(ctx, 8, 8, 100)
	require.NoError(t, err)
	require.Equal(t, 0, len(roots))
}

func TestPruneRevealedEnvelopeIndexBelow(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	anchor := bytesutil.ToBytes32([]byte("anchor"))

	var entries []iface.RevealedEnvelopeIndexEntry
	for _, slot := range []primitives.Slot{1, 2, 3, 4, 5} {
		entries = append(entries, saveCoverageEnvelope(t, db, slot, byte(slot)))
	}
	require.NoError(t, db.CommitEnvelopeCoverage(ctx, testCoverage(1, 6, anchor), []iface.EnvelopeIndexReplacement{
		{Start: 1, End: 6, Entries: entries},
	}))

	// Bounded: only two keys removed per call.
	n, err := db.PruneRevealedEnvelopeIndexBelow(ctx, 4, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	n, err = db.PruneRevealedEnvelopeIndexBelow(ctx, 4, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	roots, err := db.RevealedEnvelopeRoots(ctx, 0, 100, 100)
	require.NoError(t, err)
	require.Equal(t, 2, len(roots))
	assert.Equal(t, primitives.Slot(4), roots[0].Slot)
}

func TestDeleteExecutionPayloadEnvelopeCoverageGuard(t *testing.T) {
	ctx := context.Background()
	anchor := bytesutil.ToBytes32([]byte("anchor"))

	t.Run("covered root refuses without deleting either entry", func(t *testing.T) {
		db := setupDB(t)
		e4 := saveCoverageEnvelope(t, db, 4, 4)
		require.NoError(t, db.CommitEnvelopeCoverage(ctx, testCoverage(4, 5, anchor), []iface.EnvelopeIndexReplacement{
			{Start: 4, End: 5, Entries: []iface.RevealedEnvelopeIndexEntry{e4}},
		}))
		err := db.DeleteExecutionPayloadEnvelope(ctx, e4.Root)
		require.ErrorIs(t, err, ErrEnvelopeCovered)
		// Primary and block-hash index survive.
		env, err := db.ExecutionPayloadEnvelope(ctx, e4.Root)
		require.NoError(t, err)
		byHash, err := db.ExecutionPayloadEnvelopeByBlockHash(ctx, bytesutil.ToBytes32(env.Message.BlockHash))
		require.NoError(t, err)
		assert.DeepEqual(t, e4.Root[:], byHash.Message.BeaconBlockRoot)
	})

	t.Run("unindexed root deletes primary and block-hash entries", func(t *testing.T) {
		db := setupDB(t)
		e4 := saveCoverageEnvelope(t, db, 4, 4)
		env, err := db.ExecutionPayloadEnvelope(ctx, e4.Root)
		require.NoError(t, err)
		blockHash := bytesutil.ToBytes32(env.Message.BlockHash)
		require.NoError(t, db.DeleteExecutionPayloadEnvelope(ctx, e4.Root))
		_, err = db.ExecutionPayloadEnvelope(ctx, e4.Root)
		require.ErrorIs(t, err, ErrNotFound)
		_, err = db.ExecutionPayloadEnvelopeByBlockHash(ctx, blockHash)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("same slot different root still deletes", func(t *testing.T) {
		db := setupDB(t)
		e4 := saveCoverageEnvelope(t, db, 4, 4)
		require.NoError(t, db.CommitEnvelopeCoverage(ctx, testCoverage(4, 5, anchor), []iface.EnvelopeIndexReplacement{
			{Start: 4, End: 5, Entries: []iface.RevealedEnvelopeIndexEntry{e4}},
		}))
		// A stored-but-withheld orphan envelope at the same slot is not the
		// indexed root and may be deleted.
		other := saveCoverageEnvelope(t, db, 4, 44)
		require.NoError(t, db.DeleteExecutionPayloadEnvelope(ctx, other.Root))
	})

	t.Run("malformed stored value refuses deletion", func(t *testing.T) {
		db := setupDB(t)
		root := bytesutil.ToBytes32([]byte("garbage-root"))
		require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
			return tx.Bucket(executionPayloadEnvelopesBucket).Put(root[:], []byte("not-snappy-ssz"))
		}))
		err := db.DeleteExecutionPayloadEnvelope(ctx, root)
		require.NotNil(t, err)
		// The malformed value is retained for inspection.
		var still []byte
		require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
			still = tx.Bucket(executionPayloadEnvelopesBucket).Get(root[:])
			return nil
		}))
		require.NotNil(t, still)
	})

	t.Run("delete before publication makes the combined commit refuse", func(t *testing.T) {
		db := setupDB(t)
		e4 := saveCoverageEnvelope(t, db, 4, 4)
		require.NoError(t, db.DeleteExecutionPayloadEnvelope(ctx, e4.Root))
		err := db.CommitEnvelopeCoverage(ctx, testCoverage(4, 5, anchor), []iface.EnvelopeIndexReplacement{
			{Start: 4, End: 5, Entries: []iface.RevealedEnvelopeIndexEntry{e4}},
		})
		require.ErrorIs(t, err, iface.ErrEnvelopeCoveragePrimaryMismatch)
	})
}

// putSlotIndex writes a packed root list for a slot directly into the block
// slot index bucket.
func putSlotIndex(t *testing.T, db *Store, slot primitives.Slot, roots ...[32]byte) {
	t.Helper()
	packed := make([]byte, 0, len(roots)*32)
	for _, r := range roots {
		packed = append(packed, r[:]...)
	}
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(blockSlotIndicesBucket).Put(bytesutil.SlotToBytesBigEndian(slot), packed)
	}))
}

func idxRoot(seed byte) [32]byte {
	return bytesutil.ToBytes32([]byte{seed, 0xaa})
}

func TestBlockSlotIndexPageDescending(t *testing.T) {
	ctx := context.Background()

	t.Run("multi-root slots respect the candidate budget with a boundary cursor", func(t *testing.T) {
		db := setupDB(t)
		putSlotIndex(t, db, 5, idxRoot(1), idxRoot(2))
		putSlotIndex(t, db, 7, idxRoot(3))
		putSlotIndex(t, db, 9, idxRoot(4))

		page, err := db.BlockSlotIndexPageDescending(ctx, iface.SlotIndexCursor{Slot: 9}, 5, 2, 1<<20)
		require.NoError(t, err)
		require.Equal(t, 2, len(page.Candidates))
		assert.Equal(t, primitives.Slot(9), page.Candidates[0].Slot)
		assert.Equal(t, primitives.Slot(7), page.Candidates[1].Slot)
		require.NotNil(t, page.Next)
		assert.Equal(t, primitives.Slot(5), page.Next.Slot)
		assert.Equal(t, uint64(0), page.Next.NextByteOffset)

		page, err = db.BlockSlotIndexPageDescending(ctx, *page.Next, 5, 100, 1<<20)
		require.NoError(t, err)
		require.Equal(t, 2, len(page.Candidates))
		assert.Equal(t, idxRoot(1), page.Candidates[0].Root)
		assert.Equal(t, idxRoot(2), page.Candidates[1].Root)
		require.IsNil(t, page.Next)
	})

	t.Run("oversized packed value is sliced with a validated mid-slot token", func(t *testing.T) {
		db := setupDB(t)
		putSlotIndex(t, db, 6, idxRoot(1), idxRoot(2), idxRoot(3), idxRoot(4))

		page, err := db.BlockSlotIndexPageDescending(ctx, iface.SlotIndexCursor{Slot: 8}, 1, 3, 1<<20)
		require.NoError(t, err)
		require.Equal(t, 3, len(page.Candidates))
		require.NotNil(t, page.Next)
		assert.Equal(t, primitives.Slot(6), page.Next.Slot)
		assert.Equal(t, uint64(96), page.Next.NextByteOffset)
		assert.Equal(t, idxRoot(3), page.Next.LastRoot)

		// O(1) resume continues inside the same value.
		page, err = db.BlockSlotIndexPageDescending(ctx, *page.Next, 1, 100, 1<<20)
		require.NoError(t, err)
		require.Equal(t, 1, len(page.Candidates))
		assert.Equal(t, idxRoot(4), page.Candidates[0].Root)
		require.IsNil(t, page.Next)
	})

	t.Run("byte budget is a hard bound", func(t *testing.T) {
		db := setupDB(t)
		putSlotIndex(t, db, 6, idxRoot(1), idxRoot(2), idxRoot(3))
		page, err := db.BlockSlotIndexPageDescending(ctx, iface.SlotIndexCursor{Slot: 8}, 1, 100, 64)
		require.NoError(t, err)
		require.Equal(t, 2, len(page.Candidates))
		require.NotNil(t, page.Next)
		assert.Equal(t, uint64(64), page.Next.NextByteOffset)
	})

	t.Run("deletion at or before the token offset invalidates the cursor", func(t *testing.T) {
		db := setupDB(t)
		putSlotIndex(t, db, 6, idxRoot(1), idxRoot(2), idxRoot(3))
		page, err := db.BlockSlotIndexPageDescending(ctx, iface.SlotIndexCursor{Slot: 8}, 1, 2, 1<<20)
		require.NoError(t, err)
		require.NotNil(t, page.Next)
		cur := *page.Next

		// Compact the value by removing the first root (before the offset).
		putSlotIndex(t, db, 6, idxRoot(2), idxRoot(3))
		_, err = db.BlockSlotIndexPageDescending(ctx, cur, 1, 100, 1<<20)
		require.ErrorIs(t, err, iface.ErrSlotIndexCursorInvalidated)

		// Restarting the current slot from its beginning re-examines it.
		page, err = db.BlockSlotIndexPageDescending(ctx, iface.SlotIndexCursor{Slot: cur.Slot}, 1, 100, 1<<20)
		require.NoError(t, err)
		require.Equal(t, 2, len(page.Candidates))
	})

	t.Run("deletion after the token offset resumes cleanly", func(t *testing.T) {
		db := setupDB(t)
		putSlotIndex(t, db, 6, idxRoot(1), idxRoot(2), idxRoot(3), idxRoot(4))
		page, err := db.BlockSlotIndexPageDescending(ctx, iface.SlotIndexCursor{Slot: 8}, 1, 2, 1<<20)
		require.NoError(t, err)
		require.NotNil(t, page.Next)
		cur := *page.Next
		require.Equal(t, uint64(64), cur.NextByteOffset)

		// Ordered append can only extend past the offset; a delete after the
		// offset compacts only the suffix.
		putSlotIndex(t, db, 6, idxRoot(1), idxRoot(2), idxRoot(4))
		page, err = db.BlockSlotIndexPageDescending(ctx, cur, 1, 100, 1<<20)
		require.NoError(t, err)
		require.Equal(t, 1, len(page.Candidates))
		assert.Equal(t, idxRoot(4), page.Candidates[0].Root)
	})

	t.Run("floor is inclusive and bounds the scan", func(t *testing.T) {
		db := setupDB(t)
		putSlotIndex(t, db, 3, idxRoot(1))
		putSlotIndex(t, db, 5, idxRoot(2))
		page, err := db.BlockSlotIndexPageDescending(ctx, iface.SlotIndexCursor{Slot: 10}, 5, 100, 1<<20)
		require.NoError(t, err)
		require.Equal(t, 1, len(page.Candidates))
		assert.Equal(t, primitives.Slot(5), page.Candidates[0].Slot)
		require.IsNil(t, page.Next)
	})

	t.Run("invalid budgets are rejected", func(t *testing.T) {
		db := setupDB(t)
		_, err := db.BlockSlotIndexPageDescending(ctx, iface.SlotIndexCursor{Slot: 10}, 5, 0, 100)
		require.ErrorContains(t, "invalid slot index page budget", err)
	})
}

func TestBlockSlotIndexPageAscending(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	putSlotIndex(t, db, 5, idxRoot(1))
	putSlotIndex(t, db, 7, idxRoot(2), idxRoot(3))
	putSlotIndex(t, db, 9, idxRoot(4))
	putSlotIndex(t, db, 11, idxRoot(5))

	// Ceil is inclusive; scan begins at the smallest indexed slot >= cursor.
	page, err := db.BlockSlotIndexPageAscending(ctx, iface.SlotIndexCursor{Slot: 6}, 9, 2, 1<<20)
	require.NoError(t, err)
	require.Equal(t, 2, len(page.Candidates))
	assert.Equal(t, primitives.Slot(7), page.Candidates[0].Slot)
	assert.Equal(t, idxRoot(2), page.Candidates[0].Root)
	assert.Equal(t, idxRoot(3), page.Candidates[1].Root)
	require.NotNil(t, page.Next)
	assert.Equal(t, primitives.Slot(9), page.Next.Slot)

	page, err = db.BlockSlotIndexPageAscending(ctx, *page.Next, 9, 100, 1<<20)
	require.NoError(t, err)
	require.Equal(t, 1, len(page.Candidates))
	assert.Equal(t, primitives.Slot(9), page.Candidates[0].Slot)
	require.IsNil(t, page.Next)

	// Mid-slot token resume in ascending order.
	page, err = db.BlockSlotIndexPageAscending(ctx, iface.SlotIndexCursor{Slot: 7}, 11, 1, 1<<20)
	require.NoError(t, err)
	require.Equal(t, 1, len(page.Candidates))
	require.NotNil(t, page.Next)
	assert.Equal(t, primitives.Slot(7), page.Next.Slot)
	assert.Equal(t, uint64(32), page.Next.NextByteOffset)
	page, err = db.BlockSlotIndexPageAscending(ctx, *page.Next, 11, 100, 1<<20)
	require.NoError(t, err)
	require.Equal(t, 3, len(page.Candidates))
	assert.Equal(t, idxRoot(3), page.Candidates[0].Root)
	assert.Equal(t, primitives.Slot(11), page.Candidates[2].Slot)
	require.IsNil(t, page.Next)
}

// BenchmarkRevealedEnvelopeRootsSparse measures the O(quota) index seek that
// bounds by-range serving cost on a mostly-withheld window: only every 64th
// slot has a revealed entry across a near-window-scale index.
func BenchmarkRevealedEnvelopeRootsSparse(b *testing.B) {
	db := setupDB(b)
	ctx := context.Background()

	const slotsTotal = 1 << 16
	packed := make(map[primitives.Slot][32]byte, slotsTotal/64)
	require.NoError(b, db.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(revealedEnvelopeSlotIndexBucket)
		for slot := primitives.Slot(1); slot < slotsTotal; slot += 64 {
			root := bytesutil.ToBytes32(bytesutil.SlotToBytesBigEndian(slot))
			packed[slot] = root
			if err := bkt.Put(bytesutil.SlotToBytesBigEndian(slot), root[:]); err != nil {
				return err
			}
		}
		return nil
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		roots, err := db.RevealedEnvelopeRoots(ctx, 1, slotsTotal, 128)
		if err != nil {
			b.Fatal(err)
		}
		if len(roots) != 128 {
			b.Fatalf("expected 128 roots, got %d", len(roots))
		}
	}
}

// BenchmarkBlockSlotIndexPageDescending measures one steady migration page
// over a densely populated slot index.
func BenchmarkBlockSlotIndexPageDescending(b *testing.B) {
	db := setupDB(b)
	ctx := context.Background()

	const slotsTotal = 1 << 15
	require.NoError(b, db.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(blockSlotIndicesBucket)
		for slot := primitives.Slot(1); slot < slotsTotal; slot++ {
			root := bytesutil.ToBytes32(bytesutil.SlotToBytesBigEndian(slot))
			if err := bkt.Put(bytesutil.SlotToBytesBigEndian(slot), root[:]); err != nil {
				return err
			}
		}
		return nil
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, err := db.BlockSlotIndexPageDescending(ctx, iface.SlotIndexCursor{Slot: slotsTotal}, 1, 2048, 2048*32)
		if err != nil {
			b.Fatal(err)
		}
		if len(page.Candidates) != 2048 {
			b.Fatalf("expected 2048 candidates, got %d", len(page.Candidates))
		}
	}
}
