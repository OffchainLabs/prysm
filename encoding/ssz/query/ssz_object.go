package query

import "errors"

type SSZObject interface {
	HashTreeRoot() ([32]byte, error)
}

// HashTreeRoot calls the HashTreeRoot method on the stored interface if it implements SSZObject.
// Returns the 32-byte hash tree root or an error if the interface doesn't support hashing.
func (info *sszInfo) HashTreeRoot() ([32]byte, error) {
	if info == nil {
		return [32]byte{}, errors.New("sszInfo is nil")
	}

	if info.source == nil {
		return [32]byte{}, errors.New("sszInfo.source is nil")
	}

	// Check if the value implements the Hashable interface
	return info.source.HashTreeRoot()
}
