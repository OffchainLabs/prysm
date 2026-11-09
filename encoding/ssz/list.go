package ssz

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// bytesPerLengthOffset is the size of a serialized SSZ offset.
const bytesPerLengthOffset = 4

// MarshalVariableList joins the SSZ encodings of variable-size elements into the SSZ List encoding:
// a vector of 4-byte offsets followed by the elements. It is the inverse of SplitVariableList.
func MarshalVariableList(elements ...[]byte) []byte {
	var offsets, data []byte

	offset := bytesPerLengthOffset * len(elements)
	for _, e := range elements {
		offsets = binary.LittleEndian.AppendUint32(offsets, uint32(offset))
		offset += len(e)
		data = append(data, e...)
	}

	return append(offsets, data...)
}

// SplitVariableList splits the SSZ List encoding of variable-size elements into the encodings of its
// elements, without decoding them. It bounds the element count and should be the limit the list type declares.
func SplitVariableList(b []byte, maxLen int) ([][]byte, error) {
	size := uint64(len(b))
	if size == 0 {
		return nil, nil
	}
	if size < bytesPerLengthOffset {
		return nil, errors.New("buffer is shorter than a single offset")
	}

	// The first offset points past the offset vector, so it also gives the element count.
	first := uint64(binary.LittleEndian.Uint32(b))
	if first == 0 || first%bytesPerLengthOffset != 0 {
		return nil, fmt.Errorf("first offset %d is not a positive multiple of %d", first, bytesPerLengthOffset)
	}
	n := first / bytesPerLengthOffset
	if n > uint64(maxLen) {
		return nil, fmt.Errorf("element count %d exceeds max %d", n, maxLen)
	}
	if size < first {
		return nil, fmt.Errorf("offset vector of %d bytes does not fit in a %d byte buffer", first, size)
	}

	elements := make([][]byte, n)
	start := first
	for i := range n {
		end := size
		if i+1 < n {
			end = uint64(binary.LittleEndian.Uint32(b[bytesPerLengthOffset*(i+1):]))
		}
		if start > end {
			return nil, fmt.Errorf("offsets out of order at element %d: %d > %d", i, start, end)
		}
		if end > size {
			return nil, fmt.Errorf("offset %d at element %d exceeds buffer size %d", end, i, size)
		}
		elements[i] = b[start:end]
		start = end
	}

	return elements, nil
}
