package vrftree

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	testDepth          = 8
	testPopulatedDepth = 6
	testSubtreeHeight  = 3
)

var testSeed = [32]byte{0x01, 0x02, 0x03}

// naiveLeaf returns the leaf at `index` of the padded tree of VRF seed `seed`, as the package doc defines it.
func naiveLeaf(seed [32]byte, index, populatedDepth uint64) [32]byte {
	if index >= 1<<populatedDepth {
		return [32]byte{}
	}

	secret := naiveSecret(seed, index)
	return [32]byte(crypto.Keccak256([]byte("VRF_LEAF"), secret[:], binary.LittleEndian.AppendUint64(nil, index)))
}

// naiveSecret returns the secret of `index` of the tree of VRF seed `seed`, as the package doc defines it.
func naiveSecret(seed [32]byte, index uint64) [32]byte {
	return [32]byte(crypto.Keccak256([]byte("VRF_SECRET"), seed[:], binary.LittleEndian.AppendUint64(nil, index)))
}

// naiveOpens returns whether `proof` reveals the secret the leaf at `index` of the tree of `root` commits to.
func naiveOpens(proof [][32]byte, depth, index uint64, root [32]byte) bool {
	leaf := [32]byte(crypto.Keccak256([]byte("VRF_LEAF"), proof[0][:], binary.LittleEndian.AppendUint64(nil, index)))
	return naiveIsValidMerkleBranch(leaf, proof[1:], depth, index, root)
}

// naiveFullLeaf returns the leaf at `index` of a full tree of VRF seed `seed`: the leaves the validator client
// does not derive are arbitrary.
func naiveFullLeaf(seed [32]byte, index, populatedDepth uint64) [32]byte {
	if index >= 1<<populatedDepth {
		return [32]byte(crypto.Keccak256([]byte("genesis tool"), binary.LittleEndian.AppendUint64(nil, index)))
	}

	return naiveLeaf(seed, index, populatedDepth)
}

// naiveLevels returns every level of the tree of `depth` whose leaves are given by `leaf`, hashing it level by level.
func naiveLevels(depth uint64, leaf func(uint64) [32]byte) [][][32]byte {
	level := make([][32]byte, 1<<depth)
	for i := range level {
		level[i] = leaf(uint64(i))
	}

	levels := [][][32]byte{level}
	for len(level) > 1 {
		next := make([][32]byte, len(level)/2)
		for i := range next {
			next[i] = [32]byte(crypto.Keccak256(level[2*i][:], level[2*i+1][:]))
		}

		level = next
		levels = append(levels, level)
	}

	return levels
}

// naiveRoot returns the root of the padded tree of `depth` of VRF seed `seed`.
func naiveRoot(seed [32]byte, depth, populatedDepth uint64) [32]byte {
	levels := naiveLevels(depth, func(i uint64) [32]byte { return naiveLeaf(seed, i, populatedDepth) })
	return levels[depth][0]
}

// naiveFullTree returns the root of the full tree of `depth` of VRF seed `seed`, and the branch of its leaf at index 0.
func naiveFullTree(seed [32]byte, depth, populatedDepth uint64) ([32]byte, [][32]byte) {
	levels := naiveLevels(depth, func(i uint64) [32]byte { return naiveFullLeaf(seed, i, populatedDepth) })

	branch := make([][32]byte, depth)
	for level := range depth {
		branch[level] = levels[level][1]
	}

	return levels[depth][0], branch
}

// naiveIsValidMerkleBranch is `is_valid_vrf_proof` of Lighthouse.
func naiveIsValidMerkleBranch(leaf [32]byte, branch [][32]byte, depth, index uint64, root [32]byte) bool {
	if uint64(len(branch)) != depth {
		return false
	}

	value := leaf
	for i := range depth {
		if (index>>i)&1 == 1 {
			value = [32]byte(crypto.Keccak256(branch[i][:], value[:]))
			continue
		}

		value = [32]byte(crypto.Keccak256(value[:], branch[i][:]))
	}

	return value == root
}

func newTestTree(t *testing.T) *Tree {
	tree, err := newTree(t.Context(), testSeed, testDepth, testPopulatedDepth, testSubtreeHeight, nil)
	require.NoError(t, err)

	return tree
}

