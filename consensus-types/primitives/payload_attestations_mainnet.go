//go:build !minimal

package primitives

import bitfield "github.com/prysmaticlabs/go-bitfield"

func NewPayloadAttestationAggregationBits() bitfield.Bitvector512 {
	return bitfield.NewBitvector512()
}
