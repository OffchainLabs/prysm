package fieldtrie

import (
	"encoding/binary"
	"runtime"
	"testing"

	customtypes "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/custom-types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/container/trie"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const (
	testBlockRootsSize = 8192
	testVotesSize      = 128
	testNumBalances    = 256
)

// recomputeStep describes a single mutate-then-recompute step for table-driven tests.
type recomputeStep struct {
	changedIndices []uint64
	mutate         func()
}

// newTestElements creates fresh test data for the three field trie data types.
func newTestElements() (customtypes.BlockRoots, []*ethpb.Eth1Data, []uint64) {
	blockRoots := make(customtypes.BlockRoots, testBlockRootsSize)
	for i := range blockRoots {
		binary.LittleEndian.PutUint64(blockRoots[i][:8], uint64(i))
	}

	votes := make([]*ethpb.Eth1Data, testVotesSize)
	for i := range votes {
		votes[i] = &ethpb.Eth1Data{
			DepositRoot:  make([]byte, fieldparams.RootLength),
			DepositCount: uint64(i),
			BlockHash:    make([]byte, fieldparams.RootLength),
		}
	}

	balances := make([]uint64, testNumBalances)
	for i := range balances {
		balances[i] = uint64(i) * 1000
	}

	return blockRoots, votes, balances
}

// newTestOverlay creates an overlay trie by forking a shared BlockRoots trie
// with a single mutated leaf at index 0.
func newTestOverlay(t *testing.T) (base *FieldTrie, overlay *FieldTrie, overlayRoot [32]byte, blockRoots customtypes.BlockRoots) {
	t.Helper()
	blockRoots, _, _ = newTestElements()
	base, err := NewFieldTrie(types.BlockRoots, types.BasicArray, blockRoots, testBlockRootsSize, 0)
	require.NoError(t, err)
	cp := base.CopyTrie()
	_ = cp
	binary.LittleEndian.PutUint64(blockRoots[0][:8], 999_999)
	overlay, overlayRoot, err = base.RecomputeTrie([]uint64{0}, blockRoots)
	require.NoError(t, err)
	return base, overlay, overlayRoot, blockRoots
}

// requireFreshTrieRoot builds a fresh trie from elements and asserts its root matches expectedRoot.
func requireFreshTrieRoot(t *testing.T, field types.FieldIndex, dataType types.DataType, elements any, length uint64, threshold int, expectedRoot [32]byte) {
	t.Helper()
	fresh, err := NewFieldTrie(field, dataType, elements, length, threshold)
	require.NoError(t, err)
	freshRoot, err := fresh.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, freshRoot, expectedRoot)
}

// TestFieldTrie_RefCount verifies the copy-on-write reference counting of FieldTrie.
func TestFieldTrie_RefCount(t *testing.T) {
	// Copy a trie (bumping the shared ref count to 2), drop the copy and trigger GC.
	// The cleanup callback must decrement the ref count back to 1 on the surviving original.
	t.Run("drop copy", func(t *testing.T) {
		ft, err := NewFieldTrie(types.FieldIndex(5), types.BasicArray, customtypes.BlockRoots{}, 8192, 0)
		require.NoError(t, err)
		require.Equal(t, uint(1), ft.RefCount())

		cp := ft.CopyTrie()
		require.Equal(t, uint(2), ft.RefCount())
		require.Equal(t, uint(2), cp.RefCount())

		cp = nil

		// First GC marks the copy as unreachable and schedules its cleanup callback.
		// Second GC ensures the cleanup callback has actually executed and decremented the ref count.
		runtime.GC()
		runtime.GC()

		require.Equal(t, uint(1), ft.RefCount())
	})

	// Mirror of "drop copy": drop the original and trigger GC. The cleanup must
	// decrement the ref count back to 1 on the surviving copy.
	t.Run("drop original", func(t *testing.T) {
		ft, err := NewFieldTrie(types.FieldIndex(5), types.BasicArray, customtypes.BlockRoots{}, 8192, 0)
		require.NoError(t, err)
		require.Equal(t, uint(1), ft.RefCount())

		cp := ft.CopyTrie()
		require.Equal(t, uint(2), ft.RefCount())
		require.Equal(t, uint(2), cp.RefCount())

		ft = nil

		// First GC marks the original as unreachable and schedules its cleanup callback.
		// Second GC ensures the cleanup callback has actually executed and decremented the ref count.
		runtime.GC()
		runtime.GC()

		require.Equal(t, uint(1), cp.RefCount())
	})

	// Fork a shared owned trie A into an overlay B (B.base = A), then copy B to B1.
	// Dropping B and triggering GC must not release A's dataRef.
	t.Run("copy of overlay keeps base dataRef alive", func(t *testing.T) {
		blockRoots, _, _ := newTestElements()

		a, err := NewFieldTrie(types.BlockRoots, types.BasicArray, blockRoots, testBlockRootsSize, 0)
		require.NoError(t, err)
		require.Equal(t, uint(1), a.ref.Refs())
		require.Equal(t, uint(0), a.dataRef.Refs())

		// Share A so the next Recompute forks into an overlay.
		// A and A1 share the same ref/dataRef pointers.
		a1 := a.CopyTrie()
		require.Equal(t, uint(2), a.ref.Refs())
		require.Equal(t, uint(2), a1.ref.Refs())
		require.Equal(t, uint(0), a.dataRef.Refs())
		require.Equal(t, uint(0), a1.dataRef.Refs())

		// Fork into overlay B. B gets fresh ref/dataRef counters and bumps A's dataRef.
		binary.LittleEndian.PutUint64(blockRoots[0][:8], 42)
		b, _, err := a.RecomputeTrie([]uint64{0}, blockRoots)
		require.NoError(t, err)
		require.Equal(t, a, b.base)
		require.Equal(t, uint(2), a.ref.Refs())
		require.Equal(t, uint(1), a.dataRef.Refs())
		require.Equal(t, uint(2), a1.ref.Refs())
		require.Equal(t, uint(1), a1.dataRef.Refs())
		require.Equal(t, uint(1), b.ref.Refs())
		require.Equal(t, uint(0), b.dataRef.Refs())

		// B1 now also depends on A as its immutable base.
		// B and B1 share the same ref/dataRef pointers. A's dataRef must be bumped
		// again so that B1 keeps A alive as an immutable base.
		b1 := b.CopyTrie()
		require.Equal(t, a, b1.base)
		require.Equal(t, uint(2), a.ref.Refs())
		require.Equal(t, uint(2), a.dataRef.Refs())
		require.Equal(t, uint(2), a1.ref.Refs())
		require.Equal(t, uint(2), a1.dataRef.Refs())
		require.Equal(t, uint(2), b.ref.Refs())
		require.Equal(t, uint(2), b1.ref.Refs())
		require.Equal(t, uint(0), b.dataRef.Refs())
		require.Equal(t, uint(0), b1.dataRef.Refs())

		// Drop B and run GC. B1 is still alive and still references A.
		b = nil
		runtime.GC()
		runtime.GC()

		require.Equal(t, uint(2), a.ref.Refs())
		require.Equal(t, uint(1), a.dataRef.Refs())
		require.Equal(t, uint(2), a1.ref.Refs())
		require.Equal(t, uint(1), a1.dataRef.Refs())
		require.Equal(t, uint(1), b1.ref.Refs())
		require.Equal(t, uint(0), b1.dataRef.Refs())

		runtime.KeepAlive(a1)
		runtime.KeepAlive(b1)
	})
}

