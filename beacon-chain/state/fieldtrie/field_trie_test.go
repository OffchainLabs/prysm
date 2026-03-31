package fieldtrie

import (
	"crypto/rand"
	"testing"

	customtypes "github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/custom-types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/state-native/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/assert"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// TestFieldTrie_RecomputeEquivalence verifies that recomputing an owned trie
// in place produces the same root and internal nodes as building a fresh trie
// from the final elements.
//
// For each supported data type (BasicArray, CompositeArray, CompressedArray):
//  1. Build trieA from initial elements A.
//  2. Apply changes to A to get B.
//  3. RecomputeTrie on trieA with the changed indices and B.
//  4. Build trieB from scratch with B.
//  5. Assert roots are equal and all internal nodes match.
func TestFieldTrie_RecomputeEquivalence(t *testing.T) {
	t.Run("BasicArray_RandaoMixes", func(t *testing.T) {
		const numMixes = 64
		length := uint64(params.BeaconConfig().EpochsPerHistoricalVector)

		// Build initial elements A.
		mixesA := make(customtypes.RandaoMixes, numMixes)
		for i := range mixesA {
			_, err := rand.Read(mixesA[i][:])
			require.NoError(t, err)
		}

		trieA, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, mixesA, length)
		require.NoError(t, err)

		// Build B: copy A, then change indices 0, 5, 63.
		changedIdx := []uint64{0, 5, 63}
		mixesB := make(customtypes.RandaoMixes, numMixes)
		copy(mixesB, mixesA)
		for _, idx := range changedIdx {
			_, err := rand.Read(mixesB[idx][:])
			require.NoError(t, err)
		}

		// Recompute in place.
		trieA, recomputedRoot, err := trieA.RecomputeTrie(changedIdx, mixesB)
		require.NoError(t, err)

		// Build fresh trie from B.
		trieB, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, mixesB, length)
		require.NoError(t, err)
		freshRoot, err := trieB.TrieRoot()
		require.NoError(t, err)

		assert.Equal(t, freshRoot, recomputedRoot, "roots must match")
		assertNodesEqual(t, trieA, trieB)
	})

	t.Run("CompositeArray_Validators", func(t *testing.T) {
		const numVals = 32
		length := params.BeaconConfig().ValidatorRegistryLimit

		// Build initial elements A.
		valsA := make([]stateutil.CompactValidator, numVals)
		for i := range valsA {
			var pubkey [48]byte
			_, err := rand.Read(pubkey[:])
			require.NoError(t, err)
			valsA[i] = stateutil.CompactValidator{
				PublicKey:                  pubkey,
				EffectiveBalance:           32_000_000_000,
				ActivationEligibilityEpoch: primitives.Epoch(i),
				ActivationEpoch:            primitives.Epoch(i),
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
			}
		}

		trieA, err := NewFieldTrie(types.Validators, types.CompositeArray, valsA, length)
		require.NoError(t, err)

		// Build B: copy A, then slash validators 2 and 29.
		changedIdx := []uint64{2, 29}
		valsB := make([]stateutil.CompactValidator, numVals)
		copy(valsB, valsA)
		valsB[2].Slashed = true
		valsB[2].ExitEpoch = 20
		valsB[29].Slashed = true
		valsB[29].ExitEpoch = 40

		// Recompute in place.
		trieA, recomputedRoot, err := trieA.RecomputeTrie(changedIdx, valsB)
		require.NoError(t, err)

		// Build fresh trie from B.
		trieB, err := NewFieldTrie(types.Validators, types.CompositeArray, valsB, length)
		require.NoError(t, err)
		freshRoot, err := trieB.TrieRoot()
		require.NoError(t, err)

		assert.Equal(t, freshRoot, recomputedRoot, "roots must match")
		assertNodesEqual(t, trieA, trieB)
	})

	t.Run("CompressedArray_Balances", func(t *testing.T) {
		const numBals = 32
		length := stateutil.ValidatorLimitForBalancesChunks()

		// Build initial elements A.
		balsA := make([]uint64, numBals)
		for i := range balsA {
			balsA[i] = 32_000_000_000
		}

		trieA, err := NewFieldTrie(types.Balances, types.CompressedArray, balsA, length)
		require.NoError(t, err)

		// Build B: copy A, then change balances at indices 4 and 8.
		changedIdx := []uint64{4, 8}
		balsB := make([]uint64, numBals)
		copy(balsB, balsA)
		balsB[4] = 100_000_000
		balsB[8] = 200_000_000

		// Recompute in place.
		trieA, recomputedRoot, err := trieA.RecomputeTrie(changedIdx, balsB)
		require.NoError(t, err)

		// Build fresh trie from B.
		trieB, err := NewFieldTrie(types.Balances, types.CompressedArray, balsB, length)
		require.NoError(t, err)
		freshRoot, err := trieB.TrieRoot()
		require.NoError(t, err)

		assert.Equal(t, freshRoot, recomputedRoot, "roots must match")
		assertNodesEqual(t, trieA, trieB)
	})
}

