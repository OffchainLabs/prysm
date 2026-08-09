package query

import (
	"errors"
	"fmt"
)

// containerInfo has
// 1. fields: a field map that maps a field's JSON name to its SszInfo for nested Containers
// 2. order: a list of field names in the order they should be serialized
// 3. fixedOffset: the total size of the fixed part of the container
type containerInfo struct {
	fields      map[string]*fieldInfo
	order       []string
	fixedOffset uint64
}

// FieldInfo returns the SszInfo of the specified field in the container.
func (ci *containerInfo) FieldInfo(fieldName string) (*SszInfo, error) {
	if ci == nil {
		return nil, errors.New("containerInfo is nil")
	}

	fields := ci.fields
	if fields == nil {
		return nil, errors.New("container has no fields")
	}

	field, ok := fields[fieldName]
	if !ok {
		return nil, fmt.Errorf("field %q not found in container", fieldName)
	}

	sszInfo := field.sszInfo
	if sszInfo == nil {
		return nil, fmt.Errorf("field %q has no SSZ info", fieldName)
	}

	return sszInfo, nil
}

// FieldPosition returns the position (index) of the specified field name in the container's
// serialization order. This position corresponds to the field's index in the SSZ container.
func (ci *containerInfo) FieldPosition(fieldName string) (int, bool) {
	if ci == nil {
		return -1, false
	}
	for i, name := range ci.order {
		if name == fieldName {
			return i, true
		}
	}
	return -1, false
}

type fieldInfo struct {
	// sszInfo contains the SSZ information of the field.
	sszInfo *SszInfo
	// offset is the offset of the field within the parent struct.
	offset uint64
	// goFieldName is the name of the field in Go struct.
	goFieldName string
}
