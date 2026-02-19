package stateutil

import (
	"encoding/binary"
	"math/bits"

	"github.com/OffchainLabs/prysm/v7/container/trie"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	pmath "github.com/OffchainLabs/prysm/v7/math"
	"github.com/pkg/errors"
)

// NextPow2 returns the smallest power of 2 >= n. Returns 1 for n <= 1.
func NextPow2(n int) int {
	if n <= 1 {
		return 1
	}
	return 1 << bits.Len(uint(n-1))
}

// computeOffsets builds the offset table for a flat trie buffer with pow2 level sizes.
// Used for fixed-size (BasicArray) tries where elements are already pow2-aligned.
// offsets[i] = start index of level i in the nodes slice.
// offsets[depth+1] = total number of nodes.
func computeOffsets(depth int, leafCap int) []int {
	offsets := make([]int, depth+2)
	total, size := 0, leafCap
	for i := 0; i <= depth; i++ {
		offsets[i] = total
		total += size
		if size > 1 {
			size /= 2
		}
	}
	offsets[depth+1] = total
	return offsets
}

// ComputeOffsetsVariable builds the offset table for a variable-size trie.
// Unlike computeOffsets, this uses exact level sizes (ceil division for parents)
// instead of pow2 padding. This eliminates ~61 MB of wasted padding per validator trie.
func ComputeOffsetsVariable(depth int, leafCount int) []int {
	offsets := make([]int, depth+2)
	total, size := 0, leafCount
	for i := 0; i <= depth; i++ {
		offsets[i] = total
		total += size
		if size > 1 {
			size = (size + 1) / 2
		}
	}
	offsets[depth+1] = total
	return offsets
}

// ReturnTrieLayer returns a flat contiguous merkle trie for a fixed-size array.
// All levels are packed into a single [][32]byte buffer with offsets for each level.
func ReturnTrieLayer(elements [][32]byte, length uint64) ([][32]byte, []int, error) {
	N := len(elements)
	depth := int(ssz.Depth(length)) // lint:ignore uintcast -- ssz.Depth returns uint8, always fits in int.

	if N == 1 {
		return [][32]byte{elements[0]}, []int{0, 1}, nil
	}

	if !pmath.IsPowerOf2(uint64(N)) {
		return nil, nil, errors.Errorf("elements length is not a power of 2: %d", N)
	}

	offsets := computeOffsets(depth, N)
	nodes := make([][32]byte, offsets[depth+1])
	copy(nodes[:N], elements)

	// Build upper levels using vectorized hashing.
	for level := range depth {
		levelSize := offsets[level+1] - offsets[level]
		src := nodes[offsets[level]:offsets[level+1]]
		if levelSize == 1 {
			// Single node at this level: hash with ZeroHashes[level].
			hasher := hash.CustomSHA256Hasher()
			var combined [64]byte
			copy(combined[:32], src[0][:])
			copy(combined[32:], trie.ZeroHashes[level][:])
			nodes[offsets[level+1]] = hasher(combined[:])
		} else {
			result := htr.VectorizedSha256(src)
			copy(nodes[offsets[level+1]:offsets[level+2]], result)
		}
	}
	return nodes, offsets, nil
}

// HashUpFromLeaves computes all upper levels of a flat trie buffer from level 0.
// nodes[offsets[0]:offsets[1]] must be pre-filled with leaf data.
// Odd-length levels are handled by hashing the last element with ZeroHashes[level].
func HashUpFromLeaves(nodes [][32]byte, offsets []int) {
	depth := len(offsets) - 2
	hasher := hash.CustomSHA256Hasher()
	var combined [64]byte

	for level := range depth {
		levelSize := offsets[level+1] - offsets[level]
		src := nodes[offsets[level]:offsets[level+1]]
		nextStart := offsets[level+1]

		if levelSize == 1 {
			copy(combined[:32], src[0][:])
			copy(combined[32:], trie.ZeroHashes[level][:])
			nodes[nextStart] = hasher(combined[:])
		} else if levelSize%2 == 0 {
			result := htr.VectorizedSha256(src)
			copy(nodes[nextStart:], result)
		} else {
			evenPart := levelSize - 1
			result := htr.VectorizedSha256(src[:evenPart])
			copy(nodes[nextStart:], result)
			copy(combined[:32], src[levelSize-1][:])
			copy(combined[32:], trie.ZeroHashes[level][:])
			nodes[nextStart+len(result)] = hasher(combined[:])
		}
	}
}