// TestFieldTrie_CopyTrieRootEquivalence verifies that CopyTrie produces
// correct overlays and that chained copies preserve immutability.
//
// For each supported data type (BasicArray, CompositeArray, CompressedArray):
//  1. Create trie A from elements, compute rootA.
//  2. Copy A → B, check rootA == rootB.
//  3. Modify B via RecomputeTrie, get rootB.
//  4. Create fresh trie from B's modified data, check its root == rootB.
//  5. Check A's root is still the original rootA (immutability of A).
//  6. Copy B → C, check rootB == rootC.
//  7. Modify C via RecomputeTrie, get rootC.
//  8. Check B's root is still rootB (immutability of B).
//  9. Create fresh trie from C's modified data, check its root == rootC.
func TestFieldTrie_CopyTrieRootEquivalence(t *testing.T) {
	t.Run("BasicArray_RandaoMixes", func(t *testing.T) {
		const numMixes = 64
		length := uint64(params.BeaconConfig().EpochsPerHistoricalVector)

		// Step 1: Create trie A.
		mixesA := make(customtypes.RandaoMixes, numMixes)
		for i := range mixesA {
			_, err := rand.Read(mixesA[i][:])
			require.NoError(t, err)
		}
		trieA, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, mixesA, length)
		require.NoError(t, err)
		rootA, err := trieA.TrieRoot()
		require.NoError(t, err)

		// Step 2: Copy A → B, check rootA == rootB.
		trieB := trieA.CopyTrie()
		rootB, err := trieB.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootA, rootB, "copy B root must match A")

		// Step 3: Modify B.
		changedIdxB := []uint64{0, 5, 63}
		mixesB := make(customtypes.RandaoMixes, numMixes)
		copy(mixesB, mixesA)
		for _, idx := range changedIdxB {
			_, err := rand.Read(mixesB[idx][:])
			require.NoError(t, err)
		}
		trieB, rootB, err = trieB.RecomputeTrie(changedIdxB, mixesB)
		require.NoError(t, err)

		// Step 4: Fresh trie from B's data, check root matches.
		freshB, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, mixesB, length)
		require.NoError(t, err)
		freshRootB, err := freshB.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, freshRootB, rootB, "fresh trie from B data must match rootB")

		// Step 5: A's root must be unchanged.
		rootAAfter, err := trieA.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootA, rootAAfter, "A must be immutable after modifying B")

		// Step 6: Copy B → C, check rootB == rootC.
		trieC := trieB.CopyTrie()
		rootC, err := trieC.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootB, rootC, "copy C root must match B")

		// Step 7: Modify C.
		changedIdxC := []uint64{10, 30}
		mixesC := make(customtypes.RandaoMixes, numMixes)
		copy(mixesC, mixesB)
		for _, idx := range changedIdxC {
			_, err := rand.Read(mixesC[idx][:])
			require.NoError(t, err)
		}
		trieC, rootC, err = trieC.RecomputeTrie(changedIdxC, mixesC)
		require.NoError(t, err)

		// Step 8: B's root must be unchanged.
		rootBAfter, err := trieB.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootB, rootBAfter, "B must be immutable after modifying C")

		// Step 9: Fresh trie from C's data, check root matches.
		freshC, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, mixesC, length)
		require.NoError(t, err)
		freshRootC, err := freshC.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, freshRootC, rootC, "fresh trie from C data must match rootC")
	})

	t.Run("CompositeArray_Validators", func(t *testing.T) {
		const numVals = 32
		length := params.BeaconConfig().ValidatorRegistryLimit

		// Step 1: Create trie A.
		valsA := make([]stateutil.CompactValidator, numVals)
		for i := range valsA {
			var pubkey [48]byte
			_, err := rand.Read(pubkey[:])
			require.NoError(t, err)
			valsA[i] = stateutil.CompactValidator{
				PublicKey:                  pubkey,
				EffectiveBalance:           32_000_000_000,
				ActivationEligibilityEpoch: primitives.Epoch(i),
				ActivationEpoch:            primitives.Epoch(i),
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
				WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
			}
		}
		trieA, err := NewFieldTrie(types.Validators, types.CompositeArray, valsA, length)
		require.NoError(t, err)
		rootA, err := trieA.TrieRoot()
		require.NoError(t, err)

		// Step 2: Copy A → B, check rootA == rootB.
		trieB := trieA.CopyTrie()
		rootB, err := trieB.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootA, rootB, "copy B root must match A")

		// Step 3: Modify B.
		changedIdxB := []uint64{2, 29}
		valsB := make([]stateutil.CompactValidator, numVals)
		copy(valsB, valsA)
		valsB[2].Slashed = true
		valsB[2].ExitEpoch = 20
		valsB[29].Slashed = true
		valsB[29].ExitEpoch = 40
		trieB, rootB, err = trieB.RecomputeTrie(changedIdxB, valsB)
		require.NoError(t, err)

		// Step 4: Fresh trie from B's data, check root matches.
		freshB, err := NewFieldTrie(types.Validators, types.CompositeArray, valsB, length)
		require.NoError(t, err)
		freshRootB, err := freshB.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, freshRootB, rootB, "fresh trie from B data must match rootB")

		// Step 5: A's root must be unchanged.
		rootAAfter, err := trieA.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootA, rootAAfter, "A must be immutable after modifying B")

		// Step 6: Copy B → C, check rootB == rootC.
		trieC := trieB.CopyTrie()
		rootC, err := trieC.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootB, rootC, "copy C root must match B")

		// Step 7: Modify C.
		changedIdxC := []uint64{5, 15}
		valsC := make([]stateutil.CompactValidator, numVals)
		copy(valsC, valsB)
		valsC[5].Slashed = true
		valsC[5].ExitEpoch = 50
		valsC[15].Slashed = true
		valsC[15].ExitEpoch = 60
		trieC, rootC, err = trieC.RecomputeTrie(changedIdxC, valsC)
		require.NoError(t, err)

		// Step 8: B's root must be unchanged.
		rootBAfter, err := trieB.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootB, rootBAfter, "B must be immutable after modifying C")

		// Step 9: Fresh trie from C's data, check root matches.
		freshC, err := NewFieldTrie(types.Validators, types.CompositeArray, valsC, length)
		require.NoError(t, err)
		freshRootC, err := freshC.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, freshRootC, rootC, "fresh trie from C data must match rootC")
	})

	t.Run("CompressedArray_Balances", func(t *testing.T) {
		const numBals = 32
		length := stateutil.ValidatorLimitForBalancesChunks()

		// Step 1: Create trie A.
		balsA := make([]uint64, numBals)
		for i := range balsA {
			balsA[i] = 32_000_000_000
		}
		trieA, err := NewFieldTrie(types.Balances, types.CompressedArray, balsA, length)
		require.NoError(t, err)
		rootA, err := trieA.TrieRoot()
		require.NoError(t, err)

		// Step 2: Copy A → B, check rootA == rootB.
		trieB := trieA.CopyTrie()
		rootB, err := trieB.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootA, rootB, "copy B root must match A")

		// Step 3: Modify B.
		changedIdxB := []uint64{4, 8}
		balsB := make([]uint64, numBals)
		copy(balsB, balsA)
		balsB[4] = 100_000_000
		balsB[8] = 200_000_000
		trieB, rootB, err = trieB.RecomputeTrie(changedIdxB, balsB)
		require.NoError(t, err)

		// Step 4: Fresh trie from B's data, check root matches.
		freshB, err := NewFieldTrie(types.Balances, types.CompressedArray, balsB, length)
		require.NoError(t, err)
		freshRootB, err := freshB.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, freshRootB, rootB, "fresh trie from B data must match rootB")

		// Step 5: A's root must be unchanged.
		rootAAfter, err := trieA.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootA, rootAAfter, "A must be immutable after modifying B")

		// Step 6: Copy B → C, check rootB == rootC.
		trieC := trieB.CopyTrie()
		rootC, err := trieC.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootB, rootC, "copy C root must match B")

		// Step 7: Modify C.
		changedIdxC := []uint64{0, 16}
		balsC := make([]uint64, numBals)
		copy(balsC, balsB)
		balsC[0] = 500_000_000
		balsC[16] = 600_000_000
		trieC, rootC, err = trieC.RecomputeTrie(changedIdxC, balsC)
		require.NoError(t, err)

		// Step 8: B's root must be unchanged.
		rootBAfter, err := trieB.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, rootB, rootBAfter, "B must be immutable after modifying C")

		// Step 9: Fresh trie from C's data, check root matches.
		freshC, err := NewFieldTrie(types.Balances, types.CompressedArray, balsC, length)
		require.NoError(t, err)
		freshRootC, err := freshC.TrieRoot()
		require.NoError(t, err)
		assert.Equal(t, freshRootC, rootC, "fresh trie from C data must match rootC")
	})
}

