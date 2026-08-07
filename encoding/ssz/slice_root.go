package ssz

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/pkg/errors"
)

// SliceRoot computes the root of a slice of hashable objects.
func SliceRoot[T Hashable](slice []T, limit uint64) ([32]byte, error) {
	max := limit
	if uint64(len(slice)) > max {
		return [32]byte{}, fmt.Errorf("slice exceeds max length %d", max)
	}

	roots, err := ElementRoots(slice)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not merkleize object")
	}

	sliceRoot, err := BitwiseMerkleize(roots, uint64(len(roots)), limit)
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not slice merkleization")
	}
	sliceLenBuf := new(bytes.Buffer)
	if err := binary.Write(sliceLenBuf, binary.LittleEndian, uint64(len(slice))); err != nil {
		return [32]byte{}, errors.Wrap(err, "could not marshal slice length")
	}
	// We need to mix in the length of the slice.
	sliceLenRoot := make([]byte, 32)
	copy(sliceLenRoot, sliceLenBuf.Bytes())
	return MixInLength(sliceRoot, sliceLenRoot), nil
}
