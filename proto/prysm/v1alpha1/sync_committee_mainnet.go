//go:build !minimal

package eth

import (
	"github.com/OffchainLabs/go-bitfield"
)

func NewSyncCommitteeAggregationBits() bitfield.Bitvector128 {
	return bitfield.NewBitvector128()
}

func ConvertToSyncContributionBitVector(b []byte) bitfield.Bitvector128 {
	return b
}