// TestFieldTrie_CopyRecomputeEquivalence verifies that copying a trie and
// recomputing it with modifications (overlay path) produces the same root
// as building a fresh trie from the modified elements.
//
// For each supported data type (BasicArray, CompositeArray, CompressedArray):
//  1. Build trieA from initial elements A.
//  2. Copy trieA to get an overlay trieCopy.
//  3. Build B = A with modifications applied.
//  4. RecomputeTrie on trieCopy with the changed indices and B.
//  5. Build trieB from scratch with B.
//  6. Assert both roots are equal.
//
// Two sub-groups exercise both overlay code paths:
//   - BelowPromotionThreshold: small change set, stays in overlay mode.
//   - AbovePromotionThreshold: accumulates >overlayPromotionThreshold dirty
//     leaves, then triggers promoteOverlay on the next RecomputeTrie call.
//     After promotion the trie is owned, so roots and internal nodes are compared.
func TestFieldTrie_CopyRecomputeEquivalence(t *testing.T) {
	t.Run("BelowPromotionThreshold", func(t *testing.T) {
		t.Run("BasicArray_RandaoMixes", func(t *testing.T) {
			const numMixes = 64
			length := uint64(params.BeaconConfig().EpochsPerHistoricalVector)

			mixesA := make(customtypes.RandaoMixes, numMixes)
			for i := range mixesA {
				_, err := rand.Read(mixesA[i][:])
				require.NoError(t, err)
			}

			trieA, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, mixesA, length)
			require.NoError(t, err)

			trieCopy := trieA.CopyTrie()

			changedIdx := []uint64{0, 5, 63}
			mixesB := make(customtypes.RandaoMixes, numMixes)
			copy(mixesB, mixesA)
			for _, idx := range changedIdx {
				_, err := rand.Read(mixesB[idx][:])
				require.NoError(t, err)
			}

			trieCopy, recomputedRoot, err := trieCopy.RecomputeTrie(changedIdx, mixesB)
			require.NoError(t, err)

			trieB, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, mixesB, length)
			require.NoError(t, err)
			freshRoot, err := trieB.TrieRoot()
			require.NoError(t, err)

			assert.Equal(t, freshRoot, recomputedRoot, "roots must match")
		})

		t.Run("CompositeArray_Validators", func(t *testing.T) {
			const numVals = 32
			length := params.BeaconConfig().ValidatorRegistryLimit

			valsA := make([]stateutil.CompactValidator, numVals)
			for i := range valsA {
				var pubkey [48]byte
				_, err := rand.Read(pubkey[:])
				require.NoError(t, err)
				valsA[i] = stateutil.CompactValidator{
					PublicKey:                  pubkey,
					EffectiveBalance:           32_000_000_000,
					ActivationEligibilityEpoch: primitives.Epoch(i),
					ActivationEpoch:            primitives.Epoch(i),
					ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
					WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
				}
			}

			trieA, err := NewFieldTrie(types.Validators, types.CompositeArray, valsA, length)
			require.NoError(t, err)

			trieCopy := trieA.CopyTrie()

			changedIdx := []uint64{2, 29}
			valsB := make([]stateutil.CompactValidator, numVals)
			copy(valsB, valsA)
			valsB[2].Slashed = true
			valsB[2].ExitEpoch = 20
			valsB[29].Slashed = true
			valsB[29].ExitEpoch = 40

			trieCopy, recomputedRoot, err := trieCopy.RecomputeTrie(changedIdx, valsB)
			require.NoError(t, err)

			trieB, err := NewFieldTrie(types.Validators, types.CompositeArray, valsB, length)
			require.NoError(t, err)
			freshRoot, err := trieB.TrieRoot()
			require.NoError(t, err)

			assert.Equal(t, freshRoot, recomputedRoot, "roots must match")
		})

		t.Run("CompressedArray_Balances", func(t *testing.T) {
			const numBals = 32
			length := stateutil.ValidatorLimitForBalancesChunks()

			balsA := make([]uint64, numBals)
			for i := range balsA {
				balsA[i] = 32_000_000_000
			}

			trieA, err := NewFieldTrie(types.Balances, types.CompressedArray, balsA, length)
			require.NoError(t, err)

			trieCopy := trieA.CopyTrie()

			changedIdx := []uint64{4, 8}
			balsB := make([]uint64, numBals)
			copy(balsB, balsA)
			balsB[4] = 100_000_000
			balsB[8] = 200_000_000

			trieCopy, recomputedRoot, err := trieCopy.RecomputeTrie(changedIdx, balsB)
			require.NoError(t, err)

			trieB, err := NewFieldTrie(types.Balances, types.CompressedArray, balsB, length)
			require.NoError(t, err)
			freshRoot, err := trieB.TrieRoot()
			require.NoError(t, err)

			assert.Equal(t, freshRoot, recomputedRoot, "roots must match")
		})
	})

	t.Run("AbovePromotionThreshold", func(t *testing.T) {
		t.Run("BasicArray_RandaoMixes", func(t *testing.T) {
			const numMixes = 12_000
			length := uint64(params.BeaconConfig().EpochsPerHistoricalVector)

			mixesA := make(customtypes.RandaoMixes, numMixes)
			for i := range mixesA {
				_, err := rand.Read(mixesA[i][:])
				require.NoError(t, err)
			}

			trieA, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, mixesA, length)
			require.NoError(t, err)

			trieCopy := trieA.CopyTrie()

			// First recompute: fill overrides[0] past the threshold.
			firstBatchSize := overlayPromotionThreshold + 1
			firstIdx := make([]uint64, firstBatchSize)
			for i := range firstIdx {
				firstIdx[i] = uint64(i)
			}
			mixesB := make(customtypes.RandaoMixes, numMixes)
			copy(mixesB, mixesA)
			for _, idx := range firstIdx {
				_, err := rand.Read(mixesB[idx][:])
				require.NoError(t, err)
			}
			trieCopy, _, err = trieCopy.RecomputeTrie(firstIdx, mixesB)
			require.NoError(t, err)

			// Second recompute: triggers promotion.
			secondIdx := []uint64{uint64(numMixes - 1)}
			_, err = rand.Read(mixesB[secondIdx[0]][:])
			require.NoError(t, err)
			trieCopy, recomputedRoot, err := trieCopy.RecomputeTrie(secondIdx, mixesB)
			require.NoError(t, err)
			require.Equal(t, true, trieCopy.base == nil, "trie must have been promoted to owned mode")

			trieB, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, mixesB, length)
			require.NoError(t, err)
			freshRoot, err := trieB.TrieRoot()
			require.NoError(t, err)

			assert.Equal(t, freshRoot, recomputedRoot, "roots must match")
			assertNodesEqual(t, trieCopy, trieB)
		})

		t.Run("CompositeArray_Validators", func(t *testing.T) {
			const numVals = 12_000
			length := params.BeaconConfig().ValidatorRegistryLimit

			valsA := make([]stateutil.CompactValidator, numVals)
			for i := range valsA {
				var pubkey [48]byte
				_, err := rand.Read(pubkey[:])
				require.NoError(t, err)
				valsA[i] = stateutil.CompactValidator{
					PublicKey:                  pubkey,
					EffectiveBalance:           32_000_000_000,
					ActivationEligibilityEpoch: primitives.Epoch(i),
					ActivationEpoch:            primitives.Epoch(i),
					ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
					WithdrawableEpoch:          params.BeaconConfig().FarFutureEpoch,
				}
			}

			trieA, err := NewFieldTrie(types.Validators, types.CompositeArray, valsA, length)
			require.NoError(t, err)

			trieCopy := trieA.CopyTrie()

			// First recompute: fill overrides[0] past the threshold.
			firstBatchSize := overlayPromotionThreshold + 1
			firstIdx := make([]uint64, firstBatchSize)
			for i := range firstIdx {
				firstIdx[i] = uint64(i)
			}
			valsB := make([]stateutil.CompactValidator, numVals)
			copy(valsB, valsA)
			for _, idx := range firstIdx {
				valsB[idx].Slashed = true
				valsB[idx].ExitEpoch = primitives.Epoch(idx + 100)
			}
			trieCopy, _, err = trieCopy.RecomputeTrie(firstIdx, valsB)
			require.NoError(t, err)

			// Second recompute: triggers promotion.
			secondIdx := []uint64{uint64(numVals - 1)}
			valsB[secondIdx[0]].Slashed = true
			valsB[secondIdx[0]].ExitEpoch = 999
			trieCopy, recomputedRoot, err := trieCopy.RecomputeTrie(secondIdx, valsB)
			require.NoError(t, err)
			require.Equal(t, true, trieCopy.base == nil, "trie must have been promoted to owned mode")

			trieB, err := NewFieldTrie(types.Validators, types.CompositeArray, valsB, length)
			require.NoError(t, err)
			freshRoot, err := trieB.TrieRoot()
			require.NoError(t, err)

			assert.Equal(t, freshRoot, recomputedRoot, "roots must match")
			assertNodesEqual(t, trieCopy, trieB)
		})

		t.Run("CompressedArray_Balances", func(t *testing.T) {
			// 4 balances per chunk, so 40,004 balance changes → 10,001 chunk-level leaves.
			const numBals = 48_000
			length := stateutil.ValidatorLimitForBalancesChunks()

			balsA := make([]uint64, numBals)
			for i := range balsA {
				balsA[i] = 32_000_000_000
			}

			trieA, err := NewFieldTrie(types.Balances, types.CompressedArray, balsA, length)
			require.NoError(t, err)

			trieCopy := trieA.CopyTrie()

			// First recompute: change enough balances to fill >10K chunk-level overrides.
			// 4 balances per chunk → 40,004 balance indices = 10,001 unique chunks.
			firstBatchSize := (overlayPromotionThreshold + 1) * 4
			firstIdx := make([]uint64, firstBatchSize)
			for i := range firstIdx {
				firstIdx[i] = uint64(i)
			}
			balsB := make([]uint64, numBals)
			copy(balsB, balsA)
			for _, idx := range firstIdx {
				balsB[idx] = uint64(idx + 1)
			}
			trieCopy, _, err = trieCopy.RecomputeTrie(firstIdx, balsB)
			require.NoError(t, err)

			// Second recompute: triggers promotion.
			secondIdx := []uint64{uint64(numBals - 1)}
			balsB[secondIdx[0]] = 999
			trieCopy, recomputedRoot, err := trieCopy.RecomputeTrie(secondIdx, balsB)
			require.NoError(t, err)
			require.Equal(t, true, trieCopy.base == nil, "trie must have been promoted to owned mode")

			trieB, err := NewFieldTrie(types.Balances, types.CompressedArray, balsB, length)
			require.NoError(t, err)
			freshRoot, err := trieB.TrieRoot()
			require.NoError(t, err)

			assert.Equal(t, freshRoot, recomputedRoot, "roots must match")
			assertNodesEqual(t, trieCopy, trieB)
		})
	})
}

