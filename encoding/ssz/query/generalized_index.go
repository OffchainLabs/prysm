package query

import (
	"errors"
	"fmt"

	"github.com/OffchainLabs/prysm/v6/encoding/ssz"
)

const listBaseIndex = 2

// GetGeneralizedIndexFromPath calculates the generalized index for a given path.
// To calculate the generalized index, two inputs are needed:
// 1. The sszInfo of the root object, to be able to navigate the SSZ structure
// 2. The path to the field (e.g., "field_a.field_b[3].field_c")
// It walks the path step by step, updating the generalized index at each step.
func GetGeneralizedIndexFromPath(info *SszInfo, path []PathElement) (uint64, error) {
	if info == nil {
		return 0, errors.New("SszInfo is nil")
	}

	// If path is empty, no generalized index can be computed.
	if len(path) == 0 {
		return 0, errors.New("cannot compute generalized index for an empty path")
	}

	// Starting from the root generalized index
	root := uint64(1)
	currentInfo := info

	for _, pathElement := range path {
		element := pathElement

		// If we descend to a basic type, the path cannot continue further
		if isBasicType(currentInfo.sszType) {
			return 0, fmt.Errorf("cannot descend into basic type %s for path element %q", currentInfo.sszType, pathElement.Name)
		}

		// Check that we are in a container to access fields
		if currentInfo.sszType != Container {
			return 0, fmt.Errorf("indexing requires a container field step first, got %s", currentInfo.sszType)
		}

		// Retrieve the field position and SSZInfo for the field in the current container
		fieldPos, fieldSsz, err := getContainerFieldByName(currentInfo, element.Name)
		if err != nil {
			return 0, fmt.Errorf("container field %q not found: %w", element.Name, err)
		}

		// Get the chunk count for the current container
		chunkCount, err := getChunkCount(currentInfo)
		if err != nil {
			return 0, fmt.Errorf("chunk count error: %w", err)
		}

		// Update the generalized index to point to the specified field
		root = root*nextPowerOfTwo(chunkCount) + fieldPos
		currentInfo = fieldSsz

		// Check if a path element is a length field
		if element.Length {
			if element.Index != nil {
				return 0, fmt.Errorf("len() is not supported for indexed elements (multi-dimensional arrays)")
			}
			// Length field is only valid for List and Bitlist types
			if fieldSsz.sszType != List && fieldSsz.sszType != Bitlist {
				return 0, fmt.Errorf("len() is only supported for List and Bitlist types, got %s", fieldSsz.sszType)
			}
			// Length is a uint64 per SSZ spec
			currentInfo = &SszInfo{sszType: Uint64}
			root = root*2 + 1
			continue
		}

		if element.Index == nil {
			continue
		}

		switch fieldSsz.sszType {
		case List:
			li, err := fieldSsz.ListInfo()
			if err != nil {
				return 0, fmt.Errorf("list info error: %w", err)
			}
			elem, err := li.Element()
			if err != nil {
				return 0, fmt.Errorf("list element error: %w", err)
			}
			if *element.Index >= li.Limit() {
				return 0, fmt.Errorf("index %d out of bounds for list with limit %d", *element.Index, li.Limit())
			}
			// Compute chunk position for the element
			var chunkPos uint64
			if isBasicType(elem.sszType) {
				start := *element.Index * itemLengthFromInfo(elem)
				chunkPos = start / ssz.BytesPerChunk
			} else {
				chunkPos = *element.Index
			}
			innerChunkCount, err := getChunkCount(fieldSsz)
			if err != nil {
				return 0, fmt.Errorf("chunk count error: %w", err)
			}
			// root = root * base_index * pow2ceil(chunk_count(container)) + fieldPos
			root = root*listBaseIndex*nextPowerOfTwo(innerChunkCount) + chunkPos
			currentInfo = elem

		case Vector:
			vi, err := fieldSsz.VectorInfo()
			if err != nil {
				return 0, fmt.Errorf("vector info error: %w", err)
			}
			elem, err := vi.Element()
			if err != nil {
				return 0, fmt.Errorf("vector element error: %w", err)
			}
			if *element.Index >= vi.Length() {
				return 0, fmt.Errorf("index %d out of bounds for vector with length %d", *element.Index, vi.Length())
			}
			var chunkPos uint64
			if isBasicType(elem.sszType) {
				start := *element.Index * itemLengthFromInfo(elem)
				chunkPos = start / ssz.BytesPerChunk
			} else {
				chunkPos = *element.Index
			}
			innerChunkCount, err := getChunkCount(fieldSsz)
			if err != nil {
				return 0, fmt.Errorf("chunk count error: %w", err)
			}
			root = root*nextPowerOfTwo(innerChunkCount) + chunkPos

			currentInfo = elem

		case Bitlist:
			// Bits packed into 256-bit chunks; select the chunk containing the bit
			chunkPos := *element.Index / ssz.BitsPerChunk
			innerChunkCount, err := getChunkCount(fieldSsz)
			if err != nil {
				return 0, fmt.Errorf("chunk count error: %w", err)
			}
			root = root*listBaseIndex*nextPowerOfTwo(innerChunkCount) + chunkPos

			// Bits element is not further descendable; set to basic to guard further steps
			currentInfo = &SszInfo{sszType: Boolean}

		case Bitvector:
			chunkPos := *element.Index / ssz.BitsPerChunk
			innerChunkCount, err := getChunkCount(fieldSsz)
			if err != nil {
				return 0, fmt.Errorf("chunk count error: %w", err)
			}
			root = root*nextPowerOfTwo(innerChunkCount) + chunkPos

			// Bits element is not further descendable; set to basic to guard further steps
			currentInfo = &SszInfo{sszType: Boolean}

		default:
			return 0, fmt.Errorf("indexing not supported for type %s", fieldSsz.sszType)
		}

	}

	return root, nil
}

