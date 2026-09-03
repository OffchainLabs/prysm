package ssz_test

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/config/features"
	"github.com/OffchainLabs/prysm/v7/encoding/ssz"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

const (
	listLimit  = 1 << 20
	bytesLimit = 1 << 30
)

type byteList []byte

func (b byteList) HashTreeRoot() ([32]byte, error) {
	return ssz.ByteSliceRoot(b, bytesLimit)
}

type progressiveByteList []byte

func (b progressiveByteList) HashTreeRoot() ([32]byte, error) {
	return ssz.ByteSliceRootProgressive(b)
}

// maxByteLists returns listLimit one-byte lists backed by one allocation.
func maxByteLists[T ~[]byte]() []T {
	lists := make([]T, listLimit)
	buf := make([]byte, len(lists))
	for i := range lists {
		buf[i] = byte(i)
		lists[i] = T(buf[i : i+1 : i+1])
	}
	return lists
}

func BenchmarkSliceRoot_MaxByteLists(b *testing.B) {
	reset := features.InitWithReset(&features.Flags{})
	defer reset()
	lists := maxByteLists[byteList]()
	b.ResetTimer()
	for b.Loop() {
		_, err := ssz.SliceRoot(lists, listLimit)
		require.NoError(b, err)
	}
}

func BenchmarkSliceRootProgressive_MaxByteLists(b *testing.B) {
	reset := features.InitWithReset(&features.Flags{})
	defer reset()
	lists := maxByteLists[progressiveByteList]()
	b.ResetTimer()
	for b.Loop() {
		_, err := ssz.SliceRootProgressive(lists)
		require.NoError(b, err)
	}
}
