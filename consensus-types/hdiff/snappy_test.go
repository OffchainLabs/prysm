package hdiff

import (
	"bytes"
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/golang/snappy"
)

func TestSnappyDecode(t *testing.T) {
	t.Run("valid cases", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			data []byte
		}{
			{"empty", nil},
			{"ordinary", []byte("state diff")},
			{"high_compression", bytes.Repeat([]byte{0}, 1<<16)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				decoded, err := snappyDecode(snappy.Encode(nil, tc.data))
				require.NoError(t, err)
				require.Equal(t, true, bytes.Equal(tc.data, decoded))
			})
		}
	})

	t.Run("invalid cases", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			input []byte
		}{
			{"missing_header", nil},
			{"truncated_header", []byte{0x80}},
			{"overflowing_header", []byte{0xff, 0xff, 0xff, 0xff, 0x1f}},
			{"truncated_literal", []byte{1, 0}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				decoded, err := snappyDecode(tc.input)
				require.ErrorIs(t, err, snappy.ErrCorrupt)
				require.Equal(t, 0, len(decoded))
			})
		}
	})

	t.Run("reject before allocation", func(t *testing.T) {
		input := []byte{0x80, 0x80, 0x40} // Declares 1 MiB of decoded data with no body.
		var err error
		allocations := testing.AllocsPerRun(1, func() {
			_, err = snappyDecode(input)
		})
		require.ErrorIs(t, err, snappy.ErrCorrupt)
		require.Equal(t, float64(0), allocations)
	})

	t.Run("ratio boundary", func(t *testing.T) {
		// Both inputs are 4 bytes with a corrupt body; only the declared decoded length differs.
		// 32*4 must reach snappy.Decode (which allocates), 32*4+1 must be rejected before allocation.
		atLimit := []byte{0x80, 0x01, 0, 0}   // declares 128 decoded bytes
		overLimit := []byte{0x81, 0x01, 0, 0} // declares 129 decoded bytes
		var atLimitErr, overLimitErr error
		atLimitAllocs := testing.AllocsPerRun(1, func() { _, atLimitErr = snappyDecode(atLimit) })
		overLimitAllocs := testing.AllocsPerRun(1, func() { _, overLimitErr = snappyDecode(overLimit) })
		require.ErrorIs(t, atLimitErr, snappy.ErrCorrupt)
		require.NotEqual(t, float64(0), atLimitAllocs, "n == 32*len(input) must not be rejected by the ratio check")
		require.ErrorIs(t, overLimitErr, snappy.ErrCorrupt)
		require.Equal(t, float64(0), overLimitAllocs, "n == 32*len(input)+1 must be rejected before allocation")
	})
}
