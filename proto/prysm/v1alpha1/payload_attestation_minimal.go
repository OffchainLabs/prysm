//go:build minimal

package eth

import "github.com/OffchainLabs/go-bitfield"

func NewPayloadAttestationAggregationBits() bitfield.Bitvector2 {
	return bitfield.NewBitvector2()
}
