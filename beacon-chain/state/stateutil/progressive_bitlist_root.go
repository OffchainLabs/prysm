package stateutil

import (
	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/methodical-ssz/ssz"
)

type progressiveBitlist []byte

func (p progressiveBitlist) HashTreeRoot() ([32]byte, error) {
	return ssz.HashWithDefaultHasher(p)
}

func (p progressiveBitlist) HashTreeRootWith(hh *ssz.Hasher) error {
	hh.PutProgressiveBitlist(p)
	return nil
}

// The generated state hashes these fields with PutProgressiveBitlist, so the field root must match it.
func ProgressiveBitlistRoot(bits bitfield.Bitlist) ([32]byte, error) {
	return progressiveBitlist(bits).HashTreeRoot()
}