// ReturnTrieLayerVariable returns a flat contiguous merkle trie for a variable-size array.
// Uses exact level sizes (no pow2 padding) to minimize memory. Odd-length levels
// are handled by hashing the last element with ZeroHashes[level] separately.
func ReturnTrieLayerVariable(elements [][32]byte, length uint64) ([][32]byte, []int) {
	depth := int(ssz.Depth(length)) // lint:ignore uintcast -- ssz.Depth returns uint8, always fits in int.
	N := len(elements)

	if N == 0 {
		nodes := [][32]byte{trie.ZeroHashes[depth]}
		offsets := make([]int, depth+2)
		for i := 0; i <= depth; i++ {
			offsets[i] = 0
		}
		offsets[depth+1] = 1
		return nodes, offsets
	}

	offsets := ComputeOffsetsVariable(depth, N)
	nodes := make([][32]byte, offsets[depth+1])
	copy(nodes[:N], elements)
	HashUpFromLeaves(nodes, offsets)
	return nodes, offsets
}

// RecomputeFromLayer recomputes specific branches of a fixed-size trie.
// Modifies nodes in-place. BasicArray never grows, so nodes/offsets are stable.
func RecomputeFromLayer(changedLeaves [][32]byte, changedIdx []uint64, nodes [][32]byte, offsets []int) ([32]byte, error) {
	// Write changed leaves into level 0.
	for i, idx := range changedIdx {
		nodes[offsets[0]+int(idx)] = changedLeaves[i] // lint:ignore uintcast -- idx is bounded by trie level size.
	}

	depth := len(offsets) - 2
	if len(changedIdx) == 0 {
		return nodes[offsets[depth]], nil
	}

	// Even-neighbor offset check: ensure we recompute sibling pairs evenly.
	levelSize0 := offsets[1] - offsets[0]
	maxChangedIndex := changedIdx[len(changedIdx)-1]
	if int(maxChangedIndex+2) == levelSize0 && maxChangedIndex%2 != 0 {
		changedIdx = append(changedIdx, maxChangedIndex+1)
	}

	hasher := hash.CustomSHA256Hasher()
	var root [32]byte
	for _, idx := range changedIdx {
		ii, err := pmath.Int(idx)
		if err != nil {
			return [32]byte{}, err
		}
		root = recomputeBranch(ii, nodes, offsets, depth, hasher)
	}

	// Single leaf identity case.
	if levelSize0 == 1 {
		return nodes[offsets[0]], nil
	}
	return root, nil
}

// RecomputeFromLayerVariable recomputes specific branches of a variable-size trie.
// Returns potentially-grown nodes/offsets (growth is extremely rare with pow2 allocation).
func RecomputeFromLayerVariable(changedLeaves [][32]byte, changedIdx []uint64, nodes [][32]byte, offsets []int) ([32]byte, [][32]byte, []int, error) {
	depth := len(offsets) - 2
	if len(changedIdx) == 0 {
		return nodes[offsets[depth]], nodes, offsets, nil
	}

	hasher := hash.CustomSHA256Hasher()
	var root [32]byte
	for i, idx := range changedIdx {
		ii, err := pmath.Int(idx)
		if err != nil {
			return [32]byte{}, nil, nil, err
		}
		// Check if growth is needed (extremely rare with pow2 allocation).
		levelSize0 := offsets[1] - offsets[0]
		if ii >= levelSize0 {
			nodes, offsets = GrowFlatBuffer(nodes, offsets, ii+1)
		}
		nodes[offsets[0]+ii] = changedLeaves[i]
		root = recomputeBranch(ii, nodes, offsets, depth, hasher)
	}
	return root, nodes, offsets, nil
}

// recomputeBranch walks from a leaf index up to the root, recomputing parent hashes.
// Zero heap allocations per call — all writes go directly into the flat buffer.
func recomputeBranch(idx int, nodes [][32]byte, offsets []int, depth int, hasher func([]byte) [32]byte) [32]byte {
	root := nodes[offsets[0]+idx]
	currentIndex := idx
	var combinedChunks [64]byte

	for level := range depth {
		isLeft := currentIndex%2 == 0
		neighborIdx := currentIndex ^ 1
		levelSize := offsets[level+1] - offsets[level]

		var neighbor [32]byte
		if neighborIdx < levelSize {
			neighbor = nodes[offsets[level]+neighborIdx]
		} else {
			neighbor = trie.ZeroHashes[level]
		}

		if isLeft {
			copy(combinedChunks[:32], root[:])
			copy(combinedChunks[32:], neighbor[:])
		} else {
			copy(combinedChunks[:32], neighbor[:])
			copy(combinedChunks[32:], root[:])
		}

		root = hasher(combinedChunks[:])
		parentIdx := currentIndex / 2
		nodes[offsets[level+1]+parentIdx] = root
		currentIndex = parentIdx
	}
	return root
}