func newTestFullTree(t *testing.T) *Tree {
	root, branch := naiveFullTree(testSeed, testDepth, testPopulatedDepth)

	tree, err := newFullTree(t.Context(), testSeed, testDepth, testPopulatedDepth, testSubtreeHeight, root, branch)
	require.NoError(t, err)

	return tree
}

func TestNew(t *testing.T) {
	t.Run("nil secret key", func(t *testing.T) {
		_, err := New(t.Context(), nil)
		require.ErrorContains(t, "nil secret key", err)
	})

	t.Run("matches Lighthouse", func(t *testing.T) {
		t.Skip("the vector predates the leaves committing to secrets")

		// Test vector of Lighthouse's `pq_ethpandaops_keys_and_vrf_tree_boundaries`.
		secretKey, err := bls.SecretKeyFromBytes(hexutil.MustDecode("0x2892f3ba475c3640a67fe730798ad9c4eed3a6d934db85f91c41cfc837ce8834"))
		require.NoError(t, err)

		tree, err := New(t.Context(), secretKey)
		require.NoError(t, err)

		expected := [32]byte(hexutil.MustDecode("0x46f6772a35c14c7f8467e9d8791450ed3c6853191129876fc50e769ec9bac18d"))
		require.Equal(t, expected, tree.Root())

		// The SHA-256 hash of the bytes of the proof of each slot.
		for slot, digest := range map[primitives.Slot]string{
			0:                     "0x4107fa0f9dad85074520eb0cdcf35264504f076c0c50b8cf8103a509c372d86c",
			5:                     "0x4e0efa764e7c54ddbfba1f1266497f55b82a4d363d30f77dfb520904f59aa8d5",
			1<<PopulatedDepth - 1: "0x4be4c0afca9d7139ed7a1e52a399bf4787478cd561bb32a0138a0550fd44d7d5",
			1 << PopulatedDepth:   "0x3b7f91d3371503c6268e3fa967ca51cbe43b6803f7c9662d95734e1b1d36d381",
			1<<Depth - 1:          "0xa6e044ad1b73627b876ce31ecd9ee2c2093279ee56af9c67004f7a26cb49c5a4",
		} {
			proof, err := tree.Proof(slot)
			require.NoError(t, err)

			proofBytes := make([]byte, 0, 32*len(proof))
			for _, node := range proof {
				proofBytes = append(proofBytes, node[:]...)
			}

			require.Equal(t, [32]byte(hexutil.MustDecode(digest)), sha256.Sum256(proofBytes))
			require.Equal(t, uint64(slot) >= 1<<PopulatedDepth, proof[0] == [32]byte{})
			require.Equal(t, true, naiveOpens(proof, Depth, uint64(slot), expected))
		}
	})

	t.Run("invalid heights", func(t *testing.T) {
		for name, heights := range map[string][3]uint64{
			"subtree height zero":            {testDepth, testPopulatedDepth, 0},
			"subtree above populated leaves": {testDepth, testPopulatedDepth, testPopulatedDepth + 1},
			"populated depth beyond depth":   {testDepth, testDepth + 1, testSubtreeHeight},
			"depth beyond the tree":          {Depth + 1, testPopulatedDepth, testSubtreeHeight},
		} {
			t.Run(name, func(t *testing.T) {
				_, err := newTree(t.Context(), testSeed, heights[0], heights[1], heights[2], nil)
				require.ErrorContains(t, "are not ordered", err)
			})
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := newTree(ctx, testSeed, testDepth, testPopulatedDepth, testSubtreeHeight, nil)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("same root for every height", func(t *testing.T) {
		for populatedDepth := uint64(1); populatedDepth <= testDepth; populatedDepth++ {
			expected := naiveRoot(testSeed, testDepth, populatedDepth)

			for subtreeHeight := uint64(1); subtreeHeight <= populatedDepth; subtreeHeight++ {
				tree, err := newTree(t.Context(), testSeed, testDepth, populatedDepth, subtreeHeight, nil)
				require.NoError(t, err)
				require.Equal(t, expected, tree.Root())
			}
		}
	})
}

func TestNewFull(t *testing.T) {
	t.Run("nil secret key", func(t *testing.T) {
		_, err := NewFull(t.Context(), nil, [32]byte{}, [Depth][32]byte{})
		require.ErrorContains(t, "nil secret key", err)
	})

	t.Run("matches the state published for Lighthouse", func(t *testing.T) {
		t.Skip("the published state predates the leaves committing to secrets")

		// Test vector of Lighthouse's `full_tree_vrf_matches_published_slot_zero_state`: the validator of
		// derivation index 5 of the public ethPandaOps devnet mnemonic.
		secretKey, err := bls.SecretKeyFromBytes(hexutil.MustDecode("0x41f692b9306a699ce01ba5b13fd04776ef5898e0ab09f06e37ba22abe43e403e"))
		require.NoError(t, err)

		record, err := os.ReadFile("testdata/full-vrf-state.index-5.bin")
		require.NoError(t, err)
		require.Equal(t, 32+4+32*Depth, len(record))

		root := [32]byte(record[:32])
		require.Equal(t, [32]byte(hexutil.MustDecode("0xb81a580c74d39457d22233bd82cc0ac2d651a64e0f0cceb964ac83e2a172e0a2")), root)

		branch := [Depth][32]byte{}
		for level := range branch {
			branch[level] = [32]byte(record[36+32*level:])
		}

		tree, err := NewFull(t.Context(), secretKey, root, branch)
		require.NoError(t, err)
		require.Equal(t, root, tree.Root())

		for _, slot := range []primitives.Slot{0, 5, 1<<PopulatedDepth - 1} {
			proof, err := tree.Proof(slot)
			require.NoError(t, err)
			require.Equal(t, true, naiveOpens(proof, Depth, uint64(slot), root))
		}

		_, err = tree.Proof(1 << PopulatedDepth)
		require.ErrorContains(t, "beyond the 1048576 derived leaves", err)

		for _, level := range []int{0, PopulatedDepth, Depth - 1} {
			corrupted := branch
			corrupted[level] = [32]byte{}
			_, err := NewFull(t.Context(), secretKey, root, corrupted)
			require.NotNil(t, err)
		}

		_, err = NewFull(t.Context(), secretKey, [32]byte{}, branch)
		require.ErrorContains(t, "is not the root", err)
	})

	t.Run("matches the naive full tree", func(t *testing.T) {
		root, _ := naiveFullTree(testSeed, testDepth, testPopulatedDepth)
		require.Equal(t, root, newTestFullTree(t).Root())
	})

	t.Run("branch of the wrong length", func(t *testing.T) {
		root, branch := naiveFullTree(testSeed, testDepth, testPopulatedDepth)
		_, err := newFullTree(t.Context(), testSeed, testDepth, testPopulatedDepth, testSubtreeHeight, root, branch[1:])
		require.ErrorContains(t, "branch has 7 nodes instead of 8", err)
	})

	t.Run("zero sibling", func(t *testing.T) {
		// The branch of a padded tree.
		root := naiveRoot(testSeed, testDepth, testPopulatedDepth)
		padded := newTestTree(t)
		proof, err := padded.Proof(0)
		require.NoError(t, err)

		_, err = newFullTree(t.Context(), testSeed, testDepth, testPopulatedDepth, testSubtreeHeight, root, proof[1:])
		require.ErrorContains(t, "is the root of a zero subtree", err)
	})

	t.Run("wrong root", func(t *testing.T) {
		_, branch := naiveFullTree(testSeed, testDepth, testPopulatedDepth)
		_, err := newFullTree(t.Context(), testSeed, testDepth, testPopulatedDepth, testSubtreeHeight, [32]byte{0x01}, branch)
		require.ErrorContains(t, "is not the root", err)
	})

	t.Run("lower branch not matching the derived leaves", func(t *testing.T) {
		root, branch := naiveFullTree(testSeed, testDepth, testPopulatedDepth)
		branch[0] = [32]byte{0x01}

		_, err := newFullTree(t.Context(), testSeed, testDepth, testPopulatedDepth, testSubtreeHeight, root, branch)
		require.ErrorContains(t, "node of level 0 of the branch does not match", err)
	})

	t.Run("other seed", func(t *testing.T) {
		root, branch := naiveFullTree(testSeed, testDepth, testPopulatedDepth)
		_, err := newFullTree(t.Context(), [32]byte{0x04}, testDepth, testPopulatedDepth, testSubtreeHeight, root, branch)
		require.ErrorContains(t, "is not the root", err)
	})
}

func TestRoot(t *testing.T) {
	t.Run("depends on the seed", func(t *testing.T) {
		other, err := newTree(t.Context(), [32]byte{0x04}, testDepth, testPopulatedDepth, testSubtreeHeight, nil)
		require.NoError(t, err)
		require.NotEqual(t, newTestTree(t).Root(), other.Root())
	})

	t.Run("matches the naive root", func(t *testing.T) {
		require.Equal(t, naiveRoot(testSeed, testDepth, testPopulatedDepth), newTestTree(t).Root())
	})
}

func TestSecret(t *testing.T) {
	t.Run("every slot of a padded tree", func(t *testing.T) {
		tree := newTestTree(t)
		for slot := range primitives.Slot(1 << testPopulatedDepth) {
			leaf, err := tree.Secret(slot)
			require.NoError(t, err)
			require.Equal(t, naiveSecret(testSeed, uint64(slot)), leaf)
		}
	})

	t.Run("zero from the populated depth on in a padded tree", func(t *testing.T) {
		leaf, err := newTestTree(t).Secret(1 << testPopulatedDepth)
		require.NoError(t, err)
		require.Equal(t, [32]byte{}, leaf)
	})

	t.Run("derived slots of a full tree", func(t *testing.T) {
		tree := newTestFullTree(t)
		for slot := range primitives.Slot(1 << testPopulatedDepth) {
			leaf, err := tree.Secret(slot)
			require.NoError(t, err)
			require.Equal(t, naiveSecret(testSeed, uint64(slot)), leaf)
		}
	})

	t.Run("not derived slot of a full tree", func(t *testing.T) {
		_, err := newTestFullTree(t).Secret(1 << testPopulatedDepth)
		require.ErrorContains(t, "beyond the 64 derived leaves", err)
	})

	t.Run("slot beyond the leaves", func(t *testing.T) {
		_, err := newTestTree(t).Secret(1 << testDepth)
		require.ErrorContains(t, "beyond the 256 leaves", err)
	})
}

func TestProof(t *testing.T) {
	t.Run("every slot of a padded tree", func(t *testing.T) {
		tree := newTestTree(t)
		for slot := range primitives.Slot(1 << testDepth) {
			proof, err := tree.Proof(slot)
			if slot >= 1<<testPopulatedDepth {
				require.ErrorContains(t, "does not match the root", err)
				continue
			}

			require.NoError(t, err)
			require.Equal(t, testDepth+1, len(proof))

			leaf, err := tree.Secret(slot)
			require.NoError(t, err)
			require.Equal(t, leaf, proof[0])

			require.Equal(t, true, naiveOpens(proof, testDepth, uint64(slot), tree.Root()))
		}
	})

	t.Run("derived slots of a full tree", func(t *testing.T) {
		tree := newTestFullTree(t)
		for slot := range primitives.Slot(1 << testPopulatedDepth) {
			proof, err := tree.Proof(slot)
			require.NoError(t, err)
			require.Equal(t, testDepth+1, len(proof))
			require.Equal(t, naiveSecret(testSeed, uint64(slot)), proof[0])
			require.Equal(t, true, naiveOpens(proof, testDepth, uint64(slot), tree.Root()))
		}
	})

	t.Run("not derived slot of a full tree", func(t *testing.T) {
		_, err := newTestFullTree(t).Proof(1 << testPopulatedDepth)
		require.ErrorContains(t, "beyond the 64 derived leaves", err)
	})

	t.Run("not valid for another slot", func(t *testing.T) {
		tree := newTestTree(t)
		proof, err := tree.Proof(5)
		require.NoError(t, err)
		require.Equal(t, false, naiveIsValidMerkleBranch(proof[0], proof[1:], testDepth, 6, tree.Root()))
	})

	t.Run("slot beyond the leaves", func(t *testing.T) {
		_, err := newTestTree(t).Proof(1 << testDepth)
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

func TestVRFSeed(t *testing.T) {
	t.Run("matches the genesis tool", func(t *testing.T) {
		secretKey := []byte{0x01, 0x02, 0x03}
		require.Equal(t, sha256.Sum256(append([]byte("vrf-seed"), secretKey...)), vrfSeed(secretKey))
	})
}

func BenchmarkNew(b *testing.B) {
	for b.Loop() {
		_, err := newTree(b.Context(), testSeed, Depth, PopulatedDepth, subtreeHeight, nil)
		require.NoError(b, err)
	}
}

func BenchmarkProof(b *testing.B) {
	tree, err := newTree(b.Context(), testSeed, Depth, PopulatedDepth, subtreeHeight, nil)
	require.NoError(b, err)

	for b.Loop() {
		_, err := tree.Proof(12345)
		require.NoError(b, err)
	}
}
