package primitives

import (
	"fmt"

	"github.com/OffchainLabs/methodical-ssz/ssz"
)

var _ ssz.HashRoot = (CommitteeIndex)(0)
var _ ssz.Marshaler = (*CommitteeIndex)(nil)
var _ ssz.Unmarshaler = (*CommitteeIndex)(nil)

// CommitteeIndex --
type CommitteeIndex uint64

// HashTreeRoot returns calculated hash root.
func (c CommitteeIndex) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(c)
}

// HashTreeRootWith --
func (c CommitteeIndex) HashTreeRootWith(hh *ssz.Hasher) error {
	hh.PutUint64(uint64(c))
	return nil
}

// UnmarshalSSZ --
func (c *CommitteeIndex) UnmarshalSSZ(buf []byte) error {
	if len(buf) != c.SizeSSZ() {
		return fmt.Errorf("expected buffer of length %d receiced %d", c.SizeSSZ(), len(buf))
	}
	*c = CommitteeIndex(UnmarshalUint64(buf))
	return nil
}

// MarshalSSZTo --
func (c *CommitteeIndex) MarshalSSZTo(dst []byte) ([]byte, error) {
	marshalled, err := c.MarshalSSZ()
	if err != nil {
		return nil, err
	}
	return append(dst, marshalled...), nil
}

// MarshalSSZ --
func (c *CommitteeIndex) MarshalSSZ() ([]byte, error) {
	marshalled := MarshalUint64([]byte{}, uint64(*c))
	return marshalled, nil
}

// SizeSSZ returns the size of the serialized object.
func (c *CommitteeIndex) SizeSSZ() int {
	return 8
}
