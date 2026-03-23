package stateutil

import (
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PTCWindowRoot computes the merkle root of the cached PTC window.
func PTCWindowRoot(slice []*ethpb.PTCs) ([32]byte, error) {
	roots := make([][32]byte, len(slice))

	for i, slot := range slice {
		r, err := slot.HashTreeRoot()
		if err != nil {
			return [32]byte{}, err
		}

		roots[i] = r
	}

	return ssz.MerkleizeVector(roots, uint64(len(roots))), nil
}