// TestFieldTrie_CopyTrieSharesRef verifies that CopyTrie returns a new
// trie sharing the same data with an incremented reference count, and that
// RecomputeTrie forks into a new independent trie when the reference count exceeds 1.
func TestFieldTrie_CopyTrieSharesRef(t *testing.T) {
	const numMixes = 64
	length := uint64(params.BeaconConfig().EpochsPerHistoricalVector)

	mixesA := make(customtypes.RandaoMixes, numMixes)
	for i := range mixesA {
		_, err := rand.Read(mixesA[i][:])
		require.NoError(t, err)
	}

	trieA, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, mixesA, length)
	require.NoError(t, err)
	require.Equal(t, true, trieA.base == nil)

	trieB := trieA.CopyTrie()
	require.Equal(t, true, trieA != trieB, "CopyTrie must return a new pointer")
	require.Equal(t, uint(2), trieA.ref.Refs(), "ref count must be 2 after copy")

	rootA, err := trieA.TrieRoot()
	require.NoError(t, err)

	// RecomputeTrie on shared trie must fork.
	changedIdx := []uint64{0, 5, 63}
	mixesB := make(customtypes.RandaoMixes, numMixes)
	copy(mixesB, mixesA)
	for _, idx := range changedIdx {
		_, err := rand.Read(mixesB[idx][:])
		require.NoError(t, err)
	}
	trieB, _, err = trieB.RecomputeTrie(changedIdx, mixesB)
	require.NoError(t, err)
	require.Equal(t, true, trieA != trieB, "RecomputeTrie must return a new trie when shared")

	// Original trie must be unchanged.
	rootAAfter, err := trieA.TrieRoot()
	require.NoError(t, err)
	assert.Equal(t, rootA, rootAAfter, "A must be immutable after forking B")

	// After fork: ref stays at 2 (no eager MinusRef; decremented when holder's BeaconState is GC'd),
	// dataRef incremented to 1 (base holds ref to protect shared nodes).
	require.Equal(t, uint(2), trieA.ref.Refs(), "ref count must be 2 after fork (decremented lazily by BeaconState GC)")
	require.Equal(t, uint(1), trieA.dataRef.Refs(), "dataRef count must be 1 after fork (base protects shared nodes)")
}

