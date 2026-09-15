package primitives

import (
	"encoding/binary"
	"fmt"

	"github.com/OffchainLabs/methodical-ssz/ssz"
)

var _ ssz.HashRoot = (Epoch)(0)
var _ ssz.Marshaler = (*Epoch)(nil)
var _ ssz.Unmarshaler = (*Epoch)(nil)

// MarshalUint64 appends the SSZ (little-endian) encoding of i to dst and
// returns the extended buffer. These uint64 serialization helpers live in the
// primitives package, rather than encoding/ssz, because encoding/ssz depends
// on primitives (transitively), so primitives cannot import it.
func MarshalUint64(dst []byte, i uint64) []byte {
	return binary.LittleEndian.AppendUint64(dst, i)
}

// UnmarshalUint64 decodes a little-endian uint64 from the first 8 bytes of src.
func UnmarshalUint64(src []byte) uint64 {
	return binary.LittleEndian.Uint64(src)
}

// SSZUint64 --
type SSZUint64 uint64

// SizeSSZ --
func (s *SSZUint64) SizeSSZ() int {
	return 8
}

// MarshalSSZTo --
func (s *SSZUint64) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := s.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ --
func (s *SSZUint64) MarshalSSZ() ([]byte, error) {
	marshalled := MarshalUint64([]byte{}, uint64(*s))
	return marshalled, nil
}

// UnmarshalSSZ --
func (s *SSZUint64) UnmarshalSSZ(buf []byte) error {
	if len(buf) != s.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", s.SizeSSZ(), len(buf))
	}
	*s = SSZUint64(UnmarshalUint64(buf))
	return nil
}

// HashTreeRoot --
func (s *SSZUint64) HashTreeRoot() ([32]byte, error) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(*s))
	var root [32]byte
	copy(root[:], buf)
	return root, nil
}

// HashTreeRootWith --
func (s *SSZUint64) HashTreeRootWith(hh *ssz.Hasher) error {
	indx := hh.Index()
	hh.PutUint64(uint64(*s))
	hh.Merkleize(indx)
	return nil
}
