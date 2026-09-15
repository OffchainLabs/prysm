package primitives

import (
	"github.com/OffchainLabs/methodical-ssz/ssz"
)

// SSZBytes --
type SSZBytes []byte

// HashTreeRoot --
func (b *SSZBytes) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(b)
}

// HashTreeRootWith --
func (b *SSZBytes) HashTreeRootWith(hh *ssz.Hasher) error {
	indx := hh.Index()
	hh.PutBytes(*b)
	hh.Merkleize(indx)
	return nil
}
