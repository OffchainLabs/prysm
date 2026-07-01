package stateutil

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/crypto/hash/htr"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/math"
	"github.com/pkg/errors"
)

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
	chunkBuffer := bytes.NewBuffer([]byte{})
	chunkBuffer.Grow(64)
	for len(hashLayer) > 1 && i < len(layers) {
		if !math.IsPowerOf2(uint64(len(hashLayer))) {
			return nil, nil, errors.Errorf("hash layer is a non power of 2: %d", len(hashLayer))
		}
		hashLayer = htr.VectorizedSha256(hashLayer)
		layers[i] = hashLayer
		i++
	}
	return layers, hashLayer, nil
}
