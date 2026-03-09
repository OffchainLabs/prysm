package stateutil

// progressiveSubtreeCapacity returns the leaf capacity of subtree at level k.
// Level 0 = 1, Level 1 = 4, Level 2 = 16, Level 3 = 64, ... (4^k)
func progressiveSubtreeCapacity(k int) int {
	if k < 0 {
		return 0
	}
	return 1 << (2 * k)
}

// progressiveSubtreeDepth returns the depth of the balanced subtree at level k.
// A subtree with 4^k leaves has depth 2*k (since 4^k = 2^(2k)).
// Special case: level 0 has 1 leaf and depth 0 (single node, no hashing needed).
func progressiveSubtreeDepth(k int) int {
	return 2 * k
}

// progressiveSubtreeStart returns the global leaf index of the first element
// in subtree at level k.
// Level 0 starts at 0, Level 1 starts at 1, Level 2 starts at 5, Level 3 starts at 21, ...
func progressiveSubtreeStart(k int) int {
	sum := 0
	for i := range k {
		sum += progressiveSubtreeCapacity(i)
	}
	return sum
}

// progressiveNumLevels returns how many subtree levels are needed to hold n leaves.
// Returns the smallest numLevels such that sum(4^k, k=0..numLevels-1) >= n.
func progressiveNumLevels(n int) int {
	if n <= 0 {
		return 0
	}
	levels := 0
	sum := 0
	for sum < n {
		sum += progressiveSubtreeCapacity(levels)
		levels++
	}

	return levels
}

// progressiveSubtreeForIndex returns which subtree level and local index
// a global leaf index maps to.
// Example: global index 6 → level=2, localIdx=1 (subtree 2 starts at 5, so 6-5=1)
func progressiveSubtreeForIndex(globalIdx int) (level int, localIdx int) {
	level = progressiveNumLevels(globalIdx+1) - 1
	localIdx = globalIdx - progressiveSubtreeStart(level)
	return level, localIdx
}

// hashTwo concatenates two 32-byte slices and hashes them.
func hashTwo(hashFunc func([]byte) [32]byte, left, right []byte) []byte {
	var combined [64]byte
	copy(combined[:32], left)
	copy(combined[32:], right)
	h := hashFunc(combined[:])
	result := make([]byte, 32)
	copy(result, h[:])
	return result
}

// subtreeRoot returns the root element of a subtree's layers.
// For depth-0 subtrees (single leaf), layers is just [][]byte{leaf}.
// For deeper subtrees, layers[depth] has one element: the root.
func subtreeRoot(layers [][][]byte) []byte {
	return layers[len(layers)-1][0]
}
