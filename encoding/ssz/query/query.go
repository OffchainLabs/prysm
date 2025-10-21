package query

import (
	"errors"
	"fmt"
)

// CalculateOffsetAndLength calculates the offset and length of a given path within the SSZ object.
// By walking the given path, it accumulates the offsets based on SszInfo.
func CalculateOffsetAndLength(sszInfo *SszInfo, path []PathElement) (*SszInfo, uint64, uint64, error) {
	if sszInfo == nil {
		return nil, 0, 0, errors.New("sszInfo is nil")
	}

	if len(path) == 0 {
		return nil, 0, 0, errors.New("path is empty")
	}

	walk := sszInfo
	offset := uint64(0)

	for pathIndex, elem := range path {
		containerInfo, err := walk.ContainerInfo()
		if err != nil {
			return nil, 0, 0, fmt.Errorf("could not get field infos: %w", err)
		}

		fieldInfo, exists := containerInfo.fields[elem.Name]
		if !exists {
			return nil, 0, 0, fmt.Errorf("field %s not found in containerInfo", elem.Name)
		}

		offset += fieldInfo.offset
		walk = fieldInfo.sszInfo

		// Check for accessing List/Vector elements by index
		if elem.Index != nil {
			switch walk.sszType {
			case List:
				index := *elem.Index
				listInfo := walk.listInfo
				if index >= listInfo.length {
					return nil, 0, 0, fmt.Errorf("index %d out of bounds for field %s with size %d", index, elem.Name, listInfo.length)
				}

				walk = listInfo.element
				if walk.isVariable {
					// Cumulative sum of sizes of previous elements to get the offset.
					for i := range index {
						offset += listInfo.elementSizes[i]
					}

					// NOTE: When populating recursively, the shared element template is updated for each
					// list item, causing it to retain the size information of the last processed element.
					// This wouldn't be an issue if this is in the middle of the path, as the walk would be updated
					// to the next field's sszInfo, which would have the correct size information.
					// However, if this is the last element in the path, we need to ensure we return the correct size
					// for the indexed element. Hence, we return the size from elementSizes.
					if pathIndex == len(path)-1 {
						return walk, offset, listInfo.elementSizes[index], nil
					}
				} else {
					offset += index * listInfo.element.Size()
				}

			case Vector:
				index := *elem.Index
				vectorInfo := walk.vectorInfo
				if index >= vectorInfo.length {
					return nil, 0, 0, fmt.Errorf("index %d out of bounds for field %s with size %d", index, elem.Name, vectorInfo.length)
				}

				offset += index * vectorInfo.element.Size()
				walk = vectorInfo.element

			default:
				return nil, 0, 0, fmt.Errorf("field %s of type %s does not support index access", elem.Name, walk.sszType)
			}
		}
	}

	return walk, offset, walk.Size(), nil
}
