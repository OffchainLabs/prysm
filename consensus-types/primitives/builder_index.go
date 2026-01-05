package primitives

import (
	"fmt"

	fssz "github.com/prysmaticlabs/fastssz"
)

var _ fssz.HashRoot = (BuilderIndex)(0)
var _ fssz.Marshaler = (*BuilderIndex)(nil)
var _ fssz.Unmarshaler = (*BuilderIndex)(nil)

// BuilderIndex is an index into the builder registry.
type BuilderIndex uint64

// HashTreeRoot returns the SSZ hash tree root of the index.
func (b BuilderIndex) HashTreeRoot() ([32]byte, error) {
	return fssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith appends the SSZ uint64 representation of the index to the given hasher.
func (b BuilderIndex) HashTreeRootWith(hh *fssz.Hasher) error {
	hh.PutUint64(uint64(b))
	return nil
}

// UnmarshalSSZ decodes the SSZ-encoded uint64 index from buf.
func (b *BuilderIndex) UnmarshalSSZ(buf []byte) error {
	if len(buf) != b.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", b.SizeSSZ(), len(buf))
	}
	*b = BuilderIndex(fssz.UnmarshallUint64(buf))
	return nil
}

// MarshalSSZTo appends the SSZ-encoded index to dst and returns the extended buffer.
func (b *BuilderIndex) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := b.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ encodes the index as an SSZ uint64.
func (b *BuilderIndex) MarshalSSZ() ([]byte, error) {
	marshalled := fssz.MarshalUint64([]byte{}, uint64(*b))
	return marshalled, nil
}

// SizeSSZ returns the size of the SSZ-encoded index in bytes.
func (b *BuilderIndex) SizeSSZ() int {
	return 8
}
