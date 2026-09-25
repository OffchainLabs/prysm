package stateutil

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

func AvailableCommitteeWindowRoot(committees []*ethpb.AvailableCommittee) ([32]byte, error) {
	roots := make([][32]byte, len(committees))
	for i, c := range committees {
		if c == nil {
			return [32]byte{}, fmt.Errorf("invalid available committee at position %d", i)
		}
		r, err := c.HashTreeRoot()
		if err != nil {
			return [32]byte{}, err
		}
		roots[i] = r
	}
	return ssz.MerkleizeVector(roots, uint64(len(roots))), nil
}