// TestFieldTrie_CopyPreservesRoot verifies that copying a FieldTrie preserves the trie root.
// Both the original and the copy must return the same root from TrieRoot().
func TestFieldTrie_CopyPreservesRoot(t *testing.T) {
	blockRoots, _, _ := newTestElements()

	ft, err := NewFieldTrie(types.BlockRoots, types.BasicArray, blockRoots, testBlockRootsSize, 0)
	require.NoError(t, err)

	originalRoot, err := ft.TrieRoot()
	require.NoError(t, err)

	cp := ft.CopyTrie()

	copyRoot, err := cp.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, originalRoot, copyRoot)

	// Verify the original's root is still the same after the copy.
	rootAfterCopy, err := ft.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, originalRoot, rootAfterCopy)
}

// TestFieldTrie_RecomputeNoChange verifies that recomputing a non-shared trie with no changed
// indices returns the same root and does not alter the dataRef count.
func TestFieldTrie_RecomputeNoChange(t *testing.T) {
	blockRoots, _, _ := newTestElements()

	ft, err := NewFieldTrie(types.BlockRoots, types.BasicArray, blockRoots, testBlockRootsSize, 0)
	require.NoError(t, err)
	require.Equal(t, uint(0), ft.dataRef.Refs())

	originalRoot, err := ft.TrieRoot()
	require.NoError(t, err)

	returned, recomputedRoot, err := ft.RecomputeTrie([]uint64{}, blockRoots)
	require.NoError(t, err)
	require.Equal(t, originalRoot, recomputedRoot)

	returnedRoot, err := returned.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, originalRoot, returnedRoot)

	require.Equal(t, uint(0), ft.dataRef.Refs())
}