// GrowFlatBuffer grows the flat trie buffer to accommodate at least minLeafCount leaves.
// Called extremely rarely — only when validator count exceeds current capacity.
func GrowFlatBuffer(nodes [][32]byte, offsets []int, minLeafCount int) ([][32]byte, []int) {
	depth := len(offsets) - 2
	newOffsets := ComputeOffsetsVariable(depth, minLeafCount)
	newNodes := make([][32]byte, newOffsets[depth+1])

	for level := 0; level <= depth; level++ {
		oldSize := offsets[level+1] - offsets[level]
		newSize := newOffsets[level+1] - newOffsets[level]
		if oldSize > 0 {
			copy(newNodes[newOffsets[level]:], nodes[offsets[level]:offsets[level]+oldSize])
		}
		// Initialize new entries with ZeroHashes[level]. An uncomputed node
		// at level L represents an empty subtree whose root is ZeroHashes[L],
		// not the zero value ([32]byte{}). Level 0 is skipped because
		// ZeroHashes[0] == [32]byte{} (already zero-filled by make).
		if level > 0 {
			for i := oldSize; i < newSize; i++ {
				newNodes[newOffsets[level]+i] = trie.ZeroHashes[level]
			}
		}
	}
	return newNodes, newOffsets
}

// AddInMixin describes a method from which a length mixin is added to the
// provided root.
func AddInMixin(root [32]byte, length uint64) ([32]byte, error) {
	var rootBufRoot [32]byte
	binary.LittleEndian.PutUint64(rootBufRoot[:], length)
	return ssz.MixInLength(root, rootBufRoot[:]), nil
}

// Merkleize 32-byte leaves into a Merkle trie for its adequate depth, returning
// the resulting layers of the trie based on the appropriate depth. This function
// pads the leaves to a length of a multiple of 32.
func Merkleize(leaves [][]byte) [][][]byte {
	hashFunc := hash.CustomSHA256Hasher()
	layers := make([][][]byte, ssz.Depth(uint64(len(leaves)))+1)
	for len(leaves)%32 != 0 {
		leaves = append(leaves, make([]byte, 32))
	}
	currentLayer := leaves
	layers[0] = currentLayer

	// We keep track of the hash layers of a Merkle trie until we reach
	// the top layer of length 1, which contains the single root element.
	//        [Root]      -> Top layer has length 1.
	//    [E]       [F]   -> This layer has length 2.
	// [A]  [B]  [C]  [D] -> The bottom layer has length 4 (needs to be a power of two).
	i := 1
	for len(currentLayer) > 1 && i < len(layers) {
		layer := make([][]byte, 0)
		for i := 0; i < len(currentLayer); i += 2 {
			hashedChunk := hashFunc(append(currentLayer[i], currentLayer[i+1]...))
			layer = append(layer, hashedChunk[:])
		}
		currentLayer = layer
		layers[i] = currentLayer
		i++
	}
	return layers
}

// MerkleizeTrieLeaves merkleize the trie leaves.
func MerkleizeTrieLeaves(layers [][][32]byte, hashLayer [][32]byte) ([][][32]byte, [][32]byte, error) {
	// We keep track of the hash layers of a Merkle trie until we reach
	// the top layer of length 1, which contains the single root element.
	//        [Root]      -> Top layer has length 1.
	//    [E]       [F]   -> This layer has length 2.
	// [A]  [B]  [C]  [D] -> The bottom layer has length 4 (needs to be a power of two).
	i := 1
	for len(hashLayer) > 1 && i < len(layers) {
		if !pmath.IsPowerOf2(uint64(len(hashLayer))) {
			return nil, nil, errors.Errorf("hash layer is a non power of 2: %d", len(hashLayer))
		}
		hashLayer = htr.VectorizedSha256(hashLayer)
		layers[i] = hashLayer
		i++
	}
	return layers, hashLayer, nil
}
