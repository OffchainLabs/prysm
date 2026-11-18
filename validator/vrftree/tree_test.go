package vrftree

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const (
	testDepth         = 8
	testSubtreeHeight = 3
)

var testSeed = [32]byte{0x01, 0x02, 0x03}

// naiveLeaf returns the leaf at `index` of the validator whose XMSS seed is `xmssSeed`, as the package doc defines it.
func naiveLeaf(xmssSeed [32]byte, index uint64) [32]byte {
	vrfSeed := sha256.Sum256(append(xmssSeed[:], []byte("VRF_SEED")...))

	input := make([]byte, 64)
	copy(input, vrfSeed[:])
	binary.LittleEndian.PutUint64(input[32:], index)
	return sha256.Sum256(input)
}

// naiveRoot returns the root of the subtree of `height` whose first leaf is at index `first`,
// in the tree of the validator whose XMSS seed is `xmssSeed`, hashing level by level.
func naiveRoot(xmssSeed [32]byte, first, height uint64) [32]byte {
	level := make([][32]byte, 1<<height)
	for i := range level {
		level[i] = naiveLeaf(xmssSeed, first+uint64(i))
	}

	for len(level) > 1 {
		next := make([][32]byte, len(level)/2)
		for i := range next {
			next[i] = sha256.Sum256(append(level[2*i][:], level[2*i+1][:]...))
		}

		level = next
	}

	return level[0]
}

// naiveIsValidMerkleBranch is `is_valid_merkle_branch` of the spec.
func naiveIsValidMerkleBranch(leaf [32]byte, branch [][32]byte, depth, index uint64, root [32]byte) bool {
	if uint64(len(branch)) != depth {
		return false
	}

	value := leaf
	for i := range depth {
		if (index>>i)&1 == 1 {
			value = sha256.Sum256(append(branch[i][:], value[:]...))
			continue
		}

		value = sha256.Sum256(append(value[:], branch[i][:]...))
	}

	return value == root
}

func newTestTree(t *testing.T) *Tree {
	tree, err := newTree(t.Context(), testSeed, testDepth, testSubtreeHeight)
	require.NoError(t, err)
	return tree
}

