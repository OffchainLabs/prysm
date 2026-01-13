package query

import "errors"

// vectorInfo holds information about a SSZ Vector type.
type vectorInfo struct {
	// element is the SSZ info of the vector's element type.
	element *SszInfo
	// length is the fixed length of the vector.
	length uint64
}

func (v *vectorInfo) Length() uint64 {
	if v == nil {
		return 0
	}

	return v.length
}

func (v *vectorInfo) Element() (*SszInfo, error) {
	if v == nil {
		return nil, errors.New("vectorInfo is nil")
	}

	return v.element, nil
}

func (v *vectorInfo) Size() uint64 {
	if v == nil {
		return 0
	}

	if v.element == nil {
		return 0
	}

	return v.length * v.element.Size()
}
