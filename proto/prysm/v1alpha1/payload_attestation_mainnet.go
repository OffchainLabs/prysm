//go:build !minimal

package eth

import "github.com/OffchainLabs/go-bitfield"

func NewPayloadAttestationAggregationBits() bitfield.Bitvector512 {
	return bitfield.NewBitvector512()
}
