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
func GetGeneralizedIndexFromPath(info *SszInfo, path Path) (uint64, error) {
	if info == nil {
		return 0, errors.New("SszInfo is nil")
	}

	// If path is empty, no generalized index can be computed.
	if len(path.Elements) == 0 {
		return 0, errors.New("cannot compute generalized index for an empty path")
	}

	// Starting from the root generalized index
	currentIndex := uint64(1)
	currentInfo := info

	for index, pathElement := range path.Elements {
		element := pathElement

		// Check that we are in a container to access fields
		if currentInfo.sszType != Container {
			return 0, fmt.Errorf("indexing requires a container field step first, got %s", currentInfo.sszType)
		}

		// Retrieve the field position and SSZInfo for the field in the current container
		fieldPos, fieldSsz, err := getContainerFieldByName(currentInfo, element.Name)
		if err != nil {
			return 0, fmt.Errorf("container field %s not found: %w", element.Name, err)
		}

		// Get the chunk count for the current container
		chunkCount, err := getChunkCount(currentInfo)
		if err != nil {
			return 0, fmt.Errorf("chunk count error: %w", err)
		}

		// Update the generalized index to point to the specified field
		currentIndex = currentIndex*nextPowerOfTwo(chunkCount) + fieldPos
		currentInfo = fieldSsz

		// Check for length access: element is the last in the path and requests length
		if path.Length && index == len(path.Elements)-1 {
			currentInfo, currentIndex, err = calculateLengthGeneralizedIndex(fieldSsz, element, currentIndex)
			if err != nil {
				return 0, fmt.Errorf("length calculation error: %w", err)
			}
			continue
		}

		if element.Index == nil {
			continue
		}

		switch fieldSsz.sszType {
		case List:
			currentInfo, currentIndex, err = calculateListGeneralizedIndex(fieldSsz, element, currentIndex)
			if err != nil {
				return 0, fmt.Errorf("list calculation error: %w", err)
			}

		case Vector:
			currentInfo, currentIndex, err = calculateVectorGeneralizedIndex(fieldSsz, element, currentIndex)
			if err != nil {
				return 0, fmt.Errorf("vector calculation error: %w", err)
			}

		case Bitlist:
			currentInfo, currentIndex, err = calculateBitlistGeneralizedIndex(fieldSsz, element, currentIndex)
			if err != nil {
				return 0, fmt.Errorf("bitlist calculation error: %w", err)
			}

		case Bitvector:
			currentInfo, currentIndex, err = calculateBitvectorGeneralizedIndex(fieldSsz, element, currentIndex)
			if err != nil {
				return 0, fmt.Errorf("bitvector calculation error: %w", err)
			}

		default:
			return 0, fmt.Errorf("indexing not supported for type %s", fieldSsz.sszType)
		}

	}

	return currentIndex, nil
}

// getContainerFieldByName finds a container field by its name
// and returns its index and SSZInfo.
func getContainerFieldByName(info *SszInfo, fieldName string) (uint64, *SszInfo, error) {
	containerInfo, err := info.ContainerInfo()
	if err != nil {
		return 0, nil, err
	}

	for index, name := range containerInfo.order {
		if name == fieldName {
			fieldInfo := containerInfo.fields[name]
			if fieldInfo == nil || fieldInfo.sszInfo == nil {
				return 0, nil, fmt.Errorf("field %s has no ssz info", name)
			}
			return uint64(index), fieldInfo.sszInfo, nil
		}
	}

	return 0, nil, fmt.Errorf("field %s not found", fieldName)
}

// Helpers for Generalized Index calculation per type

// calculateLengthGeneralizedIndex calculates the generalized index for a length field.
// note: length fields are only valid for List and Bitlist types. Multi-dimensional arrays are not supported.
// Returns:
// - its descendant SSZInfo (length field i.e. uint64)
// - its generalized index.
func calculateLengthGeneralizedIndex(fieldSsz *SszInfo, element PathElement, parentIndex uint64) (*SszInfo, uint64, error) {
	if element.Index != nil {
		return nil, 0, fmt.Errorf("len() is not supported for multi-dimensional arrays")
	}
	// Length field is only valid for List and Bitlist types
	if fieldSsz.sszType != List && fieldSsz.sszType != Bitlist {
		return nil, 0, fmt.Errorf("len() is only supported for List and Bitlist types, got %s", fieldSsz.sszType)
	}
	// Length is a uint64 per SSZ spec
	currentInfo := &SszInfo{sszType: Uint64}
	lengthIndex := parentIndex*2 + 1
	return currentInfo, lengthIndex, nil
}

