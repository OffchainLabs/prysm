package query

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	offsetBytes = 4

	// sszMaxTag specifies the maximum capacity of a variable-sized collection, like an SSZ List.
	sszMaxTag = "ssz-max"

	// sszSizeTag specifies the length of a fixed-sized collection, like an SSZ Vector.
	// A wildcard ('?') indicates that the dimension is variable-sized (a List).
	sszSizeTag = "ssz-size"
)

// AnalyzeObject analyzes given object and returns its SSZ information.
func AnalyzeObject(obj any) (*sszInfo, error) {
	value := dereferencePointer(obj)

	info, err := analyzeType(value.Type(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not analyze type %s: %w", value.Type().Name(), err)
	}

	return info, nil
}

// analyzeType is an entry point that inspects a reflect.Type and computes its SSZ layout information.
func analyzeType(typ reflect.Type, tag *reflect.StructTag) (*sszInfo, error) {
	switch typ.Kind() {
	// Basic types (e.g., uintN where N is 8, 16, 32, 64)
	// NOTE: uint128 and uint256 are represented as []byte in Go,
	// so we handle them as slices. See `analyzeHomogeneousColType`.
	case reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Bool:
		return analyzeBasicType(typ)

	case reflect.Slice:
		return analyzeHomogeneousColType(typ, tag)

	case reflect.Struct:
		return analyzeContainerType(typ)

	case reflect.Ptr:
		// Dereference pointer types.
		return analyzeType(typ.Elem(), tag)

	default:
		return nil, fmt.Errorf("unsupported type %v for SSZ calculation", typ.Kind())
	}
}

// analyzeBasicType analyzes SSZ basic types (uintN, bool) and returns its info.
func analyzeBasicType(typ reflect.Type) (*sszInfo, error) {
	sszInfo := &sszInfo{
		typ: typ,

		// Every basic type is fixed-size and not variable.
		isVariable: false,
	}

	switch typ.Kind() {
	case reflect.Uint64:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 8
	case reflect.Uint32:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 4
	case reflect.Uint16:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 2
	case reflect.Uint8:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 1
	case reflect.Bool:
		sszInfo.sszType = Boolean
		sszInfo.fixedSize = 1
	default:
		return nil, fmt.Errorf("unsupported basic type %v for SSZ calculation", typ.Kind())
	}

	return sszInfo, nil
}

// analyzeHomogeneousColType analyzes homogeneous collection types (e.g., List, Vector, Bitlist, Bitvector) and returns its SSZ info.
func analyzeHomogeneousColType(typ reflect.Type, tag *reflect.StructTag) (*sszInfo, error) {
	if typ.Kind() != reflect.Slice {
		return nil, fmt.Errorf("can only analyze slice types, got %v", typ.Kind())
	}

	if tag == nil {
		return nil, fmt.Errorf("tag is required for slice types")
	}

	elementInfo, err := analyzeType(typ.Elem(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not analyze element type for homogeneous collection: %w", err)
	}

	// 1. Check if the type is List/Bitlist by checking `ssz-max` tag.
	sszMax := tag.Get(sszMaxTag)
	if sszMax != "" {
		dims := strings.Split(sszMax, ",")
		if len(dims) > 1 {
			return nil, fmt.Errorf("multi-dimensional lists are not supported, got %d dimensions", len(dims))
		}

		limit, err := strconv.ParseUint(dims[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid ssz-max tag (%s): %w", sszMax, err)
		}

		return analyzeListType(typ, elementInfo, limit)
	}

	// 2. Handle Vector/Bitvector type.
	sszSize := tag.Get(sszSizeTag)
	dims := strings.Split(sszSize, ",")
	if len(dims) > 1 {
		return nil, fmt.Errorf("multi-dimensional vectors are not supported, got %d dimensions", len(dims))
	}

	length, err := strconv.ParseUint(dims[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ssz-size tag (%s): %w", sszSize, err)
	}

	return analyzeVectorType(typ, elementInfo, length)
}

// analyzeListType analyzes SSZ List type and returns its SSZ info.
func analyzeListType(typ reflect.Type, elementInfo *sszInfo, limit uint64) (*sszInfo, error) {
	if elementInfo == nil {
		return nil, fmt.Errorf("element info is required for List")
	}

	return &sszInfo{
		sszType: List,
		typ:     typ,

		fixedSize:  offsetBytes,
		isVariable: true,
	}, nil
}

// analyzeVectorType analyzes SSZ Vector type and returns its SSZ info.
func analyzeVectorType(typ reflect.Type, elementInfo *sszInfo, length uint64) (*sszInfo, error) {
	if elementInfo == nil {
		return nil, fmt.Errorf("element info is required for Vector")
	}

	return &sszInfo{
		sszType: Vector,
		typ:     typ,

		fixedSize:  length * elementInfo.Size(),
		isVariable: false,
	}, nil
}

// analyzeContainerType analyzes SSZ Container type and returns its SSZ info.
func analyzeContainerType(typ reflect.Type) (*sszInfo, error) {
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("can only analyze struct types, got %v", typ.Kind())
	}

	sszInfo := &sszInfo{
		sszType: Container,
		typ:     typ,

		containerInfo: make(map[string]*fieldInfo),
	}
	var currentOffset uint64

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Protobuf-generated structs contain private fields we must skip.
		// e.g., state, sizeCache, unknownFields, etc.
		if !field.IsExported() {
			continue
		}

		// The JSON tag contains the field name in the first part.
		// e.g., "attesting_indices,omitempty" -> "attesting_indices".
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" {
			return nil, fmt.Errorf("field %s has no JSON tag", field.Name)
		}

		// NOTE: `fieldName` is a string with `snake_case` format (following consensus specs).
		fieldName := strings.Split(jsonTag, ",")[0]
		if fieldName == "" {
			return nil, fmt.Errorf("field %s has an empty JSON tag", field.Name)
		}

		// Analyze each field so that we can complete full SSZ information.
		info, err := analyzeType(field.Type, &field.Tag)
		if err != nil {
			return nil, fmt.Errorf("could not analyze type for field %s: %w", fieldName, err)
		}

		// If one of the fields is variable-sized,
		// the entire struct is considered variable-sized.
		if info.isVariable {
			sszInfo.isVariable = true
		}

		// Store nested struct info.
		sszInfo.containerInfo[fieldName] = &fieldInfo{
			sszInfo: info,
			offset:  currentOffset,
		}

		// Update the current offset based on the field's fixed size.
		currentOffset += info.fixedSize
	}

	sszInfo.fixedSize = currentOffset

	return sszInfo, nil
}

// dereferencePointer dereferences a pointer to get the underlying value using reflection.
func dereferencePointer(obj any) reflect.Value {
	value := reflect.ValueOf(obj)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			// If we encounter a nil pointer before the end of the path, we can still proceed
			// by analyzing the type, not the value.
			value = reflect.New(value.Type().Elem()).Elem()
		} else {
			value = value.Elem()
		}
	}

	return value
}
