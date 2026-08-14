package ssz

import (
	"fmt"

	fssz "github.com/prysmaticlabs/fastssz"
)

// SplitVariableList splits the SSZ List encoding of variable-size elements into the encodings of its
// elements, without decoding them. It bounds the element count and should be the limit the list type declares.
func SplitVariableList(b []byte, maxLen int) ([][]byte, error) {
	n, err := fssz.DecodeDynamicLength(b, maxLen)
	if err != nil {
		return nil, fmt.Errorf("decode dynamic length: %w", err)
	}

	elements := make([][]byte, n)
	if err = fssz.UnmarshalDynamic(b, n, func(i int, elem []byte) error {
		elements[i] = elem
		return nil
	}); err != nil {
		return nil, fmt.Errorf("unmarshal dynamic: %w", err)
	}

	return elements, nil
}