func TestNew(t *testing.T) {
	t.Run("subtree height zero", func(t *testing.T) {
		_, err := newTree(t.Context(), testSeed, testDepth, 0)
		require.ErrorContains(t, "subtree height 0", err)
	})

	t.Run("subtree height beyond depth", func(t *testing.T) {
		_, err := newTree(t.Context(), testSeed, testDepth, testDepth+1)
		require.ErrorContains(t, "subtree height 9", err)
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := newTree(ctx, testSeed, testDepth, testSubtreeHeight)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("same root for every subtree height", func(t *testing.T) {
		expected := naiveRoot(testSeed, 0, testDepth)

		for subtreeHeight := uint64(1); subtreeHeight <= testDepth; subtreeHeight++ {
			tree, err := newTree(t.Context(), testSeed, testDepth, subtreeHeight)
			require.NoError(t, err)
			require.Equal(t, expected, tree.Root())
		}
	})
}

func TestRoot(t *testing.T) {
	t.Run("depends on the seed", func(t *testing.T) {
		other, err := newTree(t.Context(), [32]byte{0x04}, testDepth, testSubtreeHeight)
		require.NoError(t, err)
		require.NotEqual(t, newTestTree(t).Root(), other.Root())
	})

	t.Run("matches the naive root", func(t *testing.T) {
		require.Equal(t, naiveRoot(testSeed, 0, testDepth), newTestTree(t).Root())
	})
}

func TestLeaf(t *testing.T) {
	tree := newTestTree(t)

	t.Run("every slot", func(t *testing.T) {
		for slot := range primitives.Slot(1 << testDepth) {
			leaf, err := tree.Leaf(slot)
			require.NoError(t, err)
			require.Equal(t, naiveLeaf(testSeed, uint64(slot)), leaf)
		}
	})

	t.Run("slot beyond the leaves", func(t *testing.T) {
		_, err := tree.Leaf(1 << testDepth)
		require.ErrorContains(t, "beyond the 256 leaves", err)
	})
}

func TestProof(t *testing.T) {
	tree := newTestTree(t)

	t.Run("every slot", func(t *testing.T) {
		for slot := range primitives.Slot(1 << testDepth) {
			proof, err := tree.Proof(slot)
			require.NoError(t, err)
			require.Equal(t, testDepth+1, len(proof))

			leaf, err := tree.Leaf(slot)
			require.NoError(t, err)
			require.Equal(t, leaf, proof[0])

			require.Equal(t, true, naiveIsValidMerkleBranch(proof[0], proof[1:], testDepth, uint64(slot), tree.Root()))
		}
	})

	t.Run("not valid for another slot", func(t *testing.T) {
		proof, err := tree.Proof(5)
		require.NoError(t, err)
		require.Equal(t, false, naiveIsValidMerkleBranch(proof[0], proof[1:], testDepth, 6, tree.Root()))
	})

	t.Run("slot beyond the leaves", func(t *testing.T) {
		_, err := tree.Proof(1 << testDepth)
		require.ErrorContains(t, "beyond the 256 leaves", err)
	})

	t.Run("corrupted upper levels", func(t *testing.T) {
		corrupted := newTestTree(t)
		corrupted.upper[1][0][0] ^= 0xff // Upper node over subtrees 0 and 1.

		_, err := corrupted.Proof(5) // In subtree 0, whose branch goes through upper[1][1] only.
		require.NoError(t, err)

		_, err = corrupted.Proof(1 << (testSubtreeHeight + 1)) // In subtree 2, whose branch goes through upper[1][0].
		require.ErrorContains(t, "does not match the root", err)
	})
}

func TestSubtreeRoots(t *testing.T) {
	tree := newTestTree(t)

	t.Run("one root per subtree", func(t *testing.T) {
		roots := tree.SubtreeRoots()
		require.Equal(t, 1<<(testDepth-testSubtreeHeight), len(roots))

		for i, root := range roots {
			require.Equal(t, naiveRoot(testSeed, uint64(i)<<testSubtreeHeight, testSubtreeHeight), root)
		}
	})

	t.Run("copy", func(t *testing.T) {
		roots := tree.SubtreeRoots()
		roots[0][0] ^= 0xff
		require.NotEqual(t, roots[0], tree.SubtreeRoots()[0])
	})
}

func TestFromSubtreeRoots(t *testing.T) {
	tree := newTestTree(t)

	t.Run("same tree", func(t *testing.T) {
		rebuilt, err := fromSubtreeRoots(testSeed, tree.SubtreeRoots(), tree.Root(), testDepth, testSubtreeHeight)
		require.NoError(t, err)
		require.Equal(t, tree.Root(), rebuilt.Root())

		for slot := range primitives.Slot(1 << testDepth) {
			expected, err := tree.Proof(slot)
			require.NoError(t, err)

			actual, err := rebuilt.Proof(slot)
			require.NoError(t, err)
			require.DeepEqual(t, expected, actual)
		}
	})

	t.Run("wrong number of roots", func(t *testing.T) {
		roots := tree.SubtreeRoots()
		_, err := fromSubtreeRoots(testSeed, roots[1:], tree.Root(), testDepth, testSubtreeHeight)
		require.ErrorContains(t, "got 31 subtree roots, expected 32", err)
	})

	t.Run("corrupted root", func(t *testing.T) {
		roots := tree.SubtreeRoots()
		roots[7][3] ^= 0xff
		_, err := fromSubtreeRoots(testSeed, roots, tree.Root(), testDepth, testSubtreeHeight)
		require.ErrorContains(t, "is not the expected root", err)
	})

	t.Run("wrong expected root", func(t *testing.T) {
		_, err := fromSubtreeRoots(testSeed, tree.SubtreeRoots(), [32]byte{0x42}, testDepth, testSubtreeHeight)
		require.ErrorContains(t, "is not the expected root", err)
	})

	t.Run("not aliased", func(t *testing.T) {
		roots := tree.SubtreeRoots()
		rebuilt, err := fromSubtreeRoots(testSeed, roots, tree.Root(), testDepth, testSubtreeHeight)
		require.NoError(t, err)

		roots[0][0] ^= 0xff
		_, err = rebuilt.Proof(0)
		require.NoError(t, err)
	})

	t.Run("full depth", func(t *testing.T) {
		roots := make([][32]byte, 1<<(Depth-subtreeHeight))
		_, err := FromSubtreeRoots(testSeed, roots, [32]byte{})
		require.ErrorContains(t, "is not the expected root", err)
	})
}

func BenchmarkNew(b *testing.B) {
	// A tree of depth 24, 128 times smaller than a real one.
	for b.Loop() {
		_, err := newTree(b.Context(), testSeed, 24, subtreeHeight)
		require.NoError(b, err)
	}
}

func BenchmarkProof(b *testing.B) {
	tree, err := newTree(b.Context(), testSeed, Depth-8, subtreeHeight)
	require.NoError(b, err)

	for b.Loop() {
		_, err := tree.Proof(12345)
		require.NoError(b, err)
	}
}
