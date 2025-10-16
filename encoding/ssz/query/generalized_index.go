package query

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	bytesPerChunk = 32
	bitsPerChunk  = 256
	listBaseIndex = 2
)

// GetGeneralizedIndexFromPath calculates the generalized index for a given path.
// To calculate the generalized index, two inputs are needed:
// 1. The path to the field (e.g., "FieldA.FieldB[3].FieldC") - snake case
// 2. The sszInfo of the root info (to know the structure of the data)
// It walks the path step by step, updating the generalized index at each step.
func GetGeneralizedIndexFromPath(info *sszInfo, path []PathElement) (uint64, error) {
	if info == nil {
		return 0, errors.New("sszInfo is nil")
	}

	// if element list is empty, no generalized index can be computed.
	if len(path) == 0 {
		return 0, errors.New("cannot compute generalized index for an empty path")
	}

	// Start from the root generalized index
	root := uint64(1)
	currentInfo := info

	for _, pathElement := range path {
		name := pathElement.Name

		// If we descend to a basic type, the path cannot continue further
		if isBasicType(currentInfo.sszType) {
			return 0, fmt.Errorf("cannot descend into basic type %s for path element %q", currentInfo.sszType, name)
		}

		// checks if a path element is a length field
		if isLengthField(name) {
			var err error
			root, currentInfo, err = processLengthField(currentInfo, name, root)
			if err != nil {
				return 0, err
			}
			continue
		}

		// case: field name (with optional array index)
		if currentInfo.sszType != Container {
			return 0, fmt.Errorf("indexing requires a container field step first, got %s", currentInfo.sszType)
		}

		// checks if a path element is an array index (e.g., field_name[5])
		fieldName := name
		var idx *uint64
		var err error
		if strings.Contains(name, "[") {
			// Split into field and index
			fieldName = extractFieldName(name)
			idx, err = extractArrayIndex(name)
			if err != nil {
				return 0, err
			}
		}

		// Retrieve the field position and SSZInfo for the field in the current container
		fieldPos, fieldSsz, err := getContainerFieldByName(currentInfo, fieldName)
		if err != nil {
			return 0, fmt.Errorf("container field %q not found: %w", fieldName, err)
		}

		// root = root * base_index(=1) * pow2ceil(chunk_count(container)) + fieldPos
		chunkCount, err := getChunkCount(currentInfo)
		if err != nil {
			return 0, fmt.Errorf("chunk count error: %w", err)
		}

		root = root*nextPowerOfTwo(chunkCount) + fieldPos
		currentInfo = fieldSsz

		if idx != nil {
			// index into list/vector/bitfield/bitvector
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
				// Compute chunk position for the element
				var chunkPos uint64
				if isBasicType(elem.sszType) {
					start := *idx * itemLengthFromInfo(elem)
					chunkPos = start / bytesPerChunk
				} else {
					chunkPos = *idx
				}
				// base_index = 2 for lists
				cc2, err := getChunkCount(fieldSsz)
				if err != nil {
					return 0, fmt.Errorf("chunk count error: %w", err)
				}
				root = root*listBaseIndex*nextPowerOfTwo(cc2) + chunkPos
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
				var (
					offset     uint64
					multiplier uint64
				)
				if isBasicType(elem.sszType) {
					multiplier = nextPowerOfTwo(vi.Length())
					offset = *idx
				} else { // cannot build a vector of complex elements
					cc2, err := getChunkCount(fieldSsz)
					if err != nil {
						return 0, fmt.Errorf("chunk count error: %w", err)
					}
					multiplier = nextPowerOfTwo(cc2)
					offset = *idx
				}
				root = root*multiplier + offset
				currentInfo = elem

			case Bitlist:
				// Bits packed into 256-bit chunks; select the chunk containing the bit
				chunkPos := *idx / bitsPerChunk
				cc2, err := getChunkCount(fieldSsz)
				if err != nil {
					return 0, fmt.Errorf("chunk count error: %w", err)
				}
				root = root*listBaseIndex*nextPowerOfTwo(cc2) + chunkPos
				// Bits element is not further descendable; set to basic to guard further steps
				currentInfo = &sszInfo{sszType: Boolean, fixedSize: 1}

			case Bitvector:
				chunkPos := *idx / bitsPerChunk
				cc2, err := getChunkCount(fieldSsz)
				if err != nil {
					return 0, fmt.Errorf("chunk count error: %w", err)
				}
				root = root*nextPowerOfTwo(cc2) + chunkPos
				currentInfo = &sszInfo{sszType: Boolean, fixedSize: 1}

			default:
				return 0, fmt.Errorf("indexing not supported for type %s", fieldSsz.sszType)
			}
			continue
		}
	}

	return root, nil
}

// isLengthField checks if a path element is a length field (len(...))
func isLengthField(name string) bool {
	return strings.HasPrefix(name, "len(") && strings.HasSuffix(name, ")")
}

