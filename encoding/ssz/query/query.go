package query

import (
	"errors"
	"fmt"
)

func CalculateOffsetAndLength(sszInfo *sszInfo, path []PathElement) (*sszInfo, uint64, uint64, error) {
	if sszInfo == nil {
		return nil, 0, 0, errors.New("sszInfo is nil")
	}

	if len(path) == 0 {
		return nil, 0, 0, errors.New("path is empty")
	}

	walk := sszInfo
	currentOffset := uint64(0)

	for _, elem := range path {
		containerInfo, err := walk.ContainerInfo()
		if err != nil {
			return nil, 0, 0, fmt.Errorf("could not get field infos: %w", err)
		}

		fieldInfo, exists := containerInfo.fields[elem.Name]
		if !exists {
			return nil, 0, 0, fmt.Errorf("field %s not found in containerInfo", elem.Name)
		}

		currentOffset += fieldInfo.offset
		walk = fieldInfo.sszInfo
	}

	if walk.isVariable {
		return nil, 0, 0, errors.New("cannot calculate length for variable-sized type")
	}

	return walk, currentOffset, walk.Size(), nil
}
