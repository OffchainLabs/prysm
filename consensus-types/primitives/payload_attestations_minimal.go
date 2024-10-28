//go:build minimal

package primitives

import bitfield "github.com/prysmaticlabs/go-bitfield"

func NewPayloadAttestationAggregationBits() bitfield.Bitvector32 {
	return bitfield.NewBitvector32()
}
