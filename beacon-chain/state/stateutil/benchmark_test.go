package stateutil_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func BenchmarkMerkleize_Buffered(b *testing.B) {
	roots := make([][32]byte, 8192)
	for i := range 8192 {
		roots[0] = [32]byte{byte(i)}
	}

	newMerkleize := func(chunks [][32]byte, count uint64, limit uint64) ([32]byte, error) {
		leafIndexer := func(i uint64) []byte {
			return chunks[i][:]
		}
		return ssz.Merkleize(ssz.NewHasherFunc(hash.CustomSHA256Hasher()), count, limit, leafIndexer), nil
	}

	b.ReportAllocs()
	for b.Loop() {
		_, err := newMerkleize(roots, 8192, 8192)
		require.NoError(b, err)
	}
}
