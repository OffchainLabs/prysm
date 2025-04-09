package depositsnapshot

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/container/trie"
	"github.com/prysmaticlabs/prysm/v5/encoding/bytesutil"
	"github.com/prysmaticlabs/prysm/v5/testing/assert"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func hexString(t *testing.T, hexStr string) [32]byte {
	t.Helper()
	b, err := hex.DecodeString(hexStr)
	require.NoError(t, err)
	if len(b) != 32 {
		assert.Equal(t, 32, len(b), "bad hash length, expected 32")
	}
	x := (*[32]byte)(b)
	return *x
}

func Test_create(t *testing.T) {
	tests := []struct {
		name   string
		leaves [][32]byte
		depth  uint64
		want   MerkleTreeNode
	}{
		{
			name:   "empty tree",
			leaves: nil,
			depth:  0,
			want:   &ZeroNode{},
		},
		{
			name:   "zero depth",
			leaves: [][32]byte{hexString(t, fmt.Sprintf("%064d", 0))},
			depth:  0,
			want:   &LeafNode{},
		},
		{
			name:   "depth of 1",
			leaves: [][32]byte{hexString(t, fmt.Sprintf("%064d", 0))},
			depth:  1,
			want:   &InnerNode{&LeafNode{}, &ZeroNode{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := create(tt.leaves, tt.depth); !reflect.DeepEqual(got, tt.want) {
				require.DeepEqual(t, tt.want, got)
			}
		})
	}
}

func Test_fromSnapshotParts(t *testing.T) {
	tests := []struct {
		name      string
		finalized [][32]byte
	}{
		{
			name:      "multiple deposits and multiple Finalized",
			finalized: [][32]byte{hexString(t, fmt.Sprintf("%064d", 1)), hexString(t, fmt.Sprintf("%064d", 2))},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test := NewDepositTree()
			for _, leaf := range tt.finalized {
				err := test.pushLeaf(leaf)
				require.NoError(t, err)
			}
			got, err := test.HashTreeRoot()
			require.NoError(t, err)

			transformed := make([][]byte, len(tt.finalized))
			for i := 0; i < len(tt.finalized); i++ {
				transformed[i] = bytesutil.SafeCopyBytes(tt.finalized[i][:])
			}
			generatedTrie, err := trie.GenerateTrieFromItems(transformed, 32)
			require.NoError(t, err)

			want, err := generatedTrie.HashTreeRoot()
			require.NoError(t, err)

			require.Equal(t, want, got)

			// Test finalization
			for i := 0; i < len(tt.finalized); i++ {
				err = test.Finalize(int64(i), tt.finalized[i], 0)
				require.NoError(t, err)
			}

			sShot, err := test.GetSnapshot()
			require.NoError(t, err)
			got, err = sShot.CalculateRoot()
			require.NoError(t, err)

			require.Equal(t, 1, len(sShot.finalized))
			require.Equal(t, want, got)

			// Build from the snapshot once more
			recovered, err := fromSnapshot(sShot)
			require.NoError(t, err)
			got, err = recovered.HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

func Test_generateProof(t *testing.T) {
	tests := []struct {
		name   string
		leaves uint64
	}{
		{
			name:   "1 leaf",
			leaves: 1,
		},
		{
			name:   "4 leaves",
			leaves: 4,
		},
		{
			name:   "10 leaves",
			leaves: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCases, err := readTestCases()
			require.NoError(t, err)
			tree := NewDepositTree()
			for _, c := range testCases[:tt.leaves] {
				err = tree.pushLeaf(c.DepositDataRoot)
				require.NoError(t, err)
			}
			for i := uint64(0); i < tt.leaves; i++ {
				leaf, proof := generateProof(tree.tree, i, DepositContractDepth)
				require.Equal(t, leaf, testCases[i].DepositDataRoot)
				calcRoot := merkleRootFromBranch(leaf, proof, i)
				require.Equal(t, tree.tree.GetRoot(), calcRoot)
			}
		})
	}
}