// TestFieldTrie_RecomputePromotion verifies the promotion path of RecomputeTrie
// for all three field trie data types: BasicArray, CompositeArray, and CompressedArray.
// For each data type it tests two scenarios:
//  1. Direct promotion: a shared owned trie is recomputed with more changed indices than
//     the promotion threshold, causing the fork to be immediately promoted to an owned
//     trie via rebuildFromScratch, releasing the base reference.
//  2. Overlay-copy promotion: the promoted trie from (1) is made shared and recomputed
//     with few indices to create an overlay. That overlay is then copied (exercising the
//     overlay-mode branch of fork that deep-copies overrides and shares the base) and
//     recomputed with more indices than the threshold, triggering promotion again.
//
// After every RecomputeTrie call, the resulting root is compared against a fresh trie
// built from the same elements via NewFieldTrie to ensure correctness.
func TestFieldTrie_RecomputePromotion(t *testing.T) {
	// Use a small promotion threshold so we can trigger promotion with few indices.
	const promotionThreshold = 3

	type testCase struct {
		name           string
		field          types.FieldIndex
		dataType       types.DataType
		elements       any
		length         uint64
		changedIndices []uint64
		mutate         func()
		mutate2        func()
	}

	blockRoots, votes, balances := newTestElements()

	tests := []testCase{
		{
			name:           "BasicArray",
			field:          types.BlockRoots,
			dataType:       types.BasicArray,
			elements:       blockRoots,
			length:         testBlockRootsSize,
			changedIndices: []uint64{0, 10, 20, 30},
			mutate: func() {
				for _, idx := range []uint64{0, 10, 20, 30} {
					binary.LittleEndian.PutUint64(blockRoots[idx][:8], uint64(idx)+999)
				}
			},
			mutate2: func() {
				for _, idx := range []uint64{0, 10} {
					binary.LittleEndian.PutUint64(blockRoots[idx][:8], uint64(idx)+5000)
				}
			},
		},
		{
			name:           "CompositeArray",
			field:          types.Eth1DataVotes,
			dataType:       types.CompositeArray,
			elements:       votes,
			length:         testVotesSize,
			changedIndices: []uint64{0, 5, 10, 15},
			mutate: func() {
				for _, idx := range []uint64{0, 5, 10, 15} {
					votes[idx].DepositCount = uint64(idx) + 999
				}
			},
			mutate2: func() {
				for _, idx := range []uint64{0, 5} {
					votes[idx].DepositCount = uint64(idx) + 5000
				}
			},
		},
		{
			name:           "CompressedArray",
			field:          types.Balances,
			dataType:       types.CompressedArray,
			elements:       balances,
			length:         testNumBalances / 4,
			changedIndices: []uint64{0, 7, 20, 50},
			mutate: func() {
				for _, idx := range []uint64{0, 7, 20, 50} {
					balances[idx] = uint64(idx) + 999_999
				}
			},
			mutate2: func() {
				for _, idx := range []uint64{0, 7} {
					balances[idx] = uint64(idx) + 5_000_000
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ft, err := NewFieldTrie(tc.field, tc.dataType, tc.elements, tc.length, 0)
			require.NoError(t, err)
			// Set an absolute threshold small enough so promotion triggers with few indices.
			ft.promotionThreshold = promotionThreshold

			originalRoot, err := ft.TrieRoot()
			require.NoError(t, err)
			require.Equal(t, uint(0), ft.dataRef.Refs())

			// Copy to make the trie shared (ref=2), which forces RecomputeTrie to fork.
			cp := ft.CopyTrie()
			_ = cp

			// Mutate more elements than the promotion threshold.
			tc.mutate()

			forked, forkedRoot, err := ft.RecomputeTrie(tc.changedIndices, tc.elements)
			require.NoError(t, err)

			t.Run("direct promotion", func(t *testing.T) {
				// The forked root returned by RecomputeTrie must match TrieRoot() on the returned trie.
				forkedTrieRoot, err := forked.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, forkedRoot, forkedTrieRoot)

				// The recomputed root must match a fresh trie built from the same elements.
				requireFreshTrieRoot(t, tc.field, tc.dataType, tc.elements, tc.length, promotionThreshold, forkedRoot)

				// The original's dataRef is 0: the fork was promoted to an owned trie,
				// so it released its base reference during rebuildFromScratch.
				require.Equal(t, uint(0), ft.dataRef.Refs())

				// The promoted trie's dataRef is 0: it is an independent owned trie.
				require.Equal(t, uint(0), forked.dataRef.Refs())

				// The original's root must be unchanged.
				rootAfterPromotion, err := ft.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, originalRoot, rootAfterPromotion)
			})

			t.Run("promotion from a copy of an overlay", func(t *testing.T) {
				// Make the promoted (owned) trie shared by copying it.
				forkedCp2 := forked.CopyTrie()
				_ = forkedCp2

				// Mutate elements to new values and recompute with a small subset (≤ threshold)
				// to create an overlay (not promoted).
				tc.mutate2()
				overlay, overlayRoot, err := forked.RecomputeTrie(tc.changedIndices[:2], tc.elements)
				require.NoError(t, err)
				overlayTrieRoot, err := overlay.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, overlayRoot, overlayTrieRoot)

				// The overlay root must match a fresh trie built from the same elements.
				requireFreshTrieRoot(t, tc.field, tc.dataType, tc.elements, tc.length, promotionThreshold, overlayRoot)

				// The overlay references forked as its base, so forked.dataRef is now 1.
				require.Equal(t, uint(1), forked.dataRef.Refs())
				// The overlay's own dataRef is 0: nobody references the overlay's data yet.
				require.Equal(t, uint(0), overlay.dataRef.Refs())

				// Copy the overlay to make it shared (ref=2), forcing the next RecomputeTrie to fork.
				overlayCp := overlay.CopyTrie()
				_ = overlayCp

				// Mutate elements again and recompute with > threshold indices.
				// Since the overlay is shared, RecomputeTrie forks it (overlay-mode fork path
				// that deep-copies overrides and shares the base), then promotes because
				// len(changedIndices) > promotionThreshold.
				tc.mutate()
				promoted, promotedRoot, err := overlay.RecomputeTrie(tc.changedIndices, tc.elements)
				require.NoError(t, err)

				// The promoted root returned by RecomputeTrie must match TrieRoot() on the returned trie.
				promotedTrieRoot, err := promoted.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, promotedRoot, promotedTrieRoot)

				// The promoted root must match a fresh trie built from the same elements.
				requireFreshTrieRoot(t, tc.field, tc.dataType, tc.elements, tc.length, promotionThreshold, promotedRoot)

				// The overlay's dataRef is 0: the fork used overlay.base (not overlay itself) as its base.
				require.Equal(t, uint(0), overlay.dataRef.Refs())

				// The promoted trie's dataRef is 0: it is an independent owned trie.
				require.Equal(t, uint(0), promoted.dataRef.Refs())

				// The overlay's root must be unchanged.
				overlayRootAfterPromotion, err := overlay.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, overlayRoot, overlayRootAfterPromotion)
			})
		})
	}
}

