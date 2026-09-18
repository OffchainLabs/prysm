package primitives

import (
	"fmt"

	"github.com/OffchainLabs/methodical-ssz/ssz"
)

var _ ssz.HashRoot = (Height)(0)
var _ ssz.Marshaler = (*Height)(nil)
var _ ssz.Unmarshaler = (*Height)(nil)

// Height represents a finality height. Unlike an epoch it advances only when a height
// outcome is consumed, never with the clock.
type Height uint64

// HashTreeRoot --
func (h Height) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(h)
}

// HashTreeRootWith --
func (h Height) HashTreeRootWith(hh *ssz.Hasher) error {
	hh.PutUint64(uint64(h))
	return nil
}

// UnmarshalSSZ --
func (h *Height) UnmarshalSSZ(buf []byte) error {
	if len(buf) != h.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", h.SizeSSZ(), len(buf))
	}
	*h = Height(UnmarshalUint64(buf))
	return nil
}

// MarshalSSZTo --
func (h *Height) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := h.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ --
func (h *Height) MarshalSSZ() ([]byte, error) {
	return MarshalUint64([]byte{}, uint64(*h)), nil
}

// SizeSSZ --
func (h *Height) SizeSSZ() int {
	return 8
}
