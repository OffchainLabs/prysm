package ssz

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/OffchainLabs/prysm/v7/crypto/hash"
)

const maxProgressiveActiveFields = 256

// MerkleizeProgressiveChunks computes the progressive Merkle root of 32-byte chunks.
//
// This is the EIP-7916 merkleize_progressive(chunks, num_leaves=1) helper.
// Chunks are split into progressively larger subtrees with capacities 1, 4, 16,
// 64, ...; each subtree root is then folded into the spine from deepest to
// shallowest by hashing hash(subtree_root, successor_root).
func MerkleizeProgressiveChunks(chunks [][32]byte) [32]byte {
	if len(chunks) == 0 {
		return [32]byte{}
	}

	n := len(chunks)
	start := 0
	subtreeCapacity := 1
	subtreeRoots := make([][32]byte, 0)

	for start < n {
		width := min(subtreeCapacity, n-start)

		subtree := make([][32]byte, width)
		copy(subtree, chunks[start:start+width])
		subtreeRoots = append(subtreeRoots, MerkleizeVector(subtree, uint64(subtreeCapacity)))

		start += width
		if start >= n {
			break
		}

		if subtreeCapacity > math.MaxInt/4 {
			subtreeCapacity = math.MaxInt
		} else {
			subtreeCapacity *= 4
		}
	}

	hashFunc := hash.CustomSHA256Hasher()

	// Fold successor roots from deepest subtree to shallowest subtree.
	root := [32]byte{}
	for i := len(subtreeRoots) - 1; i >= 0; i-- {
		root = hashPair(hashFunc, subtreeRoots[i], root)
	}
	return root
}

// MerkleizeVectorSSZProgressive hashes each element and computes the
// merkleize_progressive body root over the resulting field roots.
func MerkleizeVectorSSZProgressive[T Hashable](elements []T) ([32]byte, error) {
	roots := make([][32]byte, len(elements))
	for i, el := range elements {
		r, err := el.HashTreeRoot()
		if err != nil {
			return [32]byte{}, err
		}
		roots[i] = r
	}
	return MerkleizeProgressiveChunks(roots), nil
}

// MerkleizeListSSZProgressive hashes each element and computes the progressive
// list root by mixing in the element count.
func MerkleizeListSSZProgressive[T Hashable](elements []T) ([32]byte, error) {
	body, err := MerkleizeVectorSSZProgressive(elements)
	if err != nil {
		return [32]byte{}, err
	}

	var length [32]byte
	binary.LittleEndian.PutUint64(length[:8], uint64(len(elements)))
	return MixInLength(body, length[:]), nil
}

// SliceRootProgressive computes the progressive list root of hashable elements.
func SliceRootProgressive[T Hashable](slice []T) ([32]byte, error) {
	return MerkleizeListSSZProgressive(slice)
}

// ByteSliceRootProgressive computes the progressive list root of a byte slice
// interpreted as ProgressiveByteList (alias of ProgressiveList[byte]).
func ByteSliceRootProgressive(slice []byte) ([32]byte, error) {
	var chunks [][32]byte
	if len(slice) > 0 {
		var err error
		chunks, err = PackByChunk([][]byte{slice})
		if err != nil {
			return [32]byte{}, err
		}
	}

	bytesRoot := MerkleizeProgressiveChunks(chunks)
	var length [32]byte
	binary.LittleEndian.PutUint64(length[:8], uint64(len(slice)))
	return MixInLength(bytesRoot, length[:]), nil
}

// ContainerRootProgressive computes the progressive container root:
// mix_in_active_fields(merkleize_progressive(fieldRoots), activeFields).
func ContainerRootProgressive(fieldRoots [][32]byte, activeFields []bool) ([32]byte, error) {
	if len(activeFields) == 0 {
		return [32]byte{}, fmt.Errorf("active fields cannot be empty")
	}

	if len(activeFields) > maxProgressiveActiveFields {
		return [32]byte{}, fmt.Errorf("active fields length %d exceeds maximum %d", len(activeFields), maxProgressiveActiveFields)
	}

	activeCount := 0
	for _, active := range activeFields {
		if active {
			activeCount++
		}
	}

	if activeCount != len(fieldRoots) {
		return [32]byte{}, fmt.Errorf("active fields count %d does not match field roots count %d", activeCount, len(fieldRoots))
	}

	body := MerkleizeProgressiveChunks(fieldRoots)
	return MixInActiveFields(body, activeFields)
}

// MixInActiveFields computes hash(root, pack_bits(activeFields)) where
// activeFields is restricted to at most 256 bits.
func MixInActiveFields(root [32]byte, activeFields []bool) ([32]byte, error) {
	if len(activeFields) > maxProgressiveActiveFields {
		return [32]byte{}, fmt.Errorf("active fields length %d exceeds maximum %d", len(activeFields), maxProgressiveActiveFields)
	}

	var packed [32]byte
	for i, active := range activeFields {
		if !active {
			continue
		}
		packed[i/8] |= 1 << (uint(i) % 8)
	}
	return hashPair(hash.Hash, root, packed), nil
}

func hashPair(hashFunc func([]byte) [32]byte, left, right [32]byte) [32]byte {
	var input [64]byte
	copy(input[:32], left[:])
	copy(input[32:], right[:])
	return hashFunc(input[:])
}
