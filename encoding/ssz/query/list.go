package query

import (
	"errors"
	"fmt"
)

// listInfo holds information about a SSZ List type.
//
// length is initialized with zero,
// and can be set using SetLength while populating the actual SSZ List.
type listInfo struct {
	// limit is the maximum number of elements in the list.
	limit uint64
	// element is the SSZ info of the list's element type.
	element *sszInfo
	// length is the actual number of elements at runtime (0 if not set).
	length uint64
}

func (l *listInfo) Limit() uint64 {
	if l == nil {
		return 0
	}
	return l.limit
}

func (l *listInfo) Element() (*sszInfo, error) {
	if l == nil {
		return nil, errors.New("listInfo is nil")
	}
	return l.element, nil
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
