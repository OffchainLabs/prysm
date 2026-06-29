package stateutil

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestProgressiveSubtreeCapacity(t *testing.T) {
	// should return 0 for negative levels
	require.Equal(t, 0, progressiveSubtreeCapacity(-2))
	// should be 4^k for k>=0
	require.Equal(t, 1, progressiveSubtreeCapacity(0))
	require.Equal(t, 4, progressiveSubtreeCapacity(1))
	require.Equal(t, 16, progressiveSubtreeCapacity(2))
	require.Equal(t, 64, progressiveSubtreeCapacity(3))
	require.Equal(t, 256, progressiveSubtreeCapacity(4))
	require.Equal(t, 1024, progressiveSubtreeCapacity(5))
}

func TestProgressiveSubtreeStart(t *testing.T) {
	require.Equal(t, 0, progressiveSubtreeStart(0))
	require.Equal(t, 1, progressiveSubtreeStart(1))
	require.Equal(t, 5, progressiveSubtreeStart(2))
	require.Equal(t, 21, progressiveSubtreeStart(3))
	require.Equal(t, 85, progressiveSubtreeStart(4))
	require.Equal(t, 341, progressiveSubtreeStart(5))
}

func TestProgressiveNumLevels(t *testing.T) {
	require.Equal(t, 0, progressiveNumLevels(-1))
	require.Equal(t, 0, progressiveNumLevels(0))
	require.Equal(t, 1, progressiveNumLevels(1))
	require.Equal(t, 2, progressiveNumLevels(2))
	require.Equal(t, 2, progressiveNumLevels(3))
	require.Equal(t, 2, progressiveNumLevels(5))
	require.Equal(t, 3, progressiveNumLevels(6))
	require.Equal(t, 3, progressiveNumLevels(21))
	require.Equal(t, 4, progressiveNumLevels(22))
	require.Equal(t, 4, progressiveNumLevels(85))
	require.Equal(t, 5, progressiveNumLevels(86))
	require.Equal(t, 5, progressiveNumLevels(341))
	require.Equal(t, 6, progressiveNumLevels(342))
}

func TestProgressiveSubtreeForIndex(t *testing.T) {
	level, localIdx := progressiveSubtreeForIndex(0)
	require.Equal(t, 0, level)
	require.Equal(t, 0, localIdx)

	level, localIdx = progressiveSubtreeForIndex(1)
	require.Equal(t, 1, level)
	require.Equal(t, 0, localIdx)

	level, localIdx = progressiveSubtreeForIndex(3)
	require.Equal(t, 1, level)
	require.Equal(t, 2, localIdx)

	level, localIdx = progressiveSubtreeForIndex(5)
	require.Equal(t, 2, level)
	require.Equal(t, 0, localIdx)

	level, localIdx = progressiveSubtreeForIndex(6)
	require.Equal(t, 2, level)
	require.Equal(t, 1, localIdx)

	level, localIdx = progressiveSubtreeForIndex(20)
	require.Equal(t, 2, level)
	require.Equal(t, 15, localIdx)

	level, localIdx = progressiveSubtreeForIndex(21)
	require.Equal(t, 3, level)
	require.Equal(t, 0, localIdx)

	level, localIdx = progressiveSubtreeForIndex(23)
	require.Equal(t, 3, level)
	require.Equal(t, 2, localIdx)

	level, localIdx = progressiveSubtreeForIndex(85)
	require.Equal(t, 4, level)
	require.Equal(t, 0, localIdx)

	level, localIdx = progressiveSubtreeForIndex(86)
	require.Equal(t, 4, level)
	require.Equal(t, 1, localIdx)

	level, localIdx = progressiveSubtreeForIndex(341)
	require.Equal(t, 5, level)
	require.Equal(t, 0, localIdx)
}

func TestHashTwo(t *testing.T) {
	hashFunc := hash.CustomSHA256Hasher()

	t.Run("two zero inputs produce known hash", func(t *testing.T) {
		result := hashTwo(hashFunc, trie.ZeroHashes[0][:], trie.ZeroHashes[0][:])
		require.Equal(t, 32, len(result))
		require.DeepSSZEqual(t, trie.ZeroHashes[1][:], result)
	})

	t.Run("matches manual sha256", func(t *testing.T) {
		a := make([]byte, 32)
		a[0] = 0x01
		b := make([]byte, 32)
		b[0] = 0x02
		result := hashTwo(hashFunc, a, b)
		// Compute the same thing manually.
		var combined [64]byte
		copy(combined[:32], a)
		copy(combined[32:], b)
		expected := hash.Hash(combined[:])
		require.DeepSSZEqual(t, expected[:], result)
	})

	t.Run("result is independent copy", func(t *testing.T) {
		a := make([]byte, 32)
		a[0] = 0x01
		b := make([]byte, 32)
		b[0] = 0x02
		result1 := hashTwo(hashFunc, a, b)

		var combined [64]byte
		copy(combined[:32], a)
		copy(combined[32:], b)
		expected := hash.Hash(combined[:])
		// Call again with different inputs — result1 must not change.
		c := make([]byte, 32)
		c[0] = 0xFF
		_ = hashTwo(hashFunc, c, c)

		require.DeepSSZEqual(t, expected[:], result1)
	})
}

