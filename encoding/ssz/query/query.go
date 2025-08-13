package query

import "fmt"

func CalculateOffsetAndLength(sszInfo *sszInfo, path []PathElement) (*sszInfo, uint64, uint64, error) {
	if sszInfo == nil {
		return nil, 0, 0, fmt.Errorf("sszInfo is nil")
	}

	if len(path) == 0 {
		return nil, 0, 0, fmt.Errorf("path is empty")
	}

	walk := sszInfo
	currentOffset := uint64(0)

	// TODO: This path finding logic assumes access to the field in SSZ container types.
	for _, elem := range path {
		fieldInfos, err := walk.ContainerInfo()
		if err != nil {
			return nil, 0, 0, fmt.Errorf("get field infos: %w", err)
		}

		fieldInfo, exists := fieldInfos[elem.Name]
		if !exists {
			return nil, 0, 0, fmt.Errorf("field %s not found in fieldInfos", elem.Name)
		}

		currentOffset += fieldInfo.offset
		walk = fieldInfo.sszInfo
	}

	// TODO: Handle variable-sized types
	if walk.isVariable {
		return nil, 0, 0, fmt.Errorf("cannot calculate length for variable-sized type")
	}

	return walk, currentOffset, walk.Size(), nil
}
