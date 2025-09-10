package proof

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	ssz "github.com/OffchainLabs/prysm/v6/encoding/ssz"
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

// computeContainerHashTreeRoot computes the hash tree root for containers
func computeContainerHashTreeRoot(info *sszquery.SSZInfo, data []byte) ([32]byte, error) {
	// 1. For vector of composite objects or a container: merkleize([hash_tree_root(element) for element in value])
	if info.Type() != sszquery.Container {
		return [32]byte{}, fmt.Errorf("computeContainerHashTreeRoot called with non-container type: %s", info.Type())
	}

	containerInfo, err := info.ContainerInfo()
	if err != nil {
		return [32]byte{}, fmt.Errorf("ContainerInfo: %w", err)
	}

	var elementRoots [][32]byte

	// Ordered fields
	ci := containerInfo.Fields()
	var orderedFields []*sszquery.FieldInfo
	for _, name := range containerInfo.Order() {
		orderedFields = append(orderedFields, ci[name])
	}

	for _, fieldInfo := range orderedFields {
		fieldSSZ := fieldInfo.SSZ()
		fieldSize := fieldSSZ.FixedSize()
		if fieldSSZ.IsVariable() {
			// For variable-sized fields, we need to read the offset from the fixed part
			if len(data) < int(fieldInfo.Offset()+4) {
				return [32]byte{}, fmt.Errorf("data too short to read offset for field %s", fieldInfo.Name())
			}

			// Read the offset (4 bytes little-endian)
			fieldOffset := binary.LittleEndian.Uint32(data[fieldInfo.Offset() : fieldInfo.Offset()+4])

			// Extract the variable data starting from the offset
			if uint64(fieldOffset) >= uint64(len(data)) {
				return [32]byte{}, fmt.Errorf("offset %d exceeds data length %d for field %s", fieldOffset, len(data), fieldInfo.Name())
			}
			fieldData := data[fieldOffset:] // TODO: Am I missing the final delimiter? data[fieldOffset:fieldOffset+FIELD_LENGTH]

			fieldRoot, err := hashTreeRootFromBytes(fieldSSZ, fieldData)
			if err != nil {
				return [32]byte{}, fmt.Errorf("hashTreeRootFromBytes for field %s: %w", fieldInfo.Name(), err)
			}
			elementRoots = append(elementRoots, fieldRoot)
		} else {
			// For fixed-sized fields, extract directly using offset and size
			if len(data) < int(fieldInfo.Offset()+fieldSize) {
				return [32]byte{}, fmt.Errorf("data too short for fixed field %s", fieldInfo.Name())
			}
			fieldData := data[fieldInfo.Offset() : fieldInfo.Offset()+fieldSize]

			fieldRoot, err := hashTreeRootFromBytes(fieldSSZ, fieldData)
			if err != nil {
				return [32]byte{}, fmt.Errorf("hashTreeRootFromBytes for field %s: %w", fieldInfo.Name(), err)
			}
			elementRoots = append(elementRoots, fieldRoot)
		}
	}
	return ssz.MerkleizeVector(elementRoots, uint64(len(elementRoots))), nil
}

// computeVectorHashTreeRoot computes the hash tree root for vectors
func computeVectorHashTreeRoot(info *sszquery.SSZInfo, data []byte) ([32]byte, error) {
	if info.Type() != sszquery.Vector {
		return [32]byte{}, fmt.Errorf("computeVectorHashTreeRoot called with non-vector type: %s", info.Type())
	}

	return computeBasicHashTreeRoot(info, data)
}


	}

	return merkleize(elementRoots, nil), nil

}