// TestFieldTrie_RecomputeAccumulatedPromotion verifies the promoteOverlay path of RecomputeTrie
// for all three field trie data types. Unlike TestFieldTrie_RecomputePromotion (which triggers
// rebuildFromScratch when len(indices) > threshold on the initial fork), this test accumulates
// leaf-level overrides across multiple recomputes until len(overridesData.levels[0]) > threshold
// triggers promoteOverlay, which rebuilds the overlay into an owned trie.
// It also verifies the overlay-copy accumulated promotion path: after the initial promotion,
// the promoted trie is made shared and a new overlay is created, copied (exercising the
// overlay-mode branch of fork that deep-copies overrides and shares the base), then
// accumulated overrides trigger promoteOverlay on the forked copy.
//
// After every RecomputeTrie call, the resulting root is compared against a fresh trie
// built from the same elements via NewFieldTrie to ensure correctness.
func TestFieldTrie_RecomputeAccumulatedPromotion(t *testing.T) {
	const promotionThreshold = 3

	type step int
	const (
		createOverlay step = iota
		accumulate
		triggerPromotion
	)

	type testCase struct {
		name     string
		field    types.FieldIndex
		dataType types.DataType
		elements any
		length   uint64
		// steps[0]: creates the overlay via fork (3 leaf overrides).
		// steps[1]: adds 1 more override via recomputeOverlay (4 total, still no promotion because check is before add).
		// steps[2]: triggers promoteOverlay because len(levels[0])=4 > 3.
		steps [3]recomputeStep
		// overlayCopySteps mirrors the same 3-step pattern but on a copy of an overlay,
		// exercising the overlay-mode branch of fork() before accumulated promotion.
		overlayCopySteps [3]recomputeStep
	}

	blockRoots, votes, balances := newTestElements()

	tests := []testCase{
		{
			name:     "BasicArray",
			field:    types.BlockRoots,
			dataType: types.BasicArray,
			elements: blockRoots,
			length:   testBlockRootsSize,
			steps: [3]recomputeStep{
				{
					changedIndices: []uint64{0, 10, 20},
					mutate: func() {
						for _, idx := range []uint64{0, 10, 20} {
							binary.LittleEndian.PutUint64(blockRoots[idx][:8], uint64(idx)+1000)
						}
					},
				},
				{
					changedIndices: []uint64{30},
					mutate: func() {
						binary.LittleEndian.PutUint64(blockRoots[30][:8], 30+2000)
					},
				},
				{
					changedIndices: []uint64{40},
					mutate: func() {
						binary.LittleEndian.PutUint64(blockRoots[40][:8], 40+3000)
					},
				},
			},
			overlayCopySteps: [3]recomputeStep{
				{
					changedIndices: []uint64{50, 60, 70},
					mutate: func() {
						for _, idx := range []uint64{50, 60, 70} {
							binary.LittleEndian.PutUint64(blockRoots[idx][:8], uint64(idx)+4000)
						}
					},
				},
				{
					changedIndices: []uint64{80},
					mutate: func() {
						binary.LittleEndian.PutUint64(blockRoots[80][:8], 80+5000)
					},
				},
				{
					changedIndices: []uint64{90},
					mutate: func() {
						binary.LittleEndian.PutUint64(blockRoots[90][:8], 90+6000)
					},
				},
			},
		},
		{
			name:     "CompositeArray",
			field:    types.Eth1DataVotes,
			dataType: types.CompositeArray,
			elements: votes,
			length:   testVotesSize,
			steps: [3]recomputeStep{
				{
					changedIndices: []uint64{0, 5, 10},
					mutate: func() {
						for _, idx := range []uint64{0, 5, 10} {
							votes[idx].DepositCount = uint64(idx) + 1000
						}
					},
				},
				{
					changedIndices: []uint64{15},
					mutate: func() {
						votes[15].DepositCount = 15 + 2000
					},
				},
				{
					changedIndices: []uint64{20},
					mutate: func() {
						votes[20].DepositCount = 20 + 3000
					},
				},
			},
			overlayCopySteps: [3]recomputeStep{
				{
					changedIndices: []uint64{25, 30, 35},
					mutate: func() {
						for _, idx := range []uint64{25, 30, 35} {
							votes[idx].DepositCount = uint64(idx) + 4000
						}
					},
				},
				{
					changedIndices: []uint64{40},
					mutate: func() {
						votes[40].DepositCount = 40 + 5000
					},
				},
				{
					changedIndices: []uint64{45},
					mutate: func() {
						votes[45].DepositCount = 45 + 6000
					},
				},
			},
		},
		{
			name:     "CompressedArray",
			field:    types.Balances,
			dataType: types.CompressedArray,
			elements: balances,
			length:   testNumBalances / 4,
			// Balances pack 4 uint64s per chunk. Use indices in distinct chunks
			// so each element-level index maps to a unique chunk-level override.
			// idx 0 -> chunk 0, idx 4 -> chunk 1, idx 8 -> chunk 2, idx 12 -> chunk 3, idx 16 -> chunk 4.
			steps: [3]recomputeStep{
				{
					changedIndices: []uint64{0, 4, 8},
					mutate: func() {
						for _, idx := range []uint64{0, 4, 8} {
							balances[idx] = uint64(idx) + 1_000_000
						}
					},
				},
				{
					changedIndices: []uint64{12},
					mutate: func() {
						balances[12] = 12 + 2_000_000
					},
				},
				{
					changedIndices: []uint64{16},
					mutate: func() {
						balances[16] = 16 + 3_000_000
					},
				},
			},
			overlayCopySteps: [3]recomputeStep{
				{
					changedIndices: []uint64{20, 24, 28},
					mutate: func() {
						for _, idx := range []uint64{20, 24, 28} {
							balances[idx] = uint64(idx) + 4_000_000
						}
					},
				},
				{
					changedIndices: []uint64{32},
					mutate: func() {
						balances[32] = 32 + 5_000_000
					},
				},
				{
					changedIndices: []uint64{36},
					mutate: func() {
						balances[36] = 36 + 6_000_000
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("accumulated promotion", func(t *testing.T) {
				ft, err := NewFieldTrie(tc.field, tc.dataType, tc.elements, tc.length, 0)
				require.NoError(t, err)

				// Set an absolute threshold small enough so promotion triggers with few indices.
				ft.promotionThreshold = promotionThreshold

				originalRoot, err := ft.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, uint(0), ft.dataRef.Refs())

				// Copy to make the trie shared (ref=2), which forces the first RecomputeTrie to fork.
				cp := ft.CopyTrie()

				// Step 1: Fork into an overlay with 3 leaf overrides (at the threshold, not above).
				tc.steps[createOverlay].mutate()
				overlay, step0Root, err := ft.RecomputeTrie(tc.steps[0].changedIndices, tc.elements)
				require.NoError(t, err)

				// Prevent the GC from collecting cp before RecomputeTrie, which would
				// decrement the ref count and skip the fork path we intend to test.
				runtime.KeepAlive(cp)
				require.Equal(t, uint(1), ft.dataRef.Refs())

				// The recomputed root must match a fresh trie built from the same elements.
				requireFreshTrieRoot(t, tc.field, tc.dataType, tc.elements, tc.length, promotionThreshold, step0Root)

				// Step 2: Add 1 more override via recomputeOverlay. The check (len(levels[0]) > 3)
				// evaluates BEFORE the new override is added, so 3 > 3 is false — no promotion yet.
				tc.steps[accumulate].mutate()
				overlay, step1Root, err := overlay.RecomputeTrie(tc.steps[1].changedIndices, tc.elements)
				require.NoError(t, err)

				// Still an overlay referencing the original as base.
				require.Equal(t, uint(1), ft.dataRef.Refs())

				// The recomputed root must match a fresh trie built from the same elements.
				requireFreshTrieRoot(t, tc.field, tc.dataType, tc.elements, tc.length, promotionThreshold, step1Root)

				// Step 3: Now len(levels[0]) = 4 > 3, which triggers promoteOverlay.
				// The overlay is rebuilt into an owned trie and releases its base reference.
				tc.steps[triggerPromotion].mutate()
				promoted, promotedRoot, err := overlay.RecomputeTrie(tc.steps[2].changedIndices, tc.elements)
				require.NoError(t, err)

				// The promoted root returned by RecomputeTrie must match TrieRoot() on the returned trie.
				promotedTrieRoot, err := promoted.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, promotedRoot, promotedTrieRoot)

				// The recomputed root must match a fresh trie built from the same elements.
				requireFreshTrieRoot(t, tc.field, tc.dataType, tc.elements, tc.length, promotionThreshold, promotedRoot)

				// The original's dataRef is 0: the base reference was released during promotion.
				require.Equal(t, uint(0), ft.dataRef.Refs())

				// The promoted trie's dataRef is 0: it is an independent owned trie.
				require.Equal(t, uint(0), promoted.dataRef.Refs())

				// The original's root must be unchanged.
				rootAfterPromotion, err := ft.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, originalRoot, rootAfterPromotion)
			})

			t.Run("overlay copy promotion", func(t *testing.T) {
				ft, err := NewFieldTrie(tc.field, tc.dataType, tc.elements, tc.length, 0)
				require.NoError(t, err)

				// Set an absolute threshold small enough so promotion triggers with few indices.
				ft.promotionThreshold = promotionThreshold

				// Copy to make the trie shared (ref=2), which forces the first RecomputeTrie to fork.
				cp := ft.CopyTrie()

				// Step A: Fork into an overlay with 3 leaf overrides (at the threshold, not above).
				tc.overlayCopySteps[createOverlay].mutate()
				overlay, overlayRoot, err := ft.RecomputeTrie(tc.overlayCopySteps[0].changedIndices, tc.elements)
				require.NoError(t, err)

				// Prevent the GC from collecting cp before RecomputeTrie, which would
				// decrement the ref count and skip the fork path we intend to test.
				runtime.KeepAlive(cp)

				// The overlay references ft as its base, so ft.dataRef is now 1.
				require.Equal(t, uint(1), ft.dataRef.Refs())

				// The overlay's own dataRef is 0: nobody references the overlay's data yet.
				require.Equal(t, uint(0), overlay.dataRef.Refs())

				// The recomputed root must match a fresh trie built from the same elements.
				requireFreshTrieRoot(t, tc.field, tc.dataType, tc.elements, tc.length, promotionThreshold, overlayRoot)

				// Save step A's root before making the overlay shared.
				stepARoot := overlayRoot

				// Copy the overlay to make it shared (ref=2), forcing the next RecomputeTrie to fork.
				overlayCp := overlay.CopyTrie()

				// Step B: Fork the overlay (overlay-mode fork: deep-copy overrides, share base),
				// then add 1 more override via recomputeOverlay. The check (len(levels[0]) > 3)
				// evaluates BEFORE the new override is added, so 3 > 3 is false — no promotion yet.
				// Use a new variable (overlay2) so the original overlay struct stays reachable
				// and its GC cleanup does not prematurely decrement ft.dataRef.
				tc.overlayCopySteps[accumulate].mutate()
				overlay2, overlay2Root, err := overlay.RecomputeTrie(tc.overlayCopySteps[1].changedIndices, tc.elements)
				require.NoError(t, err)

				// Prevent the GC from collecting overlayCp before RecomputeTrie, which would
				// decrement the ref count and skip the overlay-mode fork path we intend to test.
				runtime.KeepAlive(overlayCp)

				// The recomputed root must match a fresh trie built from the same elements.
				requireFreshTrieRoot(t, tc.field, tc.dataType, tc.elements, tc.length, promotionThreshold, overlay2Root)

				// Step C: overlay2 (from step B) is not shared (ref=1), so recompute happens in place.
				// Now len(levels[0]) = 4 > 3, which triggers promoteOverlay.
				// The overlay is rebuilt into an owned trie and releases its base reference.
				tc.overlayCopySteps[triggerPromotion].mutate()
				promoted, promotedRoot, err := overlay2.RecomputeTrie(tc.overlayCopySteps[2].changedIndices, tc.elements)
				require.NoError(t, err)

				// The promoted root returned by RecomputeTrie must match TrieRoot() on the returned trie.
				promotedTrieRoot, err := promoted.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, promotedRoot, promotedTrieRoot)

				// The recomputed root must match a fresh trie built from the same elements.
				requireFreshTrieRoot(t, tc.field, tc.dataType, tc.elements, tc.length, promotionThreshold, promotedRoot)

				// ft.dataRef is 2: step B's fork released its ref during step C's promotion,
				// but the step A overlay and its copy (overlayCp) both still hold a base reference to ft.
				require.Equal(t, uint(2), ft.dataRef.Refs())

				// The promoted trie's dataRef is 0: it is an independent owned trie.
				require.Equal(t, uint(0), promoted.dataRef.Refs())

				// The step A overlay's root must be unchanged.
				overlayRootAfterPromotion, err := overlay.TrieRoot()
				require.NoError(t, err)
				require.Equal(t, stepARoot, overlayRootAfterPromotion)

				// Keep overlay and overlayCp alive so their GC cleanups do not fire
				// before the dataRef check above.
				runtime.KeepAlive(overlay)
				runtime.KeepAlive(overlayCp)
			})
		})
	}
}

