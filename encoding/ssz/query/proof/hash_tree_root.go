package proof

import (
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
		return [32]byte{}, fmt.Errorf("unsupported SSZ type %s", info.Type())
	}
}

// computeBasicHashTreeRoot computes the hash tree root for basic types
// For basic types, pad to 32 bytes and return the chunk
func computeBasicHashTreeRoot(info *sszquery.SSZInfo, data []byte) ([32]byte, error) {
	var chunk [32]byte
	copy(chunk[:], data[:info.FixedSize()])
	return chunk, nil
}

// computeBitHashTreeRoot computes the hash tree root for bitvector/bitlist
func computeBitHashTreeRoot(_ *sszquery.SSZInfo, _ []byte) ([32]byte, error) {
	// NOTE:
	return [32]byte{}, fmt.Errorf("computeBitHashTreeRoot not implemented yet")
	// if info.Type() == sszquery.Bitvector {
	//     // 1. For bitvector: merkleize(pack_bits(value), limit=chunk_count(type))
	//     // pack_bits packs the bits into 32-byte chunks
	//     packed := packBits(data) // return the chunks of the byte slice
	//     lenChunks := uint64(len(packed))
	//     return merkleize(packed, &lenChunks), nil
	// } else if info.Type() == sszquery.Bitlist {
	//     // 2. For bitlist: mix_in_length(merkleize(pack_bits(value), limit=chunk_count(type)), len(value))
	//     prepacked, err := stripBitlistDelimiter(data) // remove the MSB delimiter bit first
	//     if err != nil {
	//         return [32]byte{}, fmt.Errorf("stripBitlistDelimiter: %w", err)
	//     }
	//     packed := packBits(prepacked) // return the chunks of the byte slice
	//     lenChunks := uint64(len(packed))
	//     root := merkleize(packed, &lenChunks)
	//     mixInLength(&root, uint64(len(data)*8))
	//     return root, nil
	// } else {
	//     return [32]byte{}, fmt.Errorf("computeBitHashTreeRoot called with non-bit type: %s", info.Type())
	// }
}

// computeListHashTreeRoot computes the hash tree root for lists
func computeListHashTreeRoot(info *sszquery.SSZInfo, data []byte) ([32]byte, error) {
	listInfo, err := info.ListInfo()
	if err != nil {
		return [32]byte{}, fmt.Errorf("ListInfo %w", err)
	}
	elementInfo, err := listInfo.Element()
	if err != nil {
		return [32]byte{}, fmt.Errorf("Element %w", err)
	}

	// 1. For list of basic objects: mix_in_length(merkleize(pack(value), limit=chunk_count(type)), len(value)) if value is a list of basic objects.
	if elementInfo.Type() == sszquery.Boolean || elementInfo.Type() == sszquery.Byte || elementInfo.Type() == sszquery.UintN {
		// Pack the data into 32-byte chunks
		packed, err := ssz.PackByChunk([][]byte{data})
		if err != nil {
			return [32]byte{}, fmt.Errorf("pack %w", err)
		}
		maxElems := listInfo.Limit()
		limitChunks := (maxElems*elementInfo.FixedSize() + 31) / 32
		body := ssz.MerkleizeVector(packed, limitChunks)
		var lenChunk [32]byte
		binary.LittleEndian.PutUint64(lenChunk[:8], listInfo.Length())
		return ssz.MixInLength(body, lenChunk[:]), nil
	} else {
		// 2) List of composite objects:
		// mix_in_length( merkleize([htr(elem) for elem in value], limit = type limit), len(value) )
		limit := listInfo.Limit()  // type-level capacity (elements)
		count := listInfo.Length() // runtime number of elements in this value

		elemSize := elementInfo.FixedSize()
		if !elementInfo.IsVariable() {
			expected := count * elemSize
			if uint64(len(data)) != expected {
				return [32]byte{}, fmt.Errorf(
					"list payload size %d != expected %d (count=%d * elemSize=%d)",
					len(data), expected, count, elemSize,
				)
			}
			roots := make([][32]byte, 0, count)
			for i := uint64(0); i < count; i++ {
				start := i * elemSize
				end := start + elemSize
				r, err := hashTreeRootFromBytes(elementInfo, data[start:end])
				if err != nil {
					return [32]byte{}, fmt.Errorf("element %d: %w", i, err)
				}
				roots = append(roots, r)
			}
			root := ssz.MerkleizeVector(roots, limit)
			var lenChunk [32]byte
			binary.LittleEndian.PutUint64(lenChunk[:8], count*8)
			return ssz.MixInLength(root, lenChunk[:]), nil
		}

		// ---- VARIABLE-SIZE COMPOSITES ----
		// We must iterate sequentially and compute each element's consumed length.
		roots := make([][32]byte, 0, count)
		p := uint64(0)
		for i := uint64(0); i < count; i++ {
			// NOTE::
			// if p >= uint64(len(data)) {
			// 	return [32]byte{}, fmt.Errorf("element %d: out of bounds (p=%d, len=%d)", i, p, len(data))
			// }
			// r, consumed, err := hashTreeRootFromBytes(elementInfo, data[p:])
			// if err != nil {
			// 	return [32]byte{}, fmt.Errorf("element %d: %w", i, err)
			// }
			// if consumed <= 0 || p+consumed > uint64(len(data)) {
			// 	return [32]byte{}, fmt.Errorf("element %d: invalid consumed size %d", i, consumed)
			// }
			// roots = append(roots, r)
			// p += consumed
		}
		if p != uint64(len(data)) {
			return [32]byte{}, fmt.Errorf("list trailing bytes consumed=%d, total=%d", p, len(data))
		}
		root := ssz.MerkleizeVector(roots, limit)
		var lenChunk [32]byte
		binary.LittleEndian.PutUint64(lenChunk[:8], count*8)
		return ssz.MixInLength(root, lenChunk[:]), nil
	}
}