// isBasicType checks if the SSZType is a basic type
func isBasicType(sszType SSZType) bool {
	switch sszType {
	case Uint8, Uint16, Uint32, Uint64, Boolean:
		return true
	default:
		return false
	}
}

// getChunkCount returns the number of chunks for the given SSZInfo (equivalent to chunk_count in the spec)
func getChunkCount(info *SszInfo) (uint64, error) {
	switch info.sszType {
	case Uint8, Uint16, Uint32, Uint64, Boolean:
		return 1, nil
	case Container:
		containerInfo, err := info.ContainerInfo()
		if err != nil {
			return 0, err
		}
		return uint64(len(containerInfo.order)), nil
	case List:
		listInfo, err := info.ListInfo()
		if err != nil {
			return 0, err
		}
		elementInfo, err := listInfo.Element()
		if err != nil {
			return 0, err
		}
		elemLength := itemLengthFromInfo(elementInfo)
		return (listInfo.Limit()*elemLength + 31) / ssz.BytesPerChunk, nil
	case Vector:
		vectorInfo, err := info.VectorInfo()
		if err != nil {
			return 0, err
		}
		elementInfo, err := vectorInfo.Element()
		if err != nil {
			return 0, err
		}
		elemLength := itemLengthFromInfo(elementInfo)
		return (vectorInfo.Length()*elemLength + 31) / ssz.BytesPerChunk, nil
	case Bitlist:
		bitlistInfo, err := info.BitlistInfo()
		if err != nil {
			return 0, err
		}
		return (bitlistInfo.Limit() + 255) / ssz.BitsPerChunk, nil // Bits are packed into 256-bit chunks
	case Bitvector:
		bitvectorInfo, err := info.BitvectorInfo()
		if err != nil {
			return 0, err
		}
		return (bitvectorInfo.Length() + 255) / ssz.BitsPerChunk, nil // Bits are packed into 256-bit chunks
	default:
		return 0, errors.New("unsupported SSZ type for chunk count calculation")
	}
}

// getContainerFieldByName finds a container field by name.
func getContainerFieldByName(info *SszInfo, fieldName string) (uint64, *SszInfo, error) {
	containerInfo, err := info.ContainerInfo()
	if err != nil {
		return 0, nil, err
	}

	for index, name := range containerInfo.order {
		if name == fieldName {
			fieldInfo := containerInfo.fields[name]
			if fieldInfo == nil || fieldInfo.sszInfo == nil {
				return 0, nil, fmt.Errorf("field %q has no ssz info", name)
			}
			return uint64(index), fieldInfo.sszInfo, nil
		}
	}

	return 0, nil, fmt.Errorf("field %q not found", fieldName)
}

// itemLengthFromInfo calculates the byte length of an SSZ item based on its type information.
// For basic SSZ types (uint8, uint16, uint32, uint64, bool, etc.), it returns the actual
// size of the type in bytes. For complex types (containers, lists, vectors), it returns
// BytesPerChunk which represents the standard SSZ chunk size (32 bytes) used for
// Merkle tree operations in the SSZ serialization format.
func itemLengthFromInfo(info *SszInfo) uint64 {
	if isBasicType(info.sszType) {
		return info.Size()
	}
	return ssz.BytesPerChunk
}

// Helpers for input processing

// Copied from fastssz
// Modified to return uint64
func nextPowerOfTwo(v uint64) uint64 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return uint64(v)
}