// TestFieldTrie_RecomputeOwned verifies the in-place recomputation path of RecomputeTrie
// for all three field trie data types: BasicArray, CompositeArray, and CompressedArray.
// When an owned (non-shared) trie is recomputed with changed indices, only the affected
// branches are updated in place via recomputeBranches. No fork or promotion occurs.
//
// After every RecomputeTrie call, the resulting root is compared against a fresh trie
// built from the same elements via NewFieldTrie to ensure correctness.
func TestFieldTrie_RecomputeOwned(t *testing.T) {
	type testCase struct {
		name           string
		field          types.FieldIndex
		dataType       types.DataType
		elements       any
		length         uint64
		changedIndices []uint64
		mutate         func()
	}

	blockRoots, votes, balances := newTestElements()

	tests := []testCase{
		{
			name:           "BasicArray",
			field:          types.BlockRoots,
			dataType:       types.BasicArray,
			elements:       blockRoots,
			length:         testBlockRootsSize,
			changedIndices: []uint64{0, 10, 20, 30},
			mutate: func() {
				for _, idx := range []uint64{0, 10, 20, 30} {
					binary.LittleEndian.PutUint64(blockRoots[idx][:8], uint64(idx)+999)
				}
			},
		},
		{
			name:           "CompositeArray",
			field:          types.Eth1DataVotes,
			dataType:       types.CompositeArray,
			elements:       votes,
			length:         testVotesSize,
			changedIndices: []uint64{0, 5, 10, 15},
			mutate: func() {
				for _, idx := range []uint64{0, 5, 10, 15} {
					votes[idx].DepositCount = uint64(idx) + 999
				}
			},
		},
		{
			name:           "CompressedArray",
			field:          types.Balances,
			dataType:       types.CompressedArray,
			elements:       balances,
			length:         testNumBalances / 4,
			changedIndices: []uint64{0, 7, 20, 50},
			mutate: func() {
				for _, idx := range []uint64{0, 7, 20, 50} {
					balances[idx] = uint64(idx) + 999_999
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ft, err := NewFieldTrie(tc.field, tc.dataType, tc.elements, tc.length, 0)
			require.NoError(t, err)

			originalRoot, err := ft.TrieRoot()
			require.NoError(t, err)
			require.Equal(t, uint(0), ft.dataRef.Refs())

			// Mutate elements and recompute in place.
			tc.mutate()

			returned, recomputedRoot, err := ft.RecomputeTrie(tc.changedIndices, tc.elements)
			require.NoError(t, err)

			// Not shared, so the returned trie is the same object (in-place recomputation).
			require.Equal(t, ft, returned)

			// The recomputed root must match TrieRoot() on the returned trie.
			returnedRoot, err := returned.TrieRoot()
			require.NoError(t, err)
			require.Equal(t, recomputedRoot, returnedRoot)

			// The recomputed root must match a fresh trie built from the same elements.
			requireFreshTrieRoot(t, tc.field, tc.dataType, tc.elements, tc.length, 0, recomputedRoot)

			// The root must have changed from the original (elements were mutated).
			require.NotEqual(t, originalRoot, recomputedRoot)

			// dataRef stays at 0: no sharing occurred.
			require.Equal(t, uint(0), ft.dataRef.Refs())
		})
	}
}

// TestFieldTrie_Empty verifies the Empty method for all relevant states:
// nil receiver, zero-value struct, populated trie, and overlay trie.
func TestFieldTrie_Empty(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var ft *FieldTrie
		require.Equal(t, true, ft.Empty())
	})

	t.Run("zero-value struct", func(t *testing.T) {
		ft := &FieldTrie{}
		require.Equal(t, true, ft.Empty())
	})

	t.Run("populated trie", func(t *testing.T) {
		blockRoots, _, _ := newTestElements()
		ft, err := NewFieldTrie(types.BlockRoots, types.BasicArray, blockRoots, testBlockRootsSize, 0)
		require.NoError(t, err)
		require.Equal(t, false, ft.Empty())
	})

	t.Run("overlay trie", func(t *testing.T) {
		_, overlay, _, _ := newTestOverlay(t)
		require.Equal(t, false, overlay.Empty())
	})
}

