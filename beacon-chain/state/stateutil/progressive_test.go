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
