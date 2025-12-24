package stateutil

import (
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
)

// BuilderPendingPaymentsRoot computes the merkle root of a slice of BuilderPendingPayment.
func BuilderPendingPaymentsRoot(slice []*ethpb.BuilderPendingPayment) ([32]byte, error) {
	roots := make([][32]byte, len(slice))

	for i, payment := range slice {
		r, err := payment.HashTreeRoot()
		if err != nil {
			return [32]byte{}, err
		}

		roots[i] = r
	}

	return ssz.MerkleizeVector(roots, uint64(len(roots))), nil
}