// TestFieldTrie_TrieRoot_Errors verifies that TrieRoot returns the expected errors
// for empty and invalid trie states.
func TestFieldTrie_TrieRoot_Errors(t *testing.T) {
	t.Run("empty trie returns ErrEmptyFieldTrie", func(t *testing.T) {
		ft := &FieldTrie{}
		_, err := ft.TrieRoot()
		require.ErrorIs(t, err, ErrEmptyFieldTrie)
	})

	t.Run("invalid trie with zero root level returns ErrInvalidFieldTrie", func(t *testing.T) {
		// Build a trie with nodesData set but an offsets table where the root level has 0 nodes.
		// offsets = [0, 0, 0] means depth=1, level 0 has 0 nodes, level 1 (root) has 0 nodes.
		ft := &FieldTrie{
			nodesData: &nodesData{
				nodes:   [][32]byte{},
				offsets: []uint64{0, 0, 0},
			},
			dataType: types.BasicArray,
		}
		_, err := ft.TrieRoot()
		require.ErrorIs(t, err, ErrInvalidFieldTrie)
	})

	t.Run("unrecognized data type returns error", func(t *testing.T) {
		// Build a valid owned trie but with an invalid dataType so that
		// rootWithMixin hits the default case.
		ft := &FieldTrie{
			nodesData: &nodesData{
				nodes:   make([][32]byte, 3), // 2 leaves + 1 root
				offsets: []uint64{0, 2, 3},   // depth=1: leaves [0,2), root [2,3)
			},
			dataType: types.DataType(255),
		}
		_, err := ft.TrieRoot()
		require.ErrorContains(t, "unrecognized data type", err)
	})
}

