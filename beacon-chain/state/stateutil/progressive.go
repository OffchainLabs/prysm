package stateutil

import (
	"fmt"
	"slices"

	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

// ProgressiveTree caches the balanced subtrees and connecting spine of a
// progressive Merkle tree. It supports updating an existing leaf without
// rebuilding unrelated subtrees.
//
//		                    root
//		                     /\
//	                        /  \
//	subtree1(1): chunks[0:1]   /\
//	                          /  \
//	  subtree2(4): chunks[1:5]   /\
//	                            /  \
//	  subtree3(16): chunks[5:21]   /\
//	                              /  \
//	   subtree4(64): chunks[21:85]    0
type ProgressiveTree struct {
	subtreeLayers [][][][32]byte // subtreeLayers[i] is the merkle tree stored at subtree i. [subtree][layers][index]
	spine         [][32]byte     // the nodes between root and 0. corresponding to the root of each subtree.
}

// MerkleizeProgressive builds a progressive Merkle tree from 32-byte leaves.
func MerkleizeProgressive(leaves [][]byte) *ProgressiveTree {
	numLevels := progressiveNumLevels(len(leaves))
	if numLevels == 0 {
		return &ProgressiveTree{}
	}

	tree := &ProgressiveTree{
		subtreeLayers: make([][][][32]byte, numLevels),
		spine:         make([][32]byte, numLevels),
	}

	for level := range numLevels {
		capacity := progressiveSubtreeCapacity(level)
		start := progressiveSubtreeStart(level)
		subtreeLeaves := make([][32]byte, capacity)
		for i := range capacity {
			globalIndex := start + i
			if globalIndex >= len(leaves) {
				break
			}
			subtreeLeaves[i] = bytesutil.ToBytes32(leaves[globalIndex])
		}
		tree.subtreeLayers[level] = merkleizeProgressiveSubtree(subtreeLeaves)
	}

	tree.recomputeSpine(numLevels - 1)
	return tree
}

// Copy returns an independent copy of the progressive tree.
func (p *ProgressiveTree) Copy() *ProgressiveTree {
	if p == nil {
		return nil
	}

	copied := &ProgressiveTree{
		subtreeLayers: make([][][][32]byte, len(p.subtreeLayers)),
		spine:         slices.Clone(p.spine),
	}
	for subtreeIndex, subtree := range p.subtreeLayers {
		copied.subtreeLayers[subtreeIndex] = make([][][32]byte, len(subtree))
		for layerIndex, layer := range subtree {
			copied.subtreeLayers[subtreeIndex][layerIndex] = slices.Clone(layer)
		}
	}
	return copied
}

// Root returns the root of the progressive tree before any list or container
// mix-in is applied.
func (p *ProgressiveTree) Root() [32]byte {
	if p == nil || len(p.spine) == 0 {
		return [32]byte{}
	}
	return p.spine[0]
}

// RecomputeRoot updates one leaf and recomputes only its balanced subtree
// branch and the affected portion of the progressive spine.
func (p *ProgressiveTree) RecomputeRoot(globalIndex int, newLeaf [32]byte) error {
	if p == nil || len(p.subtreeLayers) == 0 {
		return fmt.Errorf("cannot recompute an empty progressive tree")
	}

	subtreeIndex, localIndex := progressiveSubtreeForIndex(globalIndex)
	if subtreeIndex < 0 || subtreeIndex >= len(p.subtreeLayers) {
		return fmt.Errorf("progressive leaf index %d is outside tree capacity", globalIndex)
	}

	layers := p.subtreeLayers[subtreeIndex]
	if localIndex < 0 || localIndex >= len(layers[0]) {
		return fmt.Errorf("progressive leaf index %d is outside subtree capacity", globalIndex)
	}
	if layers[0][localIndex] == newLeaf {
		return nil
	}

	layers[0][localIndex] = newLeaf
	hashFunc := hash.CustomSHA256Hasher()
	currentIndex := localIndex
	var pair [64]byte
	for layerIndex := 0; layerIndex < len(layers)-1; layerIndex++ {
		leftIndex := currentIndex &^ 1
		copy(pair[:32], layers[layerIndex][leftIndex][:])
		copy(pair[32:], layers[layerIndex][leftIndex+1][:])
		parent := hashFunc(pair[:])
		currentIndex /= 2
		layers[layerIndex+1][currentIndex] = parent
	}

	p.recomputeSpine(subtreeIndex)
	return nil
}

func (p *ProgressiveTree) recomputeSpine(from int) {
	hashFunc := hash.CustomSHA256Hasher()
	successor := [32]byte{}
	if from+1 < len(p.spine) {
		successor = p.spine[from+1]
	}

	var pair [64]byte
	for level := from; level >= 0; level-- {
		subtree := p.subtreeLayers[level]
		subtreeRoot := subtree[len(subtree)-1][0]
		copy(pair[:32], subtreeRoot[:])
		copy(pair[32:], successor[:])
		successor = hashFunc(pair[:])
		p.spine[level] = successor
	}
}

func merkleizeProgressiveSubtree(leaves [][32]byte) [][][32]byte {
	layers := make([][][32]byte, progressiveSubtreeDepthFromCapacity(len(leaves))+1)
	layers[0] = slices.Clone(leaves)
	hashFunc := hash.CustomSHA256Hasher()
	var pair [64]byte

	for layerIndex := 0; layerIndex < len(layers)-1; layerIndex++ {
		current := layers[layerIndex]
		next := make([][32]byte, len(current)/2)
		for i := range next {
			copy(pair[:32], current[2*i][:])
			copy(pair[32:], current[2*i+1][:])
			next[i] = hashFunc(pair[:])
		}
		layers[layerIndex+1] = next
	}
	return layers
}

func progressiveSubtreeCapacity(level int) int {
	return 1 << (2 * level) // equivalent to 4^level
}

func progressiveSubtreeStart(level int) int {
	start := 0
	for i := range level {
		start += progressiveSubtreeCapacity(i)
	}
	return start
}

func progressiveNumLevels(numLeaves int) int {
	levels := 0
	capacity := 0
	for capacity < numLeaves {
		capacity += progressiveSubtreeCapacity(levels)
		levels++
	}
	return levels
}

// returns the subtree index and the local index for a leaf index in the progressive tree.
func progressiveSubtreeForIndex(globalIndex int) (int, int) {
	if globalIndex < 0 {
		return -1, -1
	}
	level := progressiveNumLevels(globalIndex+1) - 1
	return level, globalIndex - progressiveSubtreeStart(level)
}

func progressiveSubtreeDepthFromCapacity(capacity int) int {
	depth := 0
	for capacity > 1 {
		capacity /= 2
		depth++
	}
	return depth
}