// TestFieldTrie_EdgeCases verifies error handling and edge cases.
func TestFieldTrie_EdgeCases(t *testing.T) {
	t.Run("NilElements", func(t *testing.T) {
		trie, err := NewFieldTrie(types.BlockRoots, types.BasicArray, nil, 8234)
		require.NoError(t, err)
		_, err = trie.TrieRoot()
		require.ErrorIs(t, err, ErrEmptyFieldTrie)
	})

	t.Run("UnknownType", func(t *testing.T) {
		_, err := NewFieldTrie(types.Balances, 4, []uint64{1, 2, 3}, 32)
		require.ErrorContains(t, "unrecognized data type", err)
	})

	t.Run("CopyEmpty", func(t *testing.T) {
		trie, err := NewFieldTrie(types.RandaoMixes, types.BasicArray, nil, uint64(params.BeaconConfig().EpochsPerHistoricalVector))
		require.NoError(t, err)

		copied := trie.CopyTrie()
		require.Equal(t, true, copied.Empty(), "copy of empty trie should be empty")
		require.Equal(t, trie.length, copied.length, "copy should preserve length")
	})
}

// FuzzFieldTrie exercises NewFieldTrie and TrieRoot with random inputs,
// looking for panics or unexpected crashes in the trie construction and
// hashing logic under arbitrary field indices, data types, element data,
// and trie lengths.
func FuzzFieldTrie(f *testing.F) {
	var seed []byte
	for range 40 {
		var root [32]byte
		_, _ = rand.Read(root[:])
		seed = append(seed, root[:]...)
	}
	f.Add(5, int(types.BasicArray), seed, uint64(params.BeaconConfig().SlotsPerHistoricalRoot))

	f.Fuzz(func(t *testing.T, fieldIdx, dataType int, data []byte, length uint64) {
		roots := make([][]byte, 0, len(data)/32)
		for i := 32; i <= len(data); i += 32 {
			roots = append(roots, data[i-32:i])
		}

		trie, err := NewFieldTrie(types.FieldIndex(fieldIdx), types.DataType(dataType), roots, length)
		if err != nil {
			return
		}

		_, _ = trie.TrieRoot()
	})
}

// assertNodesEqual compares the internal nodes of two owned tries.
func assertNodesEqual(t *testing.T, a, b *FieldTrie) {
	t.Helper()

	require.Equal(t, a.depth(), b.depth(), "trie depths must match")

	for level := range a.depth() + 1 {
		sizeA := a.levelSize(level)
		sizeB := b.levelSize(level)
		minSize := min(sizeA, sizeB)

		for i := range minSize {
			nodeA := a.nodes[a.offsets[level]+i]
			nodeB := b.nodes[b.offsets[level]+i]
			if nodeA != nodeB {
				t.Errorf("node mismatch at level %d index %d: got %x, want %x", level, i, nodeA, nodeB)
			}
		}
	}
}
