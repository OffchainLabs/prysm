package peers

import (
	"testing"

	"github.com/OffchainLabs/go-bitfield"
)

func Benchmark_retrieveIndicesFromBitfield(b *testing.B) {
	bv := bitfield.NewBitvector64()
	for i := uint64(0); i < bv.Len(); i++ {
		bv.SetBitAt(i, true)
	}

	for b.Loop() {
		indicesFromBitfield(bv)
	}
}
