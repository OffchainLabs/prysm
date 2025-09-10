package query

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

const offsetBytes = 4

// AnalyzeObject analyzes given object and returns its SSZ information.
func AnalyzeObject(obj any) (*sszInfo, error) {
	value := dereferencePointer(obj)

	info, err := analyzeType(value.Type(), nil)
	if err != nil {
		return nil, fmt.Errorf("could not analyze type %s: %w", value.Type().Name(), err)
	}

	return info, nil
}

// PopulateVariableLengthInfo populates runtime information for SSZ fields of variable-sized types.
// This function updates the sszInfo structure with actual lengths and offsets that can only
// be determined at runtime for variable-sized items like Lists and variable-sized Container fields.
func PopulateVariableLengthInfo(sszInfo *sszInfo, value any) error {
	if sszInfo == nil {
		return errors.New("sszInfo is nil")
	}

	if value == nil {
		return errors.New("value is nil")
	}

	// Short circuit: If the type is fixed-sized, we don't need to fill in the info.
	if !sszInfo.isVariable {
		return nil
	}

	switch sszInfo.sszType {
	// In List case, we have to set the actual length of the list.
	case List:
		listInfo, err := sszInfo.ListInfo()
		if err != nil {
			return fmt.Errorf("could not get list info: %w", err)
		}

		if listInfo == nil {
			return errors.New("listInfo is nil")
		}

		val := reflect.ValueOf(value)
		if val.Kind() != reflect.Slice {
			return fmt.Errorf("expected slice for List type, got %v", val.Kind())
		}

		length := uint64(val.Len())
		if err := listInfo.SetLength(length); err != nil {
			return fmt.Errorf("could not set list length: %w", err)
		}

		return nil
	// In Container case, we need to recursively populate variable-sized fields.
	case Container:
		containerInfo, err := sszInfo.ContainerInfo()
		if err != nil {
			return fmt.Errorf("could not get container info: %w", err)
		}

		// Dereference first in case value is a pointer.
		derefValue := dereferencePointer(value)

		// Start with the fixed size of this Container.
		currentOffset := sszInfo.FixedSize()

		for _, fieldName := range containerInfo.order {
			fieldInfo := containerInfo.fields[fieldName]
			childSszInfo := fieldInfo.sszInfo
			if childSszInfo == nil {
				return fmt.Errorf("sszInfo is nil for field %s", fieldName)
			}

			// Skip fixed-size fields.
			if !childSszInfo.isVariable {
				continue
			}

			// Set the actual offset for variable-sized fields.
			fieldInfo.offset = currentOffset

			// Recursively populate variable-sized fields.
			fieldValue := derefValue.FieldByName(fieldInfo.goFieldName)
			if err := PopulateVariableLengthInfo(childSszInfo, fieldValue.Interface()); err != nil {
				return fmt.Errorf("could not populate from value for field %s: %w", fieldName, err)
			}

			currentOffset += childSszInfo.Size()
		}

		return nil
	default:
		return fmt.Errorf("unsupported SSZ type (%s) for variable size info", sszInfo.sszType)
	}
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

	// Parse the first dimension from the tag and get remaining tag for element
	sszDimension, remainingTag, err := ParseSSZTag(tag)
	if err != nil {
		return nil, fmt.Errorf("could not parse SSZ tag: %w", err)
	}
	if sszDimension == nil {
		return nil, errors.New("ssz tag is required for slice types")
	}

	// Analyze element type with remaining dimensions
	elementInfo, err := analyzeType(typ.Elem(), remainingTag)
	if err != nil {
		return nil, fmt.Errorf("could not analyze element type for homogeneous collection: %w", err)
	}

	// 1. Handle List/Bitlist type
	if sszDimension.IsList() {
		limit, err := sszDimension.GetListLimit()
		if err != nil {
			return nil, fmt.Errorf("could not get list limit: %w", err)
		}

		return analyzeListType(typ, elementInfo, limit)
	}

	// 2. Handle Vector/Bitvector type
	if sszDimension.IsVector() {
		length, err := sszDimension.GetVectorLength()
		if err != nil {
			return nil, fmt.Errorf("could not get vector length: %w", err)
		}

		return analyzeVectorType(typ, elementInfo, length)
	}

	// Parsing ssz tag doesn't provide enough information to determine the collection type,
	// return an error.
	return nil, errors.New("could not determine collection type from tags")
}

// analyzeListType analyzes SSZ List type and returns its SSZ info.
func analyzeListType(typ reflect.Type, elementInfo *sszInfo, limit uint64) (*sszInfo, error) {
	if elementInfo == nil {
		return nil, errors.New("element info is required for List")
	}

	return &sszInfo{
		sszType: List,
		typ:     typ,

		fixedSize:  offsetBytes,
		isVariable: true,

		listInfo: &listInfo{
			limit:   limit,
			element: elementInfo,
		},
	}, nil
}

// analyzeVectorType analyzes SSZ Vector type and returns its SSZ info.
func analyzeVectorType(typ reflect.Type, elementInfo *sszInfo, length uint64) (*sszInfo, error) {
	if elementInfo == nil {
		return nil, errors.New("element info is required for Vector")
	}

	// Validate the given length.
	// https://github.com/ethereum/consensus-specs/blob/master/ssz/simple-serialize.md#illegal-types
	if length == 0 {
		return nil, fmt.Errorf("vector length must be greater than 0, got %d", length)
	}

	return &sszInfo{
		sszType: Vector,
		typ:     typ,

		fixedSize:  length * elementInfo.Size(),
		isVariable: false,

		vectorInfo: &vectorInfo{
			length:  length,
			element: elementInfo,
		},
	}, nil
}

// analyzeContainerType analyzes SSZ Container type and returns its SSZ info.
func analyzeContainerType(typ reflect.Type) (*sszInfo, error) {
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("can only analyze struct types, got %v", typ.Kind())
	}

	fields := make(map[string]*fieldInfo)
	order := make([]string, 0, typ.NumField())

	sszInfo := &sszInfo{
		sszType: Container,
		typ:     typ,
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

		// Store nested struct info.
		fields[fieldName] = &fieldInfo{
			sszInfo:     info,
			offset:      currentOffset,
			goFieldName: field.Name,
		}
		// Persist order
		order = append(order, fieldName)

		// Update the current offset depending on whether the field is variable-sized.
		if info.isVariable {
			// If one of the fields is variable-sized,
			// the entire struct is considered variable-sized.
			sszInfo.isVariable = true
			currentOffset += offsetBytes
		} else {
			currentOffset += info.fixedSize
		}
	}

	sszInfo.fixedSize = currentOffset
	sszInfo.containerInfo = &containerInfo{
		fields: fields,
		order:  order,
	}

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
