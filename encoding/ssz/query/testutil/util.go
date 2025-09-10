package testutil

import (
	"fmt"
	"reflect"

	ssz "github.com/prysmaticlabs/fastssz"
)

// marshalAny marshals any value into SSZ format.
func marshalAny(value any) ([]byte, error) {
	// First check if it implements ssz.Marshaler (this catches custom types like primitives.Epoch)
	if marshaler, ok := value.(ssz.Marshaler); ok {
		return marshaler.MarshalSSZ()
	}

	valueType := reflect.TypeOf(value)
	if valueType.Kind() == reflect.Slice && valueType.Elem().Kind() != reflect.Uint8 {
		return marshalSlice(value)
	}

	// Handle custom type aliases by checking if they're based on primitive types
	if valueType.PkgPath() != "" {
		switch valueType.Kind() {
		case reflect.Uint64:
			return ssz.MarshalUint64(make([]byte, 0), reflect.ValueOf(value).Uint()), nil
		case reflect.Uint32:
			return ssz.MarshalUint32(make([]byte, 0), uint32(reflect.ValueOf(value).Uint())), nil
		case reflect.Bool:
			return ssz.MarshalBool(make([]byte, 0), reflect.ValueOf(value).Bool()), nil
		}
	}

	switch v := value.(type) {
	case []byte:
		return v, nil
	case []uint64:
		buf := make([]byte, 0, len(v)*8)
		for _, val := range v {
			buf = ssz.MarshalUint64(buf, val)
		}
		return buf, nil
	case uint64:
		return ssz.MarshalUint64(make([]byte, 0), v), nil
	case uint32:
		return ssz.MarshalUint32(make([]byte, 0), v), nil
	case bool:
		return ssz.MarshalBool(make([]byte, 0), v), nil

	default:
		return nil, fmt.Errorf("unsupported type for SSZ marshalling: %T", value)
	}
}

func marshalSlice(value any) ([]byte, error) {
	valueType := reflect.TypeOf(value)

	if valueType.Kind() != reflect.Slice {
		return nil, fmt.Errorf("expected slice, got %T", value)
	}

	sliceValue := reflect.ValueOf(value)
	var result []byte

	// Marshal each element recursively
	for i := 0; i < sliceValue.Len(); i++ {
		elem := sliceValue.Index(i).Interface()
		data, err := marshalAny(elem)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal slice element at index %d: %w", i, err)
		}
		result = append(result, data...)
	}
	return result, nil
}
