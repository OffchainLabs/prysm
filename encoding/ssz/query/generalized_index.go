package query

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	bytesPerChunk = 32
	bitsPerChunk  = 256
	listBaseIndex = 2
)

type Element struct {
	length  bool
	name    string
	indices *[]uint64
}

// GetGeneralizedIndexFromPath calculates the generalized index for a given path.
// To calculate the generalized index, two inputs are needed:
// 1. The sszInfo of the root info, to be able to navigate the SSZ structure
// 2. The path to the field (e.g., "field_a.field_b[3].field_c")
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
		element, err := processPathElement(name)
		if err != nil {
			return 0, err
		}

		// If we descend to a basic type, the path cannot continue further
		if isBasicType(currentInfo.sszType) {
			return 0, fmt.Errorf("cannot descend into basic type %s for path element %q", currentInfo.sszType, name)
		}

		// case: field name within a container - optionally with array indices
		if currentInfo.sszType != Container {
			return 0, fmt.Errorf("indexing requires a container field step first, got %s", currentInfo.sszType)
		}

		// checks if a path element is a length field
		if element.length {
			// Use the parsed inner name (without the len(...)) to resolve the field
			root, currentInfo, err = processLengthField(currentInfo, element.name, root)
			if err != nil {
				return 0, err
			}
			continue
		}

		// checks if a path element is an array index (e.g., field_name[5])
		var idx *uint64
		if element.indices != nil && len(*element.indices) > 0 {
			// TODO: just a shortcut for now; extend to multi-dimensional arrays later
			idx = &(*element.indices)[0]
		}

		// Retrieve the field position and SSZInfo for the field in the current container
		fieldPos, fieldSsz, err := getContainerFieldByName(currentInfo, element.name)
		if err != nil {
			return 0, fmt.Errorf("container field %q not found: %w", element.name, err)
		}

		chunkCount, err := getChunkCount(currentInfo)
		if err != nil {
			return 0, fmt.Errorf("chunk count error: %w", err)
		}

		root = updateRoot(root, 1, chunkCount, fieldPos)
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
				innerChunkCount, err := getChunkCount(fieldSsz)
				if err != nil {
					return 0, fmt.Errorf("chunk count error: %w", err)
				}
				root = updateRoot(root, listBaseIndex, innerChunkCount, chunkPos)
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
					innerChunkCount, err := getChunkCount(fieldSsz)
					if err != nil {
						return 0, fmt.Errorf("chunk count error: %w", err)
					}
					multiplier = nextPowerOfTwo(innerChunkCount)
					offset = *idx
				}
				root = root*multiplier + offset
				currentInfo = elem

			case Bitlist:
				// Bits packed into 256-bit chunks; select the chunk containing the bit
				chunkPos := *idx / bitsPerChunk
				innerChunkCount, err := getChunkCount(fieldSsz)
				if err != nil {
					return 0, fmt.Errorf("chunk count error: %w", err)
				}
				root = updateRoot(root, listBaseIndex, innerChunkCount, chunkPos)
				// Bits element is not further descendable; set to basic to guard further steps
				currentInfo = &sszInfo{sszType: Boolean, fixedSize: 1}

			case Bitvector:
				chunkPos := *idx / bitsPerChunk
				innerChunkCount, err := getChunkCount(fieldSsz)
				if err != nil {
					return 0, fmt.Errorf("chunk count error: %w", err)
				}
				root = updateRoot(root, 1, innerChunkCount, chunkPos)
				currentInfo = &sszInfo{sszType: Boolean, fixedSize: 1}

			default:
				return 0, fmt.Errorf("indexing not supported for type %s", fieldSsz.sszType)
			}
			continue
		}
	}

	return root, nil
}

// processLengthField processes a length field (len(...)) and returns the corresponding SSZInfo
// TODO: multi-dimensional arrays length?
func processLengthField(info *sszInfo, name string, root uint64) (uint64, *sszInfo, error) {
	// Retrieve the field position and SSZInfo for the
	fieldPos, fieldSsz, err := getContainerFieldByName(info, name)
	if err != nil {
		return 0, nil, fmt.Errorf("container field %q not found: %w", name, err)
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
	currentRoot := updateRoot(root, 1, chunkCount, fieldPos)

	// After len(), the type is uint64 (basic). If there are more path elements, reject.
	currentInfo := &sszInfo{sszType: UintN, fixedSize: 8}
	currentRoot = updateRoot(currentRoot, 1, 2, 1)

	return currentRoot, currentInfo, nil
}

// updateRoot computes the new generalized index based on the current root, base index, chunk count, and offset
// base index is typically 1 for containers and 2 for lists
// root = root * base_index * pow2ceil(chunk_count(container)) + fieldPos
func updateRoot(root uint64, baseIndex uint64, chunkCount uint64, offset uint64) uint64 {
	return root*baseIndex*nextPowerOfTwo(chunkCount) + offset
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

// Helpers for input processing

// processPathElement processes a path element string and returns an Element struct
func processPathElement(elementStr string) (Element, error) {
	element := Element{}

	// Processing element string
	processingField := elementStr

	re := regexp.MustCompile(`^\s*len\s*\(\s*([^)]+?)\s*\)\s*$`)
	matches := re.FindStringSubmatch(processingField)
	if len(matches) == 2 {
		element.length = true
		// Extract the inner expression between len( and ) and continue parsing on that
		processingField = matches[1]
	}

	// Default name is the full working string (may be updated below if it contains indices)
	element.name = processingField

	if strings.Contains(processingField, "[") {
		// Split into field and indices, e.g., "array[0][1]" -> name:"array", indices:{0,1}
		element.name = extractFieldName(processingField)
		indices, err := extractArrayIndices(processingField)
		if err != nil {
			return Element{}, err
		}
		element.indices = &indices
	}

	return element, nil
}

// extractFieldName extracts the field name from a path element name (removes array indices)
// For example: "field_name[5]" returns "field_name"
func extractFieldName(name string) string {
	if idx := strings.Index(name, "["); idx != -1 {
		return name[:idx]
	}
	return strings.ToLower(name)
}

// extractArrayIndices returns every bracketed, non-negative index in the name,
// e.g. "array[0][1]" -> []uint64{0, 1}. Errors if none are found or if any index is invalid.
func extractArrayIndices(name string) ([]uint64, error) {
	// Match all bracketed content, then we'll parse as unsigned to catch negatives explicitly
	re := regexp.MustCompile(`\[\s*([^\]]+)\s*\]`)
	matches := re.FindAllStringSubmatch(name, -1)

	if len(matches) == 0 {
		return nil, errors.New("no array indices found")
	}

	indices := make([]uint64, 0, len(matches))
	for _, m := range matches {
		raw := strings.TrimSpace(m[1])
		// Forbid signs explicitly; we want a clear error similar to ParseUint's message
		if strings.HasPrefix(raw, "-") {
			return nil, fmt.Errorf("cannot process negative indices %q", raw)
		}
		idx, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid array index: %w", err)
		}
		indices = append(indices, idx)
	}
	return indices, nil
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