// TestFieldTrie_fork verifies the fork() method for edge cases:
// forking an empty trie and forking an overlay with nil override levels.
func TestFieldTrie_fork(t *testing.T) {
	t.Run("empty trie produces empty fork", func(t *testing.T) {
		ft, err := NewFieldTrie(types.BlockRoots, types.BasicArray, customtypes.BlockRoots{}, testBlockRootsSize, 0)
		require.NoError(t, err)

		// Clear trie data to make it empty.
		ft.nodesData = nil
		require.Equal(t, true, ft.Empty())

		// Call fork() directly to test the empty-fork path.
		forked := ft.fork()

		require.Equal(t, true, forked.Empty())
		require.Equal(t, uint(0), forked.dataRef.Refs())
	})

	t.Run("overlay fork skips nil levels", func(t *testing.T) {
		_, overlay, overlayRoot, _ := newTestOverlay(t)

		// Clear some intermediate override levels to nil to simulate an overlay
		// where not all levels have been populated. This lets us test that fork()
		// skips nil levels rather than allocating empty maps.
		nilledLevel := len(overlay.overridesData.levels) / 2
		overlay.overridesData.levels[nilledLevel] = nil

		// Fork the overlay directly.
		forked := overlay.fork()

		// The forked overlay must preserve nil levels: empty levels should not
		// be allocated as empty maps.
		for i, lvl := range forked.overridesData.levels {
			if overlay.overridesData.levels[i] == nil {
				require.Equal(t, true, lvl == nil, "forked level %d should be nil, not an empty map", i)
			} else {
				require.Equal(t, len(overlay.overridesData.levels[i]), len(lvl),
					"forked level %d should have the same number of entries", i)
			}
		}

		require.Equal(t, false, forked.Empty())

		// The original overlay's root must be unchanged.
		overlayRootAfterFork, err := overlay.TrieRoot()
		require.NoError(t, err)
		require.Equal(t, overlayRoot, overlayRootAfterFork)
	})

	t.Run("releaseBase noop on owned trie", func(t *testing.T) {
		blockRoots, _, _ := newTestElements()
		ft, err := NewFieldTrie(types.BlockRoots, types.BasicArray, blockRoots, testBlockRootsSize, 0)
		require.NoError(t, err)

		// Owned trie: base is nil, releaseBase should return immediately.
		require.Equal(t, uint(0), ft.dataRef.Refs())
		ft.releaseBase()
		require.Equal(t, uint(0), ft.dataRef.Refs())
	})
}

