package stateutil

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
)

// ProgressiveTree represents a progressive container's Merkle tree.
// It stores the full layers of each balanced subtree plus the spine
// connecting them, enabling incremental recomputation and proof generation.
type ProgressiveTree struct {
	// SubtreeLayers[k] contains the Merkle layers for the k-th balanced subtree.
	// SubtreeLayers[k][0] is the leaf layer, SubtreeLayers[k][depth] is the root.
	// Each subtree k has capacity 4^k leaves and depth 2*k.
	SubtreeLayers [][][][]byte

	// Spine contains the hash chain connecting subtree roots.
	// Spine[0] = root = hash(SubtreeLayers[0] root, Spine[1])
	// Spine[k] = hash(SubtreeLayers[k] root, Spine[k+1])
	// Spine[last] = hash(SubtreeLayers[last] root, zeroHash[subtreeDepth])
	//
	// len(Spine) == len(SubtreeLayers) == number of subtree levels.
	Spine [][]byte
}

// NumLevels returns the number of subtree levels in this progressive tree.
func (pt *ProgressiveTree) NumLevels() int {
	return len(pt.Spine)
}

// Root returns the progressive tree root (Spine[0]).
func (pt *ProgressiveTree) Root() [32]byte {
	if pt.NumLevels() == 0 {
		var zero [32]byte
		return zero
	}
	var root [32]byte
	copy(root[:], pt.Spine[0])
	return root
}

// MerkleizeProgressive builds a progressive Merkle tree from the given 32-byte leaf chunks.
// It implements the EIP-7916 merkleize_progressive algorithm, returning the full tree
// structure for incremental recomputation and proof generation.
//
// The leaves slice contains field roots in stable index order. Unused positions
// (beyond len(leaves)) are treated as zero hashes.
//
// This function is the progressive equivalent of stateutil.Merkleize.
func MerkleizeProgressive(leaves [][]byte) *ProgressiveTree {
	numLevels := progressiveNumLevels(len(leaves))
	if numLevels == 0 {
		// Empty tree: return a single zero root.
		return &ProgressiveTree{
			SubtreeLayers: nil,
			Spine:         nil,
		}
	}

	hashFunc := hash.CustomSHA256Hasher()
	subtreeLayers := make([][][][]byte, numLevels)

	// Build each balanced subtree.
	for k := range numLevels {
		capacity := progressiveSubtreeCapacity(k)
		depth := progressiveSubtreeDepth(k)
		start := progressiveSubtreeStart(k)

		// Collect leaves for this subtree, zero-padding to full capacity.
		subtreeLeaves := make([][]byte, capacity)
		for i := range capacity {
			globalIdx := start + i
			if globalIdx < len(leaves) {
				subtreeLeaves[i] = leaves[globalIdx]
			} else {
				subtreeLeaves[i] = trie.ZeroHashes[0][:]
			}
		}

		// Build the balanced Merkle tree for this subtree.
		if depth == 0 {
			// Level 0: single leaf, no hashing. One layer with one element.
			subtreeLayers[k] = [][][]byte{{subtreeLeaves[0]}}
		} else {
			subtreeLayers[k] = merkleizeBalanced(subtreeLeaves, depth, hashFunc)
		}
	}

	// Build the spine from bottom to top.
	spine := make([][]byte, numLevels)

	// Start from the deepest level (numLevels - 1).
	// Spine[numLevels-1] = hash(subtree[numLevels-1].root, zeroHash)
	// The zero hash for the spine at level k is the zero hash at the subtree's depth,
	// which represents an empty right child (the "rest of the progressive tree").
	// Per EIP-7916, the right child is merkleize_progressive([], num_leaves*4),
	// which returns Bytes32() (32 zero bytes) regardless of num_leaves.
	lastK := numLevels - 1
	lastSubtreeRoot := subtreeRoot(subtreeLayers[lastK])
	spine[lastK] = hashTwo(hashFunc, lastSubtreeRoot, make([]byte, 32))

	// Walk up: Spine[k] = hash(subtree[k].root, Spine[k+1])
	for k := numLevels - 2; k >= 0; k-- {
		subRoot := subtreeRoot(subtreeLayers[k])
		spine[k] = hashTwo(hashFunc, subRoot, spine[k+1])
	}

	return &ProgressiveTree{
		SubtreeLayers: subtreeLayers,
		Spine:         spine,
	}
}