func TestMerkleizeBalanced(t *testing.T) {
	hashFunc := hash.CustomSHA256Hasher()

	makeLeaf := func(tag byte) []byte {
		leaf := make([]byte, 32)
		leaf[0] = tag
		return leaf
	}

	t.Run("two leaves depth 1", func(t *testing.T) {
		a := makeLeaf(0x01)
		b := makeLeaf(0x02)
		layers := merkleizeBalanced([][]byte{a, b}, 1, hashFunc)

		// Should have 2 layers: leaves and root.
		require.Equal(t, 2, len(layers))
		require.Equal(t, 2, len(layers[0]))
		require.Equal(t, 1, len(layers[1]))

		// Root should be hash(a, b).
		expected := hashTwo(hashFunc, a, b)
		require.DeepSSZEqual(t, expected, layers[1][0])

		// Leaves should be unchanged.
		require.DeepSSZEqual(t, a, layers[0][0])
		require.DeepSSZEqual(t, b, layers[0][1])
	})

	t.Run("four leaves depth 2", func(t *testing.T) {
		leaves := [][]byte{makeLeaf(0x0A), makeLeaf(0x0B), makeLeaf(0x0C), makeLeaf(0x0D)}
		layers := merkleizeBalanced(leaves, 2, hashFunc)

		// 3 layers: 4 leaves, 2 parents, 1 root.
		require.Equal(t, 3, len(layers))
		require.Equal(t, 4, len(layers[0]))
		require.Equal(t, 2, len(layers[1]))
		require.Equal(t, 1, len(layers[2]))

		// verify leaves
		for i := 0; i < 4; i++ {
			require.DeepSSZEqual(t, leaves[i], layers[0][i])
		}

		// Verify parents.
		p0 := hashTwo(hashFunc, leaves[0], leaves[1])
		p1 := hashTwo(hashFunc, leaves[2], leaves[3])
		require.DeepSSZEqual(t, p0, layers[1][0])
		require.DeepSSZEqual(t, p1, layers[1][1])

		// Verify root.
		root := hashTwo(hashFunc, p0, p1)
		require.DeepSSZEqual(t, root, layers[2][0])
	})

	t.Run("layer dimensions halve at each level", func(t *testing.T) {
		// 16 leaves, depth 4.
		leaves := make([][]byte, 16)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i))
		}
		layers := merkleizeBalanced(leaves, 4, hashFunc)

		require.Equal(t, 5, len(layers))
		require.Equal(t, 16, len(layers[0]))
		require.Equal(t, 8, len(layers[1]))
		require.Equal(t, 4, len(layers[2]))
		require.Equal(t, 2, len(layers[3]))
		require.Equal(t, 1, len(layers[4]))
	})

	t.Run("every parent equals hash of its children", func(t *testing.T) {
		leaves := make([][]byte, 8)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i + 0x10))
		}
		layers := merkleizeBalanced(leaves, 3, hashFunc)

		// Check every non-leaf node.
		for d := 0; d < 3; d++ {
			for i := 0; i < len(layers[d+1]); i++ {
				expected := hashTwo(hashFunc, layers[d][2*i], layers[d][2*i+1])
				require.DeepSSZEqual(t, expected, layers[d+1][i])
			}
		}
	})

	t.Run("root matches ssz.MerkleizeVector", func(t *testing.T) {
		// Cross-check against the existing ssz.MerkleizeVector.
		leaves := make([][]byte, 4)
		chunks := make([][32]byte, 4)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i + 0x20))
			copy(chunks[i][:], leaves[i])
		}
		layers := merkleizeBalanced(leaves, 2, hashFunc)
		expected := ssz.MerkleizeVector(chunks, 4)
		require.DeepSSZEqual(t, expected[:], layers[2][0])
	})

	t.Run("all zero leaves match ZeroHashes", func(t *testing.T) {
		// A tree of all-zero leaves should produce ZeroHashes at each depth.
		leaves := make([][]byte, 4)
		for i := range leaves {
			leaves[i] = trie.ZeroHashes[0][:]
		}
		layers := merkleizeBalanced(leaves, 2, hashFunc)

		// ZeroHashes[1] = hash(ZeroHashes[0], ZeroHashes[0])
		require.DeepSSZEqual(t, trie.ZeroHashes[1][:], layers[1][0])
		require.DeepSSZEqual(t, trie.ZeroHashes[1][:], layers[1][1])
		// ZeroHashes[2] = hash(ZeroHashes[1], ZeroHashes[1])
		require.DeepSSZEqual(t, trie.ZeroHashes[2][:], layers[2][0])
	})
}