// calculateListGeneralizedIndex calculates the generalized index for a list element.
// Returns:
// - its descendant SSZInfo (list element)
// - its generalized index.
func calculateListGeneralizedIndex(fieldSsz *SszInfo, element PathElement, parentIndex uint64) (*SszInfo, uint64, error) {
	li, err := fieldSsz.ListInfo()
	if err != nil {
		return nil, 0, fmt.Errorf("list info error: %w", err)
	}
	elem, err := li.Element()
	if err != nil {
		return nil, 0, fmt.Errorf("list element error: %w", err)
	}
	if *element.Index >= li.Limit() {
		return nil, 0, fmt.Errorf("index %d out of bounds for list with limit %d", *element.Index, li.Limit())
	}
	// Compute chunk position for the element
	var chunkPos uint64
	if elem.sszType.isBasic() {
		start := *element.Index * itemLength(elem)
		chunkPos = start / ssz.BytesPerChunk
	} else {
		chunkPos = *element.Index
	}
	innerChunkCount, err := getChunkCount(fieldSsz)
	if err != nil {
		return nil, 0, fmt.Errorf("chunk count error: %w", err)
	}
	// root = root * base_index * pow2ceil(chunk_count(container)) + fieldPos
	listIndex := parentIndex*listBaseIndex*nextPowerOfTwo(innerChunkCount) + chunkPos
	currentInfo := elem

	return currentInfo, listIndex, nil
}

// calculateVectorGeneralizedIndex calculates the generalized index for a vector element.
// Returns:
// - its descendant SSZInfo (vector element)
// - its generalized index.
func calculateVectorGeneralizedIndex(fieldSsz *SszInfo, element PathElement, parentIndex uint64) (*SszInfo, uint64, error) {
	vi, err := fieldSsz.VectorInfo()
	if err != nil {
		return nil, 0, fmt.Errorf("vector info error: %w", err)
	}
	elem, err := vi.Element()
	if err != nil {
		return nil, 0, fmt.Errorf("vector element error: %w", err)
	}
	if *element.Index >= vi.Length() {
		return nil, 0, fmt.Errorf("index %d out of bounds for vector with length %d", *element.Index, vi.Length())
	}
	var chunkPos uint64
	if elem.sszType.isBasic() {
		start := *element.Index * itemLength(elem)
		chunkPos = start / ssz.BytesPerChunk
	} else {
		chunkPos = *element.Index
	}
	innerChunkCount, err := getChunkCount(fieldSsz)
	if err != nil {
		return nil, 0, fmt.Errorf("chunk count error: %w", err)
	}
	vectorIndex := parentIndex*nextPowerOfTwo(innerChunkCount) + chunkPos

	currentInfo := elem
	return currentInfo, vectorIndex, nil
}

// calculateBitlistGeneralizedIndex calculates the generalized index for a bitlist element.
// Returns:
// - its descendant SSZInfo (bitlist element i.e. a boolean)
// - its generalized index.
func calculateBitlistGeneralizedIndex(fieldSsz *SszInfo, element PathElement, parentIndex uint64) (*SszInfo, uint64, error) {
	// Bits packed into 256-bit chunks; select the chunk containing the bit
	chunkPos := *element.Index / ssz.BitsPerChunk
	innerChunkCount, err := getChunkCount(fieldSsz)
	if err != nil {
		return nil, 0, fmt.Errorf("chunk count error: %w", err)
	}
	bitlistIndex := parentIndex*listBaseIndex*nextPowerOfTwo(innerChunkCount) + chunkPos

	// Bits element is not further descendable; set to basic to guard further steps
	currentInfo := &SszInfo{sszType: Boolean}
	return currentInfo, bitlistIndex, nil
}

// calculateBitvectorGeneralizedIndex calculates the generalized index for a bitvector element.
// Returns:
// - its descendant SSZInfo (bitvector element i.e. a boolean)
// - its generalized index.
func calculateBitvectorGeneralizedIndex(fieldSsz *SszInfo, element PathElement, parentIndex uint64) (*SszInfo, uint64, error) {
	chunkPos := *element.Index / ssz.BitsPerChunk
	innerChunkCount, err := getChunkCount(fieldSsz)
	if err != nil {
		return nil, 0, fmt.Errorf("chunk count error: %w", err)
	}
	bitvectorIndex := parentIndex*nextPowerOfTwo(innerChunkCount) + chunkPos

	// Bits element is not further descendable; set to basic to guard further steps
	currentInfo := &SszInfo{sszType: Boolean}
	return currentInfo, bitvectorIndex, nil
}

// Helper functions from SSZ spec

// itemLength calculates the byte length of an SSZ item based on its type information.
// For basic SSZ types (uint8, uint16, uint32, uint64, bool, etc.), it returns the actual
// size of the type in bytes. For compound types (containers, lists, vectors), it returns
// BytesPerChunk which represents the standard SSZ chunk size (32 bytes) used for
// Merkle tree operations in the SSZ serialization format.
func itemLength(info *SszInfo) uint64 {
	if info.sszType.isBasic() {
		return info.Size()
	}
	return ssz.BytesPerChunk
}

// nextPowerOfTwo computes the next power of two greater than or equal to v.
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
		return uint64(len(containerInfo.fields)), nil
	case List:
		listInfo, err := info.ListInfo()
		if err != nil {
			return 0, err
		}
		elementInfo, err := listInfo.Element()
		if err != nil {
			return 0, err
		}
		elemLength := itemLength(elementInfo)
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
		elemLength := itemLength(elementInfo)
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
