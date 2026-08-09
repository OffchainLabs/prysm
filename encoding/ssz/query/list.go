package query

import (
	"errors"
	"fmt"
	"reflect"
)

// listInfo holds information about a SSZ List type.
//
// length is initialized with zero,
// and can be set using SetLength while populating the actual SSZ List.
type listInfo struct {
	// limit is the maximum number of elements in the list.
	limit uint64
	// element is the SSZ info of the list's element type.
	element *SszInfo
	// sliceValue is the reflect.Value of the underlying slice.
	sliceValue reflect.Value
	// length is the actual number of elements at runtime (0 if not set).
	length uint64
	// elementSizes caches each element's byte size for variable-sized type elements
	elementSizes []uint64
}

func (l *listInfo) Limit() uint64 {
	if l == nil {
		return 0
	}
	return l.limit
}

func (l *listInfo) Element() (*SszInfo, error) {
	if l == nil {
		return nil, errors.New("listInfo is nil")
	}
	return l.element, nil
}

// ElementValue returns the reflect.Value of the element at the given index.
// On error (for example, if listInfo is nil, the underlying slice is not valid or has the wrong kind,
// or the index is out of bounds), it returns the zero/invalid reflect.Value together with a non-nil error.
func (l *listInfo) ElementValue(index int) (reflect.Value, error) {
	if l == nil {
		return reflect.Value{}, errors.New("listInfo is nil")
	}

	sliceValue := l.sliceValue

	if !sliceValue.IsValid() {
		return reflect.Value{}, errors.New("sliceValue is not valid")
	}

	// Safe-guard: ensure sliceValue is actually a slice or array before calling Len and Index.
	if sliceValue.Kind() != reflect.Slice && sliceValue.Kind() != reflect.Array {
		return reflect.Value{}, fmt.Errorf("sliceValue has kind %s, expected slice or array", sliceValue.Kind())
	}

	if index < 0 || index >= sliceValue.Len() {
		return reflect.Value{}, fmt.Errorf("index %d out of bounds for list of length %d", index, sliceValue.Len())
	}

	return sliceValue.Index(index), nil
}

func (l *listInfo) Length() uint64 {
	if l == nil {
		return 0
	}
	return l.length
}

func (l *listInfo) SetLength(length uint64) error {
	if l == nil {
		return errors.New("listInfo is nil")
	}

	if length > l.limit {
		return fmt.Errorf("length %d exceeds limit %d", length, l.limit)
	}

	l.length = length
	return nil
}

func (l *listInfo) Size() uint64 {
	if l == nil {
		return 0
	}

	// For fixed-sized type elements, size is multiplying length by element size.
	if !l.element.isVariable {
		return l.length * l.element.Size()
	}

	// For variable-sized type elements, sum up the sizes of each element.
	totalSize := uint64(0)
	for _, sz := range l.elementSizes {
		totalSize += sz
	}
	return totalSize
}