// processLengthField processes a length field (len(...)) and returns the corresponding SSZInfo
// TODO: multi-dimensional arrays length?
func processLengthField(info *sszInfo, name string, root uint64) (uint64, *sszInfo, error) {
	// note: there cannot be empty spaces inside len(...) e.g. len( field )
	fieldName := strings.TrimSuffix(strings.TrimPrefix(name, "len("), ")")

	// In our case, the list and list length are two fields always wrapped in a container
	if info.sszType != Container {
		return 0, nil, fmt.Errorf("len() can only be applied for a list within a container field, got %s", info.sszType)
	}

	// Retrieve the field position and SSZInfo for the
	fieldPos, fieldSsz, err := getContainerFieldByName(info, fieldName)
	if err != nil {
		return 0, nil, fmt.Errorf("container field %q not found: %w", fieldName, err)
	}

	// Length field is only valid for List and Bitlist types
	if fieldSsz.sszType != List && fieldSsz.sszType != Bitlist {
		return 0, nil, fmt.Errorf("len() is only supported for List and Bitlist types, got %s", fieldSsz.sszType)
	}

	// root = root * base_index(=1) * pow2ceil(chunk_count(container)) + fieldPos
	chunkCount, err := getChunkCount(info)
	if err != nil {
		return 0, nil, fmt.Errorf("chunk count error: %w", err)
	}
	currentRoot := root*nextPowerOfTwo(chunkCount) + fieldPos

	// After len(), the type is uint64 (basic). If there are more path elements, reject.
	currentInfo := &sszInfo{sszType: UintN, fixedSize: 8}
	currentRoot = currentRoot*2 + 1

	return currentRoot, currentInfo, nil
}

// extractFieldName extracts the field name from a path element name (removes array indices)
// For example: "field_name[5]" returns "field_name"
func extractFieldName(name string) string {
	if idx := strings.Index(name, "["); idx != -1 {
		return name[:idx]
	}
	return strings.ToLower(name)
}

// extractArrayIndex extracts the array index from a path element name
// For example: "field_name[5]" returns 5
// TODO: there may be more than one index (e.g., multi-dimensional arrays)
func extractArrayIndex(name string) (*uint64, error) {
	start := strings.Index(name, "[")
	end := strings.Index(name, "]")

	if start == -1 || end == -1 || start >= end {
		return nil, errors.New("invalid array index format")
	}

	indexStr := name[start+1 : end]
	index, err := strconv.ParseUint(indexStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid array index: %w", err)
	}

	return &index, nil
}

// isBasicType checks if the SSZType is a basic type
func isBasicType(sszType SSZType) bool {
	switch sszType {
	case UintN, Byte, Boolean:
		return true
	default:
		return false
	}
}

// getChunkCount returns the number of chunks for the given SSZInfo (equivalent to chunk_count in the spec)
func getChunkCount(info *sszInfo) (uint64, error) {
	switch info.sszType {
	case UintN, Byte, Boolean:
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
		// For Lists with basic element types, multiple elements can be packed into 32-byte chunks
		elementInfo, err := listInfo.Element()
		if err != nil {
			return 0, err
		}
		elemLength := itemLengthFromInfo(elementInfo)
		return (listInfo.Limit()*uint64(elemLength) + 31) / bytesPerChunk, nil
	case Vector:
		vectorInfo, err := info.VectorInfo()
		if err != nil {
			return 0, err
		}
		// For Vectors with basic element types, multiple elements can be packed into 32-byte chunks
		elementInfo, err := vectorInfo.Element()
		if err != nil {
			return 0, err
		}
		elemLength := itemLengthFromInfo(elementInfo)
		return (vectorInfo.Length()*uint64(elemLength) + 31) / bytesPerChunk, nil
	case Bitlist:
		bitlistInfo, err := info.BitlistInfo()
		if err != nil {
			return 0, err
		}
		return (bitlistInfo.Limit() + 255) / bitsPerChunk, nil // Bits are packed into 256-bit chunks
	case Bitvector:
		vectorInfo, err := info.BitvectorInfo()
		if err != nil {
			return 0, err
		}
		return (vectorInfo.Length() + 255) / bitsPerChunk, nil // Bits are packed into 256-bit chunks
	default:
		return 0, errors.New("unsupported SSZ type for chunk count calculation")
	}
}

// getContainerFieldByName finds a container field by name.
func getContainerFieldByName(info *sszInfo, fieldName string) (uint64, *sszInfo, error) {
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
// bytesPerChunk which represents the standard SSZ chunk size (32 bytes) used for
// Merkle tree operations in the SSZ serialization format.
func itemLengthFromInfo(info *sszInfo) uint64 {
	if isBasicType(info.sszType) {
		return info.Size()
	}
	return bytesPerChunk
}

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
