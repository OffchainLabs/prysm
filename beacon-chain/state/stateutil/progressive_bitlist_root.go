package stateutil

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/methodical-ssz/ssz"
)

type progressiveBitlist bitfield.Bitlist

// HashTreeRoot --
func (p progressiveBitlist) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(p)
}

// HashTreeRootWith --
func (p progressiveBitlist) HashTreeRootWith(hh *ssz.Hasher) error {
	hh.PutProgressiveBitlist([]byte(p))
	return nil
}

// The generated proto hashes these fields with PutProgressiveBitlist, so this must too.
func ProgressiveBitlistRoot(bits bitfield.Bitlist) ([32]byte, error) {
	return progressiveBitlist(bits).HashTreeRoot()
}
