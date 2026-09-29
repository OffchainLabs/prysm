package server

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/pkg/errors"
)

// ConvertList converts each element of src, collecting per-index failures instead of failing the whole list.
func ConvertList[S, D any](src []S, convert func(S) (D, error)) ([]D, []*IndexedError) {
	dst := make([]D, len(src))
	var failures []*IndexedError
	for i, s := range src {
		d, err := convert(s)
		if err != nil {
			failures = append(failures, &IndexedError{Index: i, Message: err.Error()})
			continue
		}
		dst[i] = d
	}
	return dst, failures
}

// DecodeJSONList decodes a JSON array of *J and converts each element.
func DecodeJSONList[J, C any](r io.Reader, convert func(*J) (C, error)) ([]C, []*IndexedError, error) {
	var src []*J
	if err := json.NewDecoder(r).Decode(&src); err != nil {
		return nil, nil, err
	}

	items, failures := ConvertList(src, func(j *J) (C, error) {
		if j == nil {
			return *new(C), errors.New("null element")
		}
		return convert(j)
	})

	return items, failures, nil
}

type sszUnmarshaler interface {
	UnmarshalSSZ([]byte) error
	SizeSSZ() int
}

// DecodeSSZList decodes an SSZ list of fixed-size elements. An empty body yields zero items.
func DecodeSSZList[T any, PT interface {
	*T
	sszUnmarshaler
}](r io.Reader) ([]PT, []*IndexedError, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read request body: %w", err)
	}

	size := PT(new(T)).SizeSSZ()
	if len(b)%size != 0 {
		return nil, nil, errors.New("invalid SSZ list size")
	}

	elems := make([][]byte, len(b)/size)
	for i := range elems {
		elems[i] = b[i*size : (i+1)*size]
	}

	items, failures := ConvertList(elems, func(e []byte) (PT, error) {
		m := PT(new(T))
		if err := m.UnmarshalSSZ(e); err != nil {
			return nil, fmt.Errorf("could not decode SSZ message: %w", err)
		}
		return m, nil
	})

	return items, failures, nil
}
