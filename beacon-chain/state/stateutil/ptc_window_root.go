package stateutil

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// PTCWindowRoot computes the merkle root of the cached PTC window.
func PTCWindowRoot(ptcs []*ethpb.PTCs) ([32]byte, error) {
	roots := make([][32]byte, len(ptcs))

	for i, ptc := range ptcs {
		if ptc == nil {
			return [32]byte{}, fmt.Errorf("invalid PTC at position %d", i)
		}

		r, err := ptc.HashTreeRoot()
		if err != nil {
			return [32]byte{}, err
		}

		roots[i] = r
	}

	return ssz.MerkleizeVector(roots, uint64(len(roots))), nil
}
