package stateutil

import (
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestMerkleizeProgressive_MatchesSSZ(t *testing.T) {
	for _, count := range []int{0, 1, 2, 5, 6, 21, 22, 46, 85} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			leaves := progressiveTestLeaves(count)
			tree := MerkleizeProgressive(progressiveLeavesAsBytes(leaves))
			require.Equal(t, ssz.MerkleizeProgressiveChunks(leaves), tree.Root())
		})
	}
}

func TestProgressiveSubtreeStart(t *testing.T) {
	tests := []struct {
		level int
		want  int
	}{
		{level: 0, want: 0},
		{level: 1, want: 1},
		{level: 2, want: 5},
		{level: 3, want: 21},
		{level: 4, want: 85},
		{level: 5, want: 341},
	}

	for _, test := range tests {
		t.Run(strconv.Itoa(test.level), func(t *testing.T) {
			require.Equal(t, test.want, progressiveSubtreeStart(test.level))
		})
	}
}

func TestProgressiveTree_RecomputeRoot(t *testing.T) {
	for _, count := range []int{1, 5, 21, 46, 85} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			leaves := progressiveTestLeaves(count)
			tree := MerkleizeProgressive(progressiveLeavesAsBytes(leaves))

			indices := []int{0, count - 1}
			if count > 2 {
				indices = append(indices, count/2)
			}
			for _, index := range indices {
				leaves[index][0]++
				require.NoError(t, tree.RecomputeRoot(index, leaves[index]))
				require.Equal(t, ssz.MerkleizeProgressiveChunks(leaves), tree.Root())
			}
		})
	}
}

func TestProgressiveTree_RecomputeRootNoopForUnchangedLeaf(t *testing.T) {
	leaves := progressiveTestLeaves(21)
	tree := MerkleizeProgressive(progressiveLeavesAsBytes(leaves))
	root := tree.Root()

	require.NoError(t, tree.RecomputeRoot(13, leaves[13]))
	require.Equal(t, root, tree.Root())
}

func TestProgressiveTree_RecomputeRootErrors(t *testing.T) {
	var leaf [32]byte

	treeWithFiveLeaves := MerkleizeProgressive(progressiveLeavesAsBytes(progressiveTestLeaves(5)))
	malformedTree := MerkleizeProgressive(progressiveLeavesAsBytes(progressiveTestLeaves(5)))
	malformedTree.subtreeLayers[1][0] = malformedTree.subtreeLayers[1][0][:3]

	tests := []struct {
		name      string
		tree      *ProgressiveTree
		index     int
		wantError string
	}{
		{
			name:      "nil tree",
			tree:      nil,
			index:     0,
			wantError: "cannot recompute an empty progressive tree",
		},
		{
			name:      "empty tree",
			tree:      MerkleizeProgressive(nil),
			index:     0,
			wantError: "cannot recompute an empty progressive tree",
		},
		{
			name:      "negative index",
			tree:      treeWithFiveLeaves,
			index:     -1,
			wantError: "progressive leaf index -1 is outside tree capacity",
		},
		{
			name:      "index beyond tree capacity",
			tree:      treeWithFiveLeaves,
			index:     5,
			wantError: "progressive leaf index 5 is outside tree capacity",
		},
		{
			name:      "index beyond subtree capacity",
			tree:      malformedTree,
			index:     4,
			wantError: "progressive leaf index 4 is outside subtree capacity",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(t, test.wantError, test.tree.RecomputeRoot(test.index, leaf))
		})
	}
}

func TestProgressiveTree_Copy(t *testing.T) {
	leaves := progressiveTestLeaves(46)
	tree := MerkleizeProgressive(progressiveLeavesAsBytes(leaves))
	copied := tree.Copy()

	leaves[35][0]++
	require.NoError(t, copied.RecomputeRoot(35, leaves[35]))
	require.Equal(t, ssz.MerkleizeProgressiveChunks(leaves), copied.Root())
	require.DeepNotSSZEqual(t, tree.Root(), copied.Root())
}

func progressiveTestLeaves(count int) [][32]byte {
	leaves := make([][32]byte, count)
	for i := range leaves {
		leaves[i] = bytesutil.ToBytes32([]byte{byte(i + 1), byte(i >> 8)})
	}
	return leaves
}

func progressiveLeavesAsBytes(leaves [][32]byte) [][]byte {
	asBytes := make([][]byte, len(leaves))
	for i := range leaves {
		asBytes[i] = leaves[i][:]
	}
	return asBytes
}

func TestProgressiveSubtreeForIndex_ReturnsInvalidForNegativeIndex(t *testing.T) {
	subtreeIndex, localIndex := progressiveSubtreeForIndex(-1)
	require.Equal(t, -1, subtreeIndex)
	require.Equal(t, -1, localIndex)
}

func TestProgressiveSubtreeForIndex_MapsBoundaryAndInteriorIndices(t *testing.T) {
	tests := []struct {
		name        string
		globalIndex int
		wantSubtree int
		wantLocal   int
	}{
		{name: "first index in first subtree", globalIndex: 0, wantSubtree: 0, wantLocal: 0},
		{name: "first index in second subtree", globalIndex: 1, wantSubtree: 1, wantLocal: 0},
		{name: "last index in second subtree", globalIndex: 4, wantSubtree: 1, wantLocal: 3},
		{name: "first index in third subtree", globalIndex: 5, wantSubtree: 2, wantLocal: 0},
		{name: "interior index in third subtree", globalIndex: 13, wantSubtree: 2, wantLocal: 8},
		{name: "last index in third subtree", globalIndex: 20, wantSubtree: 2, wantLocal: 15},
		{name: "first index in fourth subtree", globalIndex: 21, wantSubtree: 3, wantLocal: 0},
		{name: "last index in fourth subtree", globalIndex: 84, wantSubtree: 3, wantLocal: 63},
		{name: "first index in fifth subtree", globalIndex: 85, wantSubtree: 4, wantLocal: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subtreeIndex, localIndex := progressiveSubtreeForIndex(test.globalIndex)
			require.Equal(t, test.wantSubtree, subtreeIndex)
			require.Equal(t, test.wantLocal, localIndex)
		})
	}
}

func TestProgressiveSubtreeForIndex_ReturnsLocalIndexRelativeToSubtreeStart(t *testing.T) {
	for level := 0; level <= 5; level++ {
		start := progressiveSubtreeStart(level)
		capacity := progressiveSubtreeCapacity(level)

		indices := []int{0, capacity - 1}
		if capacity > 2 {
			indices = append(indices, capacity/2)
		}

		for _, local := range indices {
			globalIndex := start + local
			subtreeIndex, localIndex := progressiveSubtreeForIndex(globalIndex)
			require.Equal(t, level, subtreeIndex)
			require.Equal(t, local, localIndex)
		}
	}
}
