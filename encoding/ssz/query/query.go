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
	}

	return walk, offset, walk.Size(), nil
}