// TestFieldTrie_readOverlayNodeZeroHash verifies that readOverlayNode returns
// ZeroHashes[level] when the index is out of bounds for the base and has no override.
func TestFieldTrie_readOverlayNodeZeroHash(t *testing.T) {
	_, overlay, _, _ := newTestOverlay(t)

	// Read a leaf index beyond the base's level size. The overlay has no override
	// for this index, and the base doesn't have it, so readOverlayNode should
	// return ZeroHashes[0].
	outOfBoundsIdx := overlay.base.levelSize(0) + 100
	node, err := overlay.readOverlayNode(0, outOfBoundsIdx)
	require.NoError(t, err)
	require.Equal(t, trie.ZeroHashes[0], node)

	// Same check at a higher level.
	depth := overlay.base.depth()
	outOfBoundsIdx = overlay.base.levelSize(depth-1) + 10
	node, err = overlay.readOverlayNode(depth-1, outOfBoundsIdx)
	require.NoError(t, err)
	require.Equal(t, trie.ZeroHashes[depth-1], node)
}

// TestFieldTrie_compressedIndicesToChunks verifies chunk-level index conversion
// and deduplication for CompressedArray fields, and passthrough for other types.
func TestFieldTrie_compressedIndicesToChunks(t *testing.T) {
	t.Run("deduplicates indices mapping to same chunk", func(t *testing.T) {
		ft := &FieldTrie{
			field:    types.Balances,
			dataType: types.CompressedArray,
		}

		// Balances pack 4 uint64s per chunk.
		// Indices 0,1,2,3 all map to chunk 0; indices 4,5 map to chunk 1.
		chunks, err := ft.compressedIndicesToChunks([]uint64{0, 1, 2, 3, 4, 5})
		require.NoError(t, err)
		require.Equal(t, 2, len(chunks))
		require.Equal(t, uint64(0), chunks[0])
		require.Equal(t, uint64(1), chunks[1])
	})

	t.Run("non-compressed returns indices unchanged", func(t *testing.T) {
		ft := &FieldTrie{
			field:    types.BlockRoots,
			dataType: types.BasicArray,
		}

		indices := []uint64{0, 5, 10, 15}
		chunks, err := ft.compressedIndicesToChunks(indices)
		require.NoError(t, err)
		require.DeepEqual(t, indices, chunks)
	})
}

// TestFieldTrie_RecomputeOwnedGrowsBuffer verifies that recomputing an owned trie
// with an index beyond the current leaf count triggers ensureLeafCapacity, which
// grows the buffer and calls nodesData.updateMetrics.
func TestFieldTrie_RecomputeOwnedGrowsBuffer(t *testing.T) {
	// Start with a small Eth1DataVotes trie (2 elements).
	votes := make([]*ethpb.Eth1Data, 2)
	for i := range votes {
		votes[i] = &ethpb.Eth1Data{
			DepositRoot:  make([]byte, fieldparams.RootLength),
			DepositCount: uint64(i),
			BlockHash:    make([]byte, fieldparams.RootLength),
		}
	}

	ft, err := NewFieldTrie(types.Eth1DataVotes, types.CompositeArray, votes, testVotesSize, 0)
	require.NoError(t, err)

	originalLeafCount := ft.levelSize(0)

	// Grow the elements slice and recompute with an index beyond the current leaf count.
	// This forces ensureLeafCapacity → updateMetrics.
	grown := make([]*ethpb.Eth1Data, originalLeafCount+10)
	copy(grown, votes)
	for i := len(votes); i < len(grown); i++ {
		grown[i] = &ethpb.Eth1Data{
			DepositRoot:  make([]byte, fieldparams.RootLength),
			DepositCount: uint64(i) + 5000,
			BlockHash:    make([]byte, fieldparams.RootLength),
		}
	}

	// Recompute with all indices so every leaf is updated consistently.
	allIndices := make([]uint64, len(grown))
	for i := range allIndices {
		allIndices[i] = uint64(i)
	}

	returned, recomputedRoot, err := ft.RecomputeTrie(allIndices, grown)
	require.NoError(t, err)

	// The buffer must have grown.
	require.Equal(t, true, returned.levelSize(0) > originalLeafCount)

	// The recomputed root must match TrieRoot().
	trieRoot, err := returned.TrieRoot()
	require.NoError(t, err)
	require.Equal(t, recomputedRoot, trieRoot)

	// The result must match a fresh trie built from the same elements.
	requireFreshTrieRoot(t, types.Eth1DataVotes, types.CompositeArray, grown, testVotesSize, 0, recomputedRoot)
}