func TestMerkleizeProgressive(t *testing.T) {
	hashFunc := hash.CustomSHA256Hasher()
	zero := make([]byte, 32)

	makeLeaf := func(tag byte) []byte {
		leaf := make([]byte, 32)
		leaf[0] = tag
		return leaf
	}

	t.Run("empty tree returns zero root", func(t *testing.T) {
		tree := MerkleizeProgressive([][]byte{})
		require.Equal(t, 0, tree.NumLevels())
		require.DeepSSZEqual(t, [32]byte{}, tree.Root())
	})

	t.Run("single leaf", func(t *testing.T) {
		// merkleize_progressive([a]) = hash(a, Bytes32())
		// Subtree 0: single leaf = a
		// Spine[0] = hash(a, zero)  (deepest and only level)
		a := makeLeaf(0x01)
		tree := MerkleizeProgressive([][]byte{a})

		require.Equal(t, 1, tree.NumLevels())
		require.Equal(t, 1, len(tree.SubtreeLayers))
		require.Equal(t, 1, len(tree.Spine))

		// Subtree 0 stores the leaf.
		require.DeepSSZEqual(t, a, tree.SubtreeLayers[0][0][0])

		// Root = hash(a, zero).
		expected := hashTwo(hashFunc, a, zero)
		require.DeepSSZEqual(t, expected, tree.Spine[0])
		var expectedArr [32]byte
		copy(expectedArr[:], expected)
		require.DeepSSZEqual(t, expectedArr, tree.Root())
	})

	t.Run("two leaves span two subtrees", func(t *testing.T) {
		// Subtree 0: [a]  (1 leaf)
		// Subtree 1: [b, 0, 0, 0]  (4 leaves, 3 zero-padded)
		// Spine[1] = hash(subtree1.root, zero)
		// Spine[0] = hash(a, Spine[1])  = root
		a := makeLeaf(0x0A)
		b := makeLeaf(0x0B)
		tree := MerkleizeProgressive([][]byte{a, b})

		require.Equal(t, 2, tree.NumLevels())
		require.Equal(t, 2, len(tree.SubtreeLayers))

		// Subtree 0: single leaf.
		require.DeepSSZEqual(t, a, tree.SubtreeLayers[0][0][0])

		// Subtree 1: [b, 0, 0, 0], depth 2.
		require.Equal(t, 3, len(tree.SubtreeLayers[1])) // depth 2 → 3 layers
		require.Equal(t, 4, len(tree.SubtreeLayers[1][0]))
		require.DeepSSZEqual(t, b, tree.SubtreeLayers[1][0][0])
		for i := 1; i < 4; i++ {
			require.DeepSSZEqual(t, trie.ZeroHashes[0][:], tree.SubtreeLayers[1][0][i])
		}

		// Verify spine manually.
		sub1Root := tree.SubtreeLayers[1][2][0] // root of subtree 1
		spine1 := hashTwo(hashFunc, sub1Root, zero)
		spine0 := hashTwo(hashFunc, a, spine1)
		require.DeepSSZEqual(t, spine1, tree.Spine[1])
		require.DeepSSZEqual(t, spine0, tree.Spine[0])
	})

	t.Run("five leaves exactly fills two subtrees", func(t *testing.T) {
		// capacity(0) + capacity(1) = 1 + 4 = 5, so numLevels = 2.
		leaves := make([][]byte, 5)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i + 1))
		}
		tree := MerkleizeProgressive(leaves)

		require.Equal(t, 2, tree.NumLevels())

		// Subtree 0 has leaf[0].
		require.DeepSSZEqual(t, leaves[0], tree.SubtreeLayers[0][0][0])

		// Subtree 1 has leaves[1..4], all filled, no zero padding.
		for i := 0; i < 4; i++ {
			require.DeepSSZEqual(t, leaves[i+1], tree.SubtreeLayers[1][0][i])
		}
	})

	t.Run("six leaves spills into third subtree", func(t *testing.T) {
		// 6 > 5, so numLevels = 3. Subtree 2 has capacity 16, only 1 used.
		leaves := make([][]byte, 6)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i + 0x10))
		}
		tree := MerkleizeProgressive(leaves)

		require.Equal(t, 3, tree.NumLevels())
		require.Equal(t, 3, len(tree.SubtreeLayers))

		// Subtree 2: 16 leaves, only index 0 is leaves[5], rest zero.
		require.Equal(t, 16, len(tree.SubtreeLayers[2][0]))
		require.DeepSSZEqual(t, leaves[5], tree.SubtreeLayers[2][0][0])
		for i := 1; i < 16; i++ {
			require.DeepSSZEqual(t, trie.ZeroHashes[0][:], tree.SubtreeLayers[2][0][i])
		}

		// Spine has 3 entries.
		require.Equal(t, 3, len(tree.Spine))

		// Rebuild spine manually and verify root.
		sub2Root := subtreeRoot(tree.SubtreeLayers[2])
		sub1Root := subtreeRoot(tree.SubtreeLayers[1])
		spine2 := hashTwo(hashFunc, sub2Root, zero)
		spine1 := hashTwo(hashFunc, sub1Root, spine2)
		spine0 := hashTwo(hashFunc, leaves[0], spine1)
		require.DeepSSZEqual(t, spine2, tree.Spine[2])
		require.DeepSSZEqual(t, spine1, tree.Spine[1])
		require.DeepSSZEqual(t, spine0, tree.Spine[0])
	})

	t.Run("deterministic same input same root", func(t *testing.T) {
		leaves := make([][]byte, 10)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i))
		}
		root1 := MerkleizeProgressive(leaves).Root()
		root2 := MerkleizeProgressive(leaves).Root()
		require.DeepSSZEqual(t, root1, root2)
	})

	t.Run("different inputs produce different roots", func(t *testing.T) {
		leavesA := [][]byte{makeLeaf(0x01), makeLeaf(0x02)}
		leavesB := [][]byte{makeLeaf(0x01), makeLeaf(0x03)}
		rootA := MerkleizeProgressive(leavesA).Root()
		rootB := MerkleizeProgressive(leavesB).Root()
		require.DeepNotSSZEqual(t, rootA, rootB)
	})

	t.Run("subtree depths match expected formula", func(t *testing.T) {
		// 22 leaves → numLevels=4 (capacity 85).
		leaves := make([][]byte, 22)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i))
		}
		tree := MerkleizeProgressive(leaves)

		require.Equal(t, 4, tree.NumLevels())

		// Subtree k should have depth 2*k → (2*k + 1) layers.
		// k=0: depth 0, 1 layer.  k=1: depth 2, 3 layers.
		// k=2: depth 4, 5 layers.  k=3: depth 6, 7 layers.
		for k := 0; k < tree.NumLevels(); k++ {
			expectedLayers := progressiveSubtreeDepth(k) + 1
			require.Equal(t, expectedLayers, len(tree.SubtreeLayers[k]))
		}
	})
}