// merkleizeBalanced builds a standard balanced Merkle tree from leaves of a given depth.
// Returns all layers: layers[0] = leaves, layers[depth] = [root].
// The number of leaves MUST equal 2^depth (i.e., already padded).
func merkleizeBalanced(leaves [][]byte, depth int, hashFunc func([]byte) [32]byte) [][][]byte {
	layers := make([][][]byte, depth+1)
	layers[0] = make([][]byte, len(leaves))
	copy(layers[0], leaves)

	currentLayer := layers[0]
	for d := range depth {
		nextLayer := make([][]byte, len(currentLayer)/2)
		for i := 0; i < len(currentLayer); i += 2 {
			h := hashTwo(hashFunc, currentLayer[i], currentLayer[i+1])
			nextLayer[i/2] = h
		}
		layers[d+1] = nextLayer
		currentLayer = nextLayer
	}
	return layers
}

// RecomputeProgressiveRoot updates the progressive tree after a leaf at globalIdx
// has changed, recomputing only the affected branch.
//
// This is the progressive-tree equivalent of stateutil.recomputeRootFromLayer.
// It modifies the tree in-place and returns the new root.
func (pt *ProgressiveTree) RecomputeProgressiveRoot(globalIdx int, newLeaf []byte) {
	level, localIdx := progressiveSubtreeForIndex(globalIdx)
	layers := pt.SubtreeLayers[level]

	if bytes.Equal(layers[0][localIdx], newLeaf) {
		// No change needed if the leaf is the same.
		return
	}

	hashFunc := hash.CustomSHA256Hasher()

	// Step 1: Update the leaf in the balanced subtree.
	if level == 0 {
		// Level 0: single leaf, just replace it.
		layers[0][0] = make([]byte, 32)
		copy(layers[0][0], newLeaf)
	} else {
		// Replace the leaf.
		layers[0][localIdx] = make([]byte, 32)
		copy(layers[0][localIdx], newLeaf)

		// Walk up the balanced subtree.
		currentIdx := localIdx
		var combined [64]byte
		for d := 0; d < progressiveSubtreeDepth(level); d++ {
			neighborIdx := currentIdx ^ 1
			isLeft := currentIdx%2 == 0

			var left, right []byte
			if isLeft {
				left = layers[d][currentIdx]
				right = layers[d][neighborIdx]
			} else {
				left = layers[d][neighborIdx]
				right = layers[d][currentIdx]
			}

			copy(combined[:32], left)
			copy(combined[32:], right)
			h := hashFunc(combined[:])
			parentIdx := currentIdx / 2
			layers[d+1][parentIdx] = make([]byte, 32)
			copy(layers[d+1][parentIdx], h[:])
			currentIdx = parentIdx
		}
	}

	// Step 2: Walk up the spine from `level` to the root.
	// Recompute Spine[k] = hash(subtree[k].root, Spine[k+1]) for k from level down to 0.

	// First, recompute the entry at the changed level.
	// If this is the deepest level (numLevels-1), the right child is zero (empty rest).
	// Otherwise, the right child is Spine[level+1].
	var rightChild []byte
	if level == pt.NumLevels()-1 {
		rightChild = make([]byte, 32)
	} else {
		rightChild = pt.Spine[level+1]
	}

	subRoot := subtreeRoot(pt.SubtreeLayers[level])
	pt.Spine[level] = hashTwo(hashFunc, subRoot, rightChild)

	// Walk up from level-1 to 0.
	for k := level - 1; k >= 0; k-- {
		subRoot = subtreeRoot(pt.SubtreeLayers[k])
		pt.Spine[k] = hashTwo(hashFunc, subRoot, pt.Spine[k+1])
	}
}
