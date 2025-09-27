package query

import (
	"errors"
	"fmt"
)

// CalculateOffsetAndLength calculates the offset and length of a given path within the SSZ object.
// By walking the given path, it accumulates the offsets based on sszInfo.
func CalculateOffsetAndLength(sszInfo *sszInfo, path []PathElement) (*sszInfo, uint64, uint64, error) {
	if sszInfo == nil {
		return nil, 0, 0, errors.New("sszInfo is nil")
	}

	if len(path) == 0 {
		return nil, 0, 0, errors.New("path is empty")
	}

	walk := sszInfo
	offset := uint64(0)

	for _, elem := range path {
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
					return nil, 0, 0, fmt.Errorf("index %d out of bounds for field %s", index, elem.Name)
				}

				walk = listInfo.element
				if walk.isVariable {
					// 1. Cumulative sum of sizes of previous elements to get the offset.
					for i := range index {
						offset += listInfo.elementSizes[i]
					}

					// 2. Set retroactively the length of sszInfo of walk.
					size := listInfo.elementSizes[index]
					err := walk.SetLengthBySize(size)
					if err != nil {
						return nil, 0, 0, fmt.Errorf("could not set length by size for field %s: %w", elem.Name, err)
					}
				} else {
					offset += index * listInfo.element.Size()
				}

			case Vector:
				index := *elem.Index
				vectorInfo := walk.vectorInfo
				if index >= vectorInfo.length {
					return nil, 0, 0, fmt.Errorf("index %d out of bounds for field %s", index, elem.Name)
				}

				offset += index * vectorInfo.element.Size()
				walk = vectorInfo.element

			case Bitlist:
			case Bitvector:
			default:
				return nil, 0, 0, fmt.Errorf("field %s is not a List/Bitvector, cannot index", elem.Name)
			}
		}
	}

	return walk, offset, walk.Size(), nil
}
