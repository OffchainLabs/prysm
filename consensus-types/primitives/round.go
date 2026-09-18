package primitives

import (
	"fmt"

	"github.com/OffchainLabs/methodical-ssz/ssz"
)

var _ ssz.HashRoot = (Round)(0)
var _ ssz.Marshaler = (*Round)(nil)
var _ ssz.Unmarshaler = (*Round)(nil)

// Round represents a global attestation round index. Every active validator attests
// once per round, not once per slot.
type Round uint64

// HashTreeRoot --
func (r Round) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(r)
}

// HashTreeRootWith --
func (r Round) HashTreeRootWith(hh *ssz.Hasher) error {
	hh.PutUint64(uint64(r))
	return nil
}

// UnmarshalSSZ --
func (r *Round) UnmarshalSSZ(buf []byte) error {
	if len(buf) != r.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d received %d", r.SizeSSZ(), len(buf))
	}
	*r = Round(UnmarshalUint64(buf))
	return nil
}

// MarshalSSZTo --
func (r *Round) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := r.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ --
func (r *Round) MarshalSSZ() ([]byte, error) {
	return MarshalUint64([]byte{}, uint64(*r)), nil
}

// SizeSSZ --
func (r *Round) SizeSSZ() int {
	return 8
}