// computeContainerHashTreeRoot computes the hash tree root for containers
func computeContainerHashTreeRoot(info *sszquery.SSZInfo, data []byte) ([32]byte, error) {
	// 1. For vector of composite objects or a container: merkleize([hash_tree_root(element) for element in value])
	if info.Type() != sszquery.Container {
		return [32]byte{}, fmt.Errorf("computeContainerHashTreeRoot called with non-container type: %s", info.Type())
	}

	containerInfo, err := info.ContainerInfo()
	if err != nil {
		return [32]byte{}, fmt.Errorf("ContainerInfo %w", err)
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
			endDelimiter := fieldInfo.Offset() + fieldSSZ.Size()
			if uint64(len(data)) < endDelimiter {
				return [32]byte{}, fmt.Errorf("data shorter than endDelimiter %s", fieldInfo.Name())
			}
			fieldData := data[fieldInfo.Offset():endDelimiter]
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
// merkleize(pack(value)) if value is a basic object or a vector of basic objects.
func computeVectorHashTreeRoot(info *sszquery.SSZInfo, data []byte) ([32]byte, error) {
	if info.Type() != sszquery.Vector {
		return [32]byte{}, fmt.Errorf("computeVectorHashTreeRoot called with non-vector type %s", info.Type())
	}

	vectorInfo, err := info.VectorInfo()
	if err != nil {
		return [32]byte{}, fmt.Errorf("VectorInfo %w", err)
	}

	elementInfo, err := vectorInfo.Element()
	if err != nil {
		return [32]byte{}, fmt.Errorf("Element %w", err)
	}

	// 1. For vector of basic objects: merkleize(pack(value))
	if elementInfo.Type() == sszquery.Boolean || elementInfo.Type() == sszquery.Byte || elementInfo.Type() == sszquery.UintN {
		// Pack the data into 32-byte chunks
		packed, err := ssz.PackByChunk([][]byte{data})
		if err != nil {
			return [32]byte{}, fmt.Errorf("PackByChunk %w", err)
		}
		maxElems := vectorInfo.Length()
		limitChunks := (maxElems*elementInfo.FixedSize() + 31) / 32
		return ssz.MerkleizeVector(packed, limitChunks), nil
	}

	// 2. For vector of composite objects or a container: merkleize([hash_tree_root(element) for element in value])
	if !elementInfo.IsVariable() {
		elemSize := elementInfo.FixedSize()
		count := vectorInfo.Length()
		expected := count * elemSize
		if uint64(len(data)) != expected {
			return [32]byte{}, fmt.Errorf(
				"vector payload size %d != expected %d (count=%d * elemSize=%d)",
				len(data), expected, count, elemSize,
			)
		}
		roots := make([][32]byte, 0, count)
		for i := uint64(0); i < count; i++ {
			start := i * elemSize
			end := start + elemSize
			r, err := hashTreeRootFromBytes(elementInfo, data[start:end])
			if err != nil {
				return [32]byte{}, fmt.Errorf("element %d: %w", i, err)
			}
			roots = append(roots, r)
		}
		return ssz.MerkleizeVector(roots, count), nil
	}

	chunks, err := ssz.PackByChunk([][]byte{data})
	if err != nil {
		return [32]byte{}, fmt.Errorf("PackByChunk %w", err)
	}

	return ssz.MerkleizeVector(chunks, uint64(len(chunks))), nil
}

// computeUnionHashTreeRoot computes the hash tree root for unions
// Placeholder
func computeUnionHashTreeRoot(_ []byte) ([32]byte, error) {
	// NOTE: handle union types
	return [32]byte{}, fmt.Errorf("computeUnionHashTreeRoot not implemented yet")
}
