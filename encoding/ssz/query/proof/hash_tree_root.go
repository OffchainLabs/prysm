package proof

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	sszquery "github.com/OffchainLabs/prysm/v6/encoding/ssz/query"
)
// Public function to compute the hash tree root for a given sszInfo struct
// and a given byte slice containing the marshalled data. Entry point for external calls.
func HashTreeRootFromBytes(info *sszquery.SSZInfo, marshalledData []byte) ([32]byte, error) {
	if info == nil {
		return [32]byte{}, fmt.Errorf("nil sszInfo provided")
	}

	if len(marshalledData) == 0 {
		return [32]byte{}, fmt.Errorf("empty marshalled data")
	}

	return hashTreeRootFromBytes(info, marshalledData)
}

// hashTreeRootFromBytes switch/case per type to compute the hash tree root for the given SSZ data
// Core recursion
func hashTreeRootFromBytes(info *sszquery.SSZInfo, data []byte) ([32]byte, error) {
	// https://github.com/ethereum/consensus-specs/blob/dev/ssz/simple-serialize.md#typing
	switch info.Type() {
	case sszquery.UintN, sszquery.Byte, sszquery.Boolean:
		return computeBasicHashTreeRoot(info, data)
	case sszquery.Bitvector, sszquery.Bitlist:
		return computeBitHashTreeRoot(info, data)
	case sszquery.List:
		return computeListHashTreeRoot(info, data)
	case sszquery.Container:
		return computeContainerHashTreeRoot(info, data)
	case sszquery.Vector:
		return computeVectorHashTreeRoot(info, data)
	case sszquery.Union:
		return computeUnionHashTreeRoot(data)
	default:
		return [32]byte{}, fmt.Errorf("unsupported SSZ type: %s", info.Type())
	}
}

// computeBasicHashTreeRoot computes the hash tree root for basic types
// For basic types, pad to 32 bytes and return the chunk
func computeBasicHashTreeRoot(info *sszquery.SSZInfo, data []byte) ([32]byte, error) {
	var chunk [bytesPerChunk]byte
	copy(chunk[:], data[:info.FixedSize()])
	return chunk, nil
}