func TestRecomputeProgressiveRoot(t *testing.T) {
	makeLeaf := func(tag byte) []byte {
		leaf := make([]byte, 32)
		leaf[0] = tag
		return leaf
	}

	t.Run("update single leaf tree", func(t *testing.T) {
		tree := MerkleizeProgressive([][]byte{makeLeaf(0x01)})
		originalRoot := tree.Root()

		// Update the only leaf.
		tree.RecomputeProgressiveRoot(0, makeLeaf(0x02))
		newRoot := tree.Root()

		// Root must change.
		require.DeepNotSSZEqual(t, originalRoot, newRoot)

		// Must match a fresh tree built with the new leaf.
		fresh := MerkleizeProgressive([][]byte{makeLeaf(0x02)})
		require.DeepSSZEqual(t, fresh.Root(), newRoot)
	})

	t.Run("update leaf in subtree 0", func(t *testing.T) {
		leaves := make([][]byte, 6)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i + 1))
		}
		tree := MerkleizeProgressive(leaves)

		newLeaf := makeLeaf(0xFF)
		leaves[0] = newLeaf
		fresh := MerkleizeProgressive(leaves)

		require.DeepNotSSZEqual(t, tree.Root(), fresh.Root())

		// Change field 0 (subtree 0, the single-leaf subtree).
		tree.RecomputeProgressiveRoot(0, newLeaf)

		require.DeepSSZEqual(t, fresh.Root(), tree.Root())
	})

	t.Run("update leaf in subtree 1", func(t *testing.T) {
		leaves := make([][]byte, 6)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i + 1))
		}
		tree := MerkleizeProgressive(leaves)

		newLeaf := makeLeaf(0xAA)
		leaves[3] = newLeaf
		fresh := MerkleizeProgressive(leaves)

		require.DeepNotSSZEqual(t, tree.Root(), fresh.Root())

		// Change field 3 (subtree 1, localIdx 2).
		tree.RecomputeProgressiveRoot(3, newLeaf)

		require.DeepSSZEqual(t, fresh.Root(), tree.Root())
	})

	t.Run("update leaf in subtree 2", func(t *testing.T) {
		leaves := make([][]byte, 10)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i + 1))
		}
		tree := MerkleizeProgressive(leaves)

		newLeaf := makeLeaf(0xBB)
		leaves[7] = newLeaf
		fresh := MerkleizeProgressive(leaves)

		require.DeepNotSSZEqual(t, tree.Root(), fresh.Root())

		// Change field 7 (subtree 2, localIdx 2).
		tree.RecomputeProgressiveRoot(7, newLeaf)

		require.DeepSSZEqual(t, fresh.Root(), tree.Root())
	})

	t.Run("update zero-padded position in last subtree", func(t *testing.T) {
		// 6 leaves → 3 levels. Subtree 2 has capacity 16, positions 5-20.
		// Position 10 is currently zero. Update it.
		leaves := make([][]byte, 6)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i + 1))
		}
		tree := MerkleizeProgressive(leaves)

		newLeaf := makeLeaf(0xCC)
		tree.RecomputeProgressiveRoot(10, newLeaf)

		// Rebuild with extended leaves (zero-fill up to index 10, then set).
		extended := make([][]byte, 11)
		for i := range extended {
			if i < len(leaves) {
				extended[i] = makeLeaf(byte(i + 1))
			} else {
				extended[i] = make([]byte, 32) // zero
			}
		}
		extended[10] = newLeaf
		fresh := MerkleizeProgressive(extended)
		require.DeepSSZEqual(t, fresh.Root(), tree.Root())
	})

	t.Run("multiple sequential updates", func(t *testing.T) {
		leaves := make([][]byte, 10)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i))
		}
		tree := MerkleizeProgressive(leaves)

		// Update three different fields across different subtrees.
		tree.RecomputeProgressiveRoot(0, makeLeaf(0xA0)) // subtree 0
		tree.RecomputeProgressiveRoot(2, makeLeaf(0xA2)) // subtree 1
		tree.RecomputeProgressiveRoot(8, makeLeaf(0xA8)) // subtree 2

		// Rebuild from scratch.
		leaves[0] = makeLeaf(0xA0)
		leaves[2] = makeLeaf(0xA2)
		leaves[8] = makeLeaf(0xA8)
		fresh := MerkleizeProgressive(leaves)
		require.DeepSSZEqual(t, fresh.Root(), tree.Root())
	})

	t.Run("update same leaf twice", func(t *testing.T) {
		leaves := make([][]byte, 5)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i))
		}
		tree := MerkleizeProgressive(leaves)

		// First update.
		tree.RecomputeProgressiveRoot(3, makeLeaf(0xDD))
		// Second update overwrites the first.
		tree.RecomputeProgressiveRoot(3, makeLeaf(0xEE))

		leaves[3] = makeLeaf(0xEE)
		fresh := MerkleizeProgressive(leaves)
		require.DeepSSZEqual(t, fresh.Root(), tree.Root())
	})

	t.Run("update with same value is no-op", func(t *testing.T) {
		leaves := make([][]byte, 5)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i))
		}
		tree := MerkleizeProgressive(leaves)
		originalRoot := tree.Root()

		// "Update" with the same leaf value.
		tree.RecomputeProgressiveRoot(2, leaves[2])
		require.DeepSSZEqual(t, originalRoot, tree.Root())
	})

	t.Run("large tree update at boundary", func(t *testing.T) {
		// 22 fields → 4 levels (capacity 85).
		// Update the last real field (index 21, first position of subtree 3).
		leaves := make([][]byte, 22)
		for i := range leaves {
			leaves[i] = makeLeaf(byte(i))
		}
		tree := MerkleizeProgressive(leaves)

		newLeaf := makeLeaf(0xFF)
		tree.RecomputeProgressiveRoot(21, newLeaf)

		leaves[21] = newLeaf
		fresh := MerkleizeProgressive(leaves)
		require.DeepSSZEqual(t, fresh.Root(), tree.Root())
	})
}
