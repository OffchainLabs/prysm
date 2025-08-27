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
	actualOffset, currentOffset := uint64(0), uint64(0)

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
		actualOffset = fieldInfo.actualOffset
		walk = fieldInfo.sszInfo
	}

	// Determine the final offset based on fixed-size/variable-size.
	offset := currentOffset
	if walk.isVariable {
		offset = actualOffset
	}

	return walk, offset, walk.Size(), nil
}
