package query

import "errors"

type SSZObject interface {
	HashTreeRoot() ([32]byte, error)
	SizeSSZ() int
}

// HashTreeRoot calls the HashTreeRoot method on the stored interface if it implements SSZObject.
// Returns the 32-byte hash tree root or an error if the interface doesn't support hashing.
func (info *SszInfo) HashTreeRoot() ([32]byte, error) {
	if info == nil {
		return [32]byte{}, errors.New("SszInfo is nil")
	}

	if info.source == nil {
		return [32]byte{}, errors.New("SszInfo.source is nil")
	}

	// Check if the value implements the Hashable interface
	return info.source.HashTreeRoot()
}
