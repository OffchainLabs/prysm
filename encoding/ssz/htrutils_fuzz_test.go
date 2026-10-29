package ssz_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// FuzzPackChunks checks that the chunks hold the joined items, zero-padded to a multiple of 32 bytes.
func FuzzPackChunks(f *testing.F) {
	// One 40-byte item: two chunks, the second padded.
	f.Add(bytes.Repeat([]byte{1}, 40), uint8(39))
	// Two 32-byte items take the one-chunk-per-item path.
	f.Add(bytes.Repeat([]byte{2}, 64), uint8(31))
	// No items.
	f.Add([]byte{}, uint8(0))

	f.Fuzz(func(t *testing.T, data []byte, size uint8) {
		// Callers pass items of one size, 1 to 64 bytes here; the remainder of data is dropped.
		n := int(size)%64 + 1
		items := make([][]byte, len(data)/n)
		for i := range items {
			items[i] = data[i*n : (i+1)*n]
		}

		chunks, err := ssz.PackByChunk(items)
		require.NoError(t, err)

		got := make([]byte, 0, len(chunks)*32)
		for _, c := range chunks {
			got = append(got, c[:]...)
		}
		want := bytes.Join(items, nil)
		want = append(want, make([]byte, (32-len(want)%32)%32)...)
		if len(want) == 0 {
			want = make([]byte, 32)
		}
		require.Equal(t, hex.EncodeToString(want), hex.EncodeToString(got))
	})
}
