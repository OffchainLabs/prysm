//go:build go1.18

package ssz_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
)

func FuzzUint64Root(f *testing.F) {
	f.Fuzz(func(t *testing.T, i uint64) {
		_ = ssz.Uint64Root(i)
	})
}

func FuzzPackChunks(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		if _, err := ssz.PackByChunk([][]byte{b}); err != nil {
			t.Fatal(err)
		}
	})
}
