package primitives_test

import (
	"encoding/binary"
	"slices"
	"strconv"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

func TestBuilderIndex_SSZRoundTripAndHashRoot(t *testing.T) {
	cases := []uint64{
		0,
		1,
		42,
		(1 << 32) - 1,
		1 << 32,
		^uint64(0),
	}

	for _, v := range cases {
		t.Run("v="+u64name(v), func(t *testing.T) {
			t.Parallel()

			val := primitives.BuilderIndex(v)
			require.Equal(t, 8, (&val).SizeSSZ())

			enc, err := (&val).MarshalSSZ()
			require.NoError(t, err)
			require.Equal(t, 8, len(enc))

			wantEnc := make([]byte, 8)
			binary.LittleEndian.PutUint64(wantEnc, v)
			require.DeepEqual(t, wantEnc, enc)

			dstPrefix := []byte("prefix:")
			dst, err := (&val).MarshalSSZTo(slices.Clone(dstPrefix))
			require.NoError(t, err)
			wantDst := append(dstPrefix, wantEnc...)
			require.DeepEqual(t, wantDst, dst)

			var decoded primitives.BuilderIndex
			require.NoError(t, (&decoded).UnmarshalSSZ(enc))
			require.Equal(t, val, decoded)

			root, err := val.HashTreeRoot()
			require.NoError(t, err)

			var wantRoot [32]byte
			binary.LittleEndian.PutUint64(wantRoot[:8], v)
			require.Equal(t, wantRoot, root)
		})
	}
}

func TestBuilderIndex_UnmarshalSSZRejectsWrongSize(t *testing.T) {
	for _, size := range []int{7, 9} {
		t.Run("size="+strconv.Itoa(size), func(t *testing.T) {
			t.Parallel()
			var v primitives.BuilderIndex
			err := (&v).UnmarshalSSZ(make([]byte, size))
			require.ErrorContains(t, "expected buffer of length 8", err)
		})
	}
}

func u64name(v uint64) string {
	switch v {
	case 0:
		return "0"
	case 1:
		return "1"
	case 42:
		return "42"
	case (1 << 32) - 1:
		return "2^32-1"
	case 1 << 32:
		return "2^32"
	case ^uint64(0):
		return "max"
	default:
		return "custom"
	}
}
